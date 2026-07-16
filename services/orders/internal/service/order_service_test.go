// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 5
//
// Behavior 1: CreateCart — resolves the store through the platform port and
//
//	binds tenant + currency; resolution failures pass through
//
// Behavior 2: AddItem — snapshots price/title server-side from the platform
//
//	port; an inactive/unknown variant passes ErrNotFound through
//
// Behavior 3: Checkout — delegates to the repository (entity decides there)
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
)

// ---------------------------------------------------------------------------
// Hand-rolled fakes at the port boundaries — no gomock, no testify/mock.
// ---------------------------------------------------------------------------

var _ domain.OrderRepository = (*fakeOrderRepo)(nil)

type fakeOrderRepo struct {
	posSales []*domain.Order
	carts    []*domain.Cart
	replaced []*domain.Cart
	cart     *domain.Cart
	order    *domain.Order
	err      error
}

func (f *fakeOrderRepo) SaveNewCart(_ context.Context, cart *domain.Cart) (*domain.Cart, error) {
	stored := *cart
	stored.ID = "cart-001"
	stored.Items = []domain.Item{}
	f.carts = append(f.carts, &stored)
	return &stored, f.err
}

func (f *fakeOrderRepo) GetCart(_ context.Context, _ string) (*domain.Cart, error) {
	return f.cart, f.err
}

func (f *fakeOrderRepo) ReplaceItems(_ context.Context, cart *domain.Cart) (*domain.Cart, error) {
	f.replaced = append(f.replaced, cart)
	return cart, nil
}

func (f *fakeOrderRepo) PlaceOrderFromCart(_ context.Context, _, _ string, _ domain.Address, _ string, _ int64) (*domain.Order, error) {
	return f.order, f.err
}

func (f *fakeOrderRepo) MarkPaidIfPayable(_ context.Context, _, _ string) (*domain.Order, error) {
	return f.order, f.err
}

func (f *fakeOrderRepo) RefundIfRefundable(_ context.Context, _, _ string) (*domain.Order, error) {
	return f.order, f.err
}

func (f *fakeOrderRepo) GetByID(_ context.Context, _, _ string) (*domain.Order, error) {
	return f.order, f.err
}

func (f *fakeOrderRepo) GetPublicByID(_ context.Context, _ string) (*domain.Order, error) {
	return f.order, f.err
}

func (f *fakeOrderRepo) SavePOSSale(_ context.Context, tenantID, _ string, order *domain.Order) (*domain.Order, error) {
	stored := *order
	stored.ID = "order-pos-001"
	stored.TenantID = tenantID
	f.posSales = append(f.posSales, &stored)
	return &stored, f.err
}

func (f *fakeOrderRepo) FulfillIfFulfillable(_ context.Context, _, _, _, _ string) (*domain.Order, error) {
	return f.order, f.err
}

func (f *fakeOrderRepo) ListByTenant(_ context.Context, _ string, _, _ int) ([]*domain.Order, int, error) {
	return nil, 0, f.err
}

func (f *fakeOrderRepo) GetSalesSummary(_ context.Context, _ string, _ int) (*domain.SalesSummary, error) {
	return &domain.SalesSummary{}, f.err
}

var _ Platform = (*fakePlatform)(nil)

type fakePlatform struct {
	store       StoreInfo
	storeErr    error
	variant     VariantSnapshot
	variantErr  error
	shipping    ShippingMethod
	shippingErr error
}

func (f *fakePlatform) ResolveStore(_ context.Context, _ string) (StoreInfo, error) {
	return f.store, f.storeErr
}

func (f *fakePlatform) GetActiveVariant(_ context.Context, _, _ string) (VariantSnapshot, error) {
	return f.variant, f.variantErr
}

func (f *fakePlatform) GetShippingMethod(_ context.Context, _, _ string) (ShippingMethod, error) {
	return f.shipping, f.shippingErr
}

// ---------------------------------------------------------------------------
// Behavior 1: CreateCart resolves the store
// ---------------------------------------------------------------------------

