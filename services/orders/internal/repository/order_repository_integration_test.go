//go:build integration

// Test Budget: 6 distinct behaviors × 2 = 12 max integration tests
// Actual: 11
//
// Behavior 1: cart round-trip — SaveNewCart + ReplaceItems + GetCart;
//
//	unknown cart IDs are ErrNotFound
//
// Behavior 2: PlaceOrderFromCart — one transaction creates order + items,
//
//	deletes the cart, and records order_placed; an empty cart is a
//	validation error and writes nothing
//
// Behavior 3: MarkPaidIfPayable — entity-approved transition persists with
//
//	order_paid; a second attempt conflicts
//
// Behavior 4: tenant scoping (ADR-001) — another tenant's order is
//
//	ErrNotFound
//
// Behavior 5: FulfillIfFulfillable — paid orders fulfill with tracking and
//
//	the fulfilled event; pending orders are rejected
package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/outbox"
	"github.com/jorge-sanchez/cloud-commerce/pkg/testdb"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
)

// openMigratedDB provisions an isolated database (shared server in CI,
// testcontainer locally) and applies this service's up migrations.
func openMigratedDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn, cleanup := testdb.Open(t)
	t.Cleanup(cleanup)

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	migrations, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, migrations, "no up migrations found")
	for _, m := range migrations {
		ddl, err := os.ReadFile(m)
		require.NoError(t, err)
		_, err = db.Exec(string(ddl))
		require.NoError(t, err, "apply %s", m)
	}
	return db
}

func intAddr() domain.Address {
	return domain.Address{Name: "Buyer", Line1: "Av. Test 123", City: "Austin", Country: "US"}
}

const (
	tenantA  = "11111111-1111-1111-1111-111111111111"
	tenantB  = "22222222-2222-2222-2222-222222222222"
	variant1 = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
)

func cartWithItem(t *testing.T, repo *PostgresOrderRepository) *domain.Cart {
	t.Helper()
	cart, err := repo.SaveNewCart(context.Background(), &domain.Cart{TenantID: tenantA, Currency: "PEN"})
	require.NoError(t, err)
	require.NoError(t, cart.AddItem(domain.Item{VariantID: variant1, SKU: "TS-S", Title: "T-Shirt", PriceCents: 4990, Qty: 2}))
	_, err = repo.ReplaceItems(context.Background(), cart)
	require.NoError(t, err)
	return cart
}

// ---------------------------------------------------------------------------
// Behavior 1: cart round-trip
// ---------------------------------------------------------------------------

func TestPostgresOrderRepository_CartRoundTrip_PersistsItems(t *testing.T) {
	repo := NewPostgresOrderRepository(openMigratedDB(t))
	cart := cartWithItem(t, repo)

	got, err := repo.GetCart(context.Background(), cart.ID)

	require.NoError(t, err)
	assert.Equal(t, tenantA, got.TenantID)
	require.Len(t, got.Items, 1, "the cart line must round-trip")
	assert.Equal(t, int64(2*4990), got.TotalCents())
}

func TestPostgresOrderRepository_GetCart_Unknown_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresOrderRepository(openMigratedDB(t))

	_, err := repo.GetCart(context.Background(), "33333333-3333-3333-3333-333333333333")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Behavior 2: PlaceOrderFromCart is atomic
// ---------------------------------------------------------------------------

func TestPostgresOrderRepository_PlaceOrderFromCart_Valid_CreatesOrderDeletesCartRecordsEvent(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresOrderRepository(db, WithEventRecorder(outbox.NewRecorder()))
	cart := cartWithItem(t, repo)

	order, err := repo.PlaceOrderFromCart(context.Background(), cart.ID, "Buyer@Example.Test", intAddr(), "Standard", 500, domain.TaxSpec{})

	require.NoError(t, err)
	assert.Equal(t, domain.OrderStatusPending, order.Status)
	assert.Equal(t, int64(2*4990+500), order.TotalCents, "total must include shipping")
	assert.Positive(t, order.Number, "the database must assign an order number")

	_, err = repo.GetCart(context.Background(), cart.ID)
	require.ErrorIs(t, err, apperrors.ErrNotFound, "the cart must be consumed by checkout")

	var eventCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE event_type = $1`, domain.OrderPlacedEventType,
	).Scan(&eventCount))
	assert.Equal(t, 1, eventCount, "order_placed must be recorded with the order")
}

func TestPostgresOrderRepository_PlaceOrderFromCart_EmptyCart_ValidationAndNothingWritten(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresOrderRepository(db, WithEventRecorder(outbox.NewRecorder()))
	cart, err := repo.SaveNewCart(context.Background(), &domain.Cart{TenantID: tenantA, Currency: "PEN"})
	require.NoError(t, err)

	_, err = repo.PlaceOrderFromCart(context.Background(), cart.ID, "buyer@example.test", intAddr(), "Standard", 500, domain.TaxSpec{})

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)

	var orders, outboxRows int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&orders))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM outbox`).Scan(&outboxRows))
	assert.Equal(t, 0, orders, "the rejected checkout must not create an order")
	assert.Equal(t, 0, outboxRows, "the rejected checkout must not record an event")

	_, err = repo.GetCart(context.Background(), cart.ID)
	require.NoError(t, err, "the cart must survive a rejected checkout")
}

// ---------------------------------------------------------------------------
// Behavior 3: MarkPaidIfPayable — the entity decides
// ---------------------------------------------------------------------------

