// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 6
//
// Behavior 1: Cart.AddItem — accumulates qty for the same variant; rejects
//
//	non-positive quantities
//
// Behavior 2: NewOrderFromCart — totals and snapshots carry over; empty
//
//	carts are rejected
//
// Behavior 3: state machine — pending→paid→fulfilled; paid orders cannot
//
//	be cancelled
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
)

func testAddr() domain.Address {
	return domain.Address{Name: "Buyer", Line1: "Av. Test 123", City: "Austin", Country: "US"}
}

func cartWithShirt(t *testing.T) *domain.Cart {
	t.Helper()
	cart := &domain.Cart{ID: "cart-001", TenantID: "tenant-001", Currency: "PEN"}
	require.NoError(t, cart.AddItem(domain.Item{VariantID: "var-001", SKU: "TS-S", Title: "T-Shirt", PriceCents: 4990, Qty: 2}))
	return cart
}

// ---------------------------------------------------------------------------
// Behavior 1: AddItem
// ---------------------------------------------------------------------------

func TestCart_AddItem_SameVariantTwice_AccumulatesQty(t *testing.T) {
	cart := cartWithShirt(t)

	require.NoError(t, cart.AddItem(domain.Item{VariantID: "var-001", SKU: "TS-S", Title: "T-Shirt", PriceCents: 4990, Qty: 1}))

	require.Len(t, cart.Items, 1, "the same variant must stay one line")
	assert.Equal(t, int64(3), cart.Items[0].Qty)
	assert.Equal(t, int64(3*4990), cart.TotalCents())
}

func TestCart_AddItem_ZeroQty_ReturnsErrBadQty(t *testing.T) {
	cart := cartWithShirt(t)

	err := cart.AddItem(domain.Item{VariantID: "var-002", Qty: 0})

	require.ErrorIs(t, err, domain.ErrBadQty)
	require.Len(t, cart.Items, 1, "the rejected item must not be added")
}

// ---------------------------------------------------------------------------
// Behavior 2: NewOrderFromCart
// ---------------------------------------------------------------------------

func TestNewOrderFromCart_ValidCart_SnapshotsItemsAndTotal(t *testing.T) {
	cart := cartWithShirt(t)

	order, err := domain.NewOrderFromCart(cart, "Buyer@Example.Test", testAddr(), "Standard", 500)

	require.NoError(t, err)
	assert.Equal(t, domain.OrderStatusPending, order.Status)
	assert.Equal(t, "buyer@example.test", order.Email, "email must be normalized")
	assert.Equal(t, int64(2*4990+500), order.TotalCents, "total must include shipping")
	require.Len(t, order.Items, 1)
	assert.Equal(t, "PEN", order.Currency)
}

func TestNewOrderFromCart_EmptyCart_ReturnsErrEmptyCart(t *testing.T) {
	cart := &domain.Cart{ID: "cart-002", TenantID: "tenant-001", Currency: "PEN"}

	_, err := domain.NewOrderFromCart(cart, "buyer@example.test", testAddr(), "Standard", 0)

	require.ErrorIs(t, err, domain.ErrEmptyCart)
}

// ---------------------------------------------------------------------------
// Behavior 3: the order state machine
// ---------------------------------------------------------------------------

func TestOrder_MarkPaidThenFulfill_TransitionsInOrder(t *testing.T) {
	cart := cartWithShirt(t)
	order, err := domain.NewOrderFromCart(cart, "buyer@example.test", testAddr(), "Standard", 0)
	require.NoError(t, err)

	require.NoError(t, order.MarkPaid())
	assert.Equal(t, domain.OrderStatusPaid, order.Status)
	require.NoError(t, order.Fulfill("TRK-123", "olva"))
	assert.Equal(t, domain.OrderStatusFulfilled, order.Status)
}

func TestOrder_CancelAfterPaid_ReturnsErrNotCancellable(t *testing.T) {
	cart := cartWithShirt(t)
	order, err := domain.NewOrderFromCart(cart, "buyer@example.test", testAddr(), "Standard", 0)
	require.NoError(t, err)
	require.NoError(t, order.MarkPaid())

	err = order.Cancel()

	require.ErrorIs(t, err, domain.ErrNotCancellable)
	assert.Equal(t, domain.OrderStatusPaid, order.Status, "a rejected cancel must not change status")
}