func TestOrderService_CreateCart_ResolvableStore_BindsTenantAndCurrency(t *testing.T) {
	repo := &fakeOrderRepo{}
	svc := NewOrderService(repo, &fakePlatform{store: StoreInfo{TenantID: "tenant-001", Currency: "PEN"}})

	cart, err := svc.CreateCart(context.Background(), "tienda-jorge")

	require.NoError(t, err)
	assert.Equal(t, "tenant-001", cart.TenantID)
	assert.Equal(t, "PEN", cart.Currency)
	require.Len(t, repo.carts, 1, "exactly one cart must be written")
}

func TestOrderService_CreateCart_UnknownStore_PassesNotFoundThrough(t *testing.T) {
	repo := &fakeOrderRepo{}
	svc := NewOrderService(repo, &fakePlatform{storeErr: apperrors.ErrNotFound})

	_, err := svc.CreateCart(context.Background(), "no-such-store")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
	require.Len(t, repo.carts, 0, "no cart may be written for an unknown store")
}

// ---------------------------------------------------------------------------
// Behavior 2: AddItem snapshots server-side
// ---------------------------------------------------------------------------

func TestOrderService_AddItem_ActiveVariant_SnapshotsPriceFromPlatform(t *testing.T) {
	repo := &fakeOrderRepo{cart: &domain.Cart{ID: "cart-001", TenantID: "tenant-001", Currency: "PEN"}}
	svc := NewOrderService(repo, &fakePlatform{
		variant: VariantSnapshot{VariantID: "var-001", SKU: "TS-S", Title: "T-Shirt", PriceCents: 4990},
	})

	cart, err := svc.AddItem(context.Background(), "cart-001", "var-001", 2)

	require.NoError(t, err)
	require.Len(t, repo.replaced, 1, "the cart must be persisted once")
	require.Len(t, cart.Items, 1)
	assert.Equal(t, int64(4990), cart.Items[0].PriceCents, "the price must come from the platform, not the client")
	assert.Equal(t, int64(2*4990), cart.TotalCents())
}

func TestOrderService_AddItem_InactiveVariant_PassesNotFoundThrough(t *testing.T) {
	repo := &fakeOrderRepo{cart: &domain.Cart{ID: "cart-001", TenantID: "tenant-001"}}
	svc := NewOrderService(repo, &fakePlatform{variantErr: apperrors.ErrNotFound})

	_, err := svc.AddItem(context.Background(), "cart-001", "var-draft", 1)

	require.ErrorIs(t, err, apperrors.ErrNotFound)
	require.Len(t, repo.replaced, 0, "nothing may be persisted for an unpurchasable variant")
}

// ---------------------------------------------------------------------------
// Behavior 3: Checkout delegates
// ---------------------------------------------------------------------------

func validAddr() domain.Address {
	return domain.Address{Name: "Buyer", Line1: "Av. Test 123", City: "Austin", Country: "US"}
}

func TestOrderService_Checkout_PricesShippingServerSide(t *testing.T) {
	repo := &fakeOrderRepo{cart: &domain.Cart{ID: "cart-001", TenantID: "tenant-001", Currency: "PEN"},
		order: &domain.Order{ID: "order-001"}}
	svc := NewOrderService(repo, &fakePlatform{shipping: ShippingMethod{ID: "m1", Name: "Standard", PriceCents: 500}})

	_, err := svc.Checkout(context.Background(), "cart-001", "buyer@example.test", validAddr(), "m1")

	require.NoError(t, err)
}

func TestOrderService_Checkout_UnknownShippingMethod_PassesNotFoundThrough(t *testing.T) {
	repo := &fakeOrderRepo{cart: &domain.Cart{ID: "cart-001", TenantID: "tenant-001"}}
	svc := NewOrderService(repo, &fakePlatform{shippingErr: apperrors.ErrNotFound})

	_, err := svc.Checkout(context.Background(), "cart-001", "buyer@example.test", validAddr(), "bogus")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
}