func TestPostgresOrderRepository_MarkPaidIfPayable_PendingThenRepeat_PaysOnceThenConflicts(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresOrderRepository(db, WithEventRecorder(outbox.NewRecorder()))
	cart := cartWithItem(t, repo)
	order, err := repo.PlaceOrderFromCart(context.Background(), cart.ID, "buyer@example.test", intAddr(), "Standard", 500, domain.TaxSpec{})
	require.NoError(t, err)

	paid, err := repo.MarkPaidIfPayable(context.Background(), order.ID, "pi_test_ref")
	require.NoError(t, err)
	assert.Equal(t, domain.OrderStatusPaid, paid.Status)

	var eventCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE event_type = $1`, domain.OrderPaidEventType,
	).Scan(&eventCount))
	assert.Equal(t, 1, eventCount, "order_paid must be recorded with the transition")

	_, err = repo.MarkPaidIfPayable(context.Background(), order.ID, "pi_test_ref")
	require.ErrorIs(t, err, apperrors.ErrConflict, "a second payment must be rejected by the entity")
}

func TestPostgresOrderRepository_MarkPaidIfPayable_UnknownOrder_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresOrderRepository(openMigratedDB(t))

	_, err := repo.MarkPaidIfPayable(context.Background(), "33333333-3333-3333-3333-333333333333", "pi_test_ref")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Behavior 4: tenant scoping — the cross-tenant negative case (ADR-001)
// ---------------------------------------------------------------------------

func TestPostgresOrderRepository_GetByID_OtherTenantsOrder_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresOrderRepository(openMigratedDB(t))
	cart := cartWithItem(t, repo)
	order, err := repo.PlaceOrderFromCart(context.Background(), cart.ID, "buyer@example.test", intAddr(), "Standard", 500, domain.TaxSpec{})
	require.NoError(t, err)

	_, err = repo.GetByID(context.Background(), tenantB, order.ID)

	require.ErrorIs(t, err, apperrors.ErrNotFound, "another tenant's order must be indistinguishable from missing")
}

func TestPostgresOrderRepository_ListByTenant_ReturnsOrdersWithItems(t *testing.T) {
	repo := NewPostgresOrderRepository(openMigratedDB(t))
	cart := cartWithItem(t, repo)
	_, err := repo.PlaceOrderFromCart(context.Background(), cart.ID, "buyer@example.test", intAddr(), "Standard", 500, domain.TaxSpec{})
	require.NoError(t, err)

	orders, total, err := repo.ListByTenant(context.Background(), tenantA, 1, 20)

	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, orders, 1, "the tenant's order must be listed")
	require.Len(t, orders[0].Items, 1, "listed orders must carry their items")
}

// ---------------------------------------------------------------------------
// Behavior 5: FulfillIfFulfillable — the entity decides
// ---------------------------------------------------------------------------

func TestPostgresOrderRepository_FulfillIfFulfillable_PaidOrder_FulfillsWithTrackingAndEvent(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresOrderRepository(db, WithEventRecorder(outbox.NewRecorder()))
	cart := cartWithItem(t, repo)
	order, err := repo.PlaceOrderFromCart(context.Background(), cart.ID, "buyer@example.test", intAddr(), "Standard", 500, domain.TaxSpec{})
	require.NoError(t, err)
	_, err = repo.MarkPaidIfPayable(context.Background(), order.ID, "pi_test_ref")
	require.NoError(t, err)

	fulfilled, err := repo.FulfillIfFulfillable(context.Background(), tenantA, order.ID, "TRK-123", "olva")

	require.NoError(t, err)
	assert.Equal(t, domain.OrderStatusFulfilled, fulfilled.Status)
	assert.Equal(t, "TRK-123", fulfilled.TrackingNumber)

	var eventCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE event_type = $1`, domain.OrderFulfilledEventType,
	).Scan(&eventCount))
	assert.Equal(t, 1, eventCount, "order_fulfilled must be recorded with the transition")
}

func TestPostgresOrderRepository_FulfillIfFulfillable_PendingOrder_Conflicts(t *testing.T) {
	repo := NewPostgresOrderRepository(openMigratedDB(t))
	cart := cartWithItem(t, repo)
	order, err := repo.PlaceOrderFromCart(context.Background(), cart.ID, "buyer@example.test", intAddr(), "Standard", 500, domain.TaxSpec{})
	require.NoError(t, err)

	_, err = repo.FulfillIfFulfillable(context.Background(), tenantA, order.ID, "", "")

	require.ErrorIs(t, err, apperrors.ErrConflict, "an unpaid order must not be fulfillable")
}

// ---------------------------------------------------------------------------
// Behavior 6 (issue #38): POS sales are born paid and idempotent
// ---------------------------------------------------------------------------

func TestPostgresOrderRepository_SavePOSSale_Replayed_ReturnsOriginalOnce(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresOrderRepository(db, WithEventRecorder(outbox.NewRecorder()))
	sale, err := domain.NewPOSSale(tenantA, "PEN", "", "",
		[]domain.Item{{VariantID: variant1, SKU: "TS-S", Title: "T-Shirt", PriceCents: 4990, Qty: 2}}, domain.TaxSpec{})
	require.NoError(t, err)

	first, err := repo.SavePOSSale(context.Background(), tenantA, "sale-abc", sale)
	require.NoError(t, err)
	assert.Equal(t, domain.OrderStatusPaid, first.Status, "POS sales are born paid")

	replay, err := repo.SavePOSSale(context.Background(), tenantA, "sale-abc", sale)
	require.NoError(t, err)
	assert.Equal(t, first.ID, replay.ID, "the queued retry must return the original sale")

	var orders, paidEvents int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&orders))
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE event_type = $1`, domain.OrderPaidEventType).Scan(&paidEvents))
	assert.Equal(t, 1, orders, "a replay must not create a second order")
	assert.Equal(t, 1, paidEvents, "a replay must not emit a second paid event")
}
