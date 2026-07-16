// Package repository holds the persistence adapters. Repositories load and
// save — the entity decides. No business logic lives here.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
)

// EventRecorder writes an event envelope inside the caller's transaction —
// the transactional-outbox port (ADR-002). Implemented by outbox.Recorder.
type EventRecorder interface {
	Record(ctx context.Context, tx *sql.Tx, env events.Envelope) error
}

// PostgresOrderRepository implements domain.OrderRepository on PostgreSQL.
type PostgresOrderRepository struct {
	db     *sql.DB       // required
	events EventRecorder // may be nil
}

var _ domain.OrderRepository = (*PostgresOrderRepository)(nil)

// Option configures optional dependencies on the repository.
type Option func(*PostgresOrderRepository)

// WithEventRecorder wires the outbox recorder. Without it, state changes
// persist but no events are recorded.
func WithEventRecorder(rec EventRecorder) Option {
	return func(r *PostgresOrderRepository) { r.events = rec }
}

// NewPostgresOrderRepository wires the repository to an open *sql.DB.
func NewPostgresOrderRepository(db *sql.DB, opts ...Option) *PostgresOrderRepository {
	r := &PostgresOrderRepository{db: db}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *PostgresOrderRepository) SaveNewCart(ctx context.Context, cart *domain.Cart) (*domain.Cart, error) {
	stored := *cart
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO carts (tenant_id, currency)
		VALUES ($1, $2)
		RETURNING id, created_at, updated_at`,
		cart.TenantID, cart.Currency,
	).Scan(&stored.ID, &stored.CreatedAt, &stored.UpdatedAt)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	stored.Items = []domain.Item{}
	return &stored, nil
}

func (r *PostgresOrderRepository) GetCart(ctx context.Context, cartID string) (*domain.Cart, error) {
	var cart domain.Cart
	err := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, currency, created_at, updated_at
		FROM carts WHERE id = $1`,
		cartID,
	).Scan(&cart.ID, &cart.TenantID, &cart.Currency, &cart.CreatedAt, &cart.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	items, err := r.loadItems(ctx, r.db, "cart_items", "cart_id", cartID)
	if err != nil {
		return nil, err
	}
	cart.Items = items
	return &cart, nil
}

// ReplaceItems persists the cart's lines atomically (delete + insert).
func (r *PostgresOrderRepository) ReplaceItems(ctx context.Context, cart *domain.Cart) (*domain.Cart, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM cart_items WHERE cart_id = $1`, cart.ID); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	for _, it := range cart.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cart_items (cart_id, variant_id, tenant_id, sku, title, price_cents, qty)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			cart.ID, it.VariantID, cart.TenantID, it.SKU, it.Title, it.PriceCents, it.Qty,
		); err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE carts SET updated_at = NOW() WHERE id = $1`, cart.ID,
	); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return cart, nil
}

// PlaceOrderFromCart converts the cart into a pending order in one
// transaction. The entity decides whether the conversion is legal.
func (r *PostgresOrderRepository) PlaceOrderFromCart(ctx context.Context, cartID, email string) (*domain.Order, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	var cart domain.Cart
	err = tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, currency FROM carts WHERE id = $1 FOR UPDATE`,
		cartID,
	).Scan(&cart.ID, &cart.TenantID, &cart.Currency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	cart.Items, err = r.loadItems(ctx, tx, "cart_items", "cart_id", cartID)
	if err != nil {
		return nil, err
	}

	order, err := domain.NewOrderFromCart(&cart, email) // entity decides
	if err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO orders (tenant_id, email, currency, total_cents, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, number, created_at, updated_at`,
		order.TenantID, order.Email, order.Currency, order.TotalCents, string(order.Status),
	).Scan(&order.ID, &order.Number, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	for _, it := range order.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO order_items (order_id, variant_id, tenant_id, sku, title, price_cents, qty)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			order.ID, it.VariantID, order.TenantID, it.SKU, it.Title, it.PriceCents, it.Qty,
		); err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM carts WHERE id = $1`, cartID); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	// Record the event in the same transaction as the aggregate (ADR-002).
	if r.events != nil {
		event := domain.NewOrderPlacedEvent(order, time.Now().UTC())
		env, err := events.New(order.TenantID, order.ID, domain.OrderPlacedEventType, event.PlacedAt, event)
		if err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		if err := r.events.Record(ctx, tx, env); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return order, nil
}

// SavePOSSale persists an in-person sale atomically, idempotent on the
// client sale ID (offline POS clients retry their queue).
func (r *PostgresOrderRepository) SavePOSSale(ctx context.Context, tenantID, clientSaleID string, order *domain.Order) (*domain.Order, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	stored := *order
	err = tx.QueryRowContext(ctx, `
		INSERT INTO orders (tenant_id, email, currency, total_cents, status, payment_reference, client_sale_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, client_sale_id) WHERE client_sale_id IS NOT NULL DO NOTHING
		RETURNING id, number, created_at, updated_at`,
		tenantID, order.Email, order.Currency, order.TotalCents, string(order.Status),
		"pos_cash_"+clientSaleID, clientSaleID,
	).Scan(&stored.ID, &stored.Number, &stored.CreatedAt, &stored.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Replay: return the original sale.
		existing, gerr := r.getBySaleID(ctx, tenantID, clientSaleID)
		if gerr != nil {
			return nil, gerr
		}
		return existing, nil
	}
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	for _, it := range stored.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO order_items (order_id, variant_id, tenant_id, sku, title, price_cents, qty)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			stored.ID, it.VariantID, tenantID, it.SKU, it.Title, it.PriceCents, it.Qty,
		); err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
	}
	if r.events != nil {
		event := domain.NewOrderPaidEvent(&stored, time.Now().UTC())
		env, err := events.New(tenantID, stored.ID, domain.OrderPaidEventType, event.PaidAt, event)
		if err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		if err := r.events.Record(ctx, tx, env); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return &stored, nil
}

func (r *PostgresOrderRepository) getBySaleID(ctx context.Context, tenantID, clientSaleID string) (*domain.Order, error) {
	order, err := r.scanOrder(r.db.QueryRowContext(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE tenant_id = $1 AND client_sale_id = $2`, tenantID, clientSaleID))
	if err != nil {
		return nil, err
	}
	order.Items, err = r.loadItems(ctx, r.db, "order_items", "order_id", order.ID)
	if err != nil {
		return nil, err
	}
	return order, nil
}

// MarkPaidIfPayable loads the order inside a transaction, lets the entity
// decide the transition, and persists what the entity decided. Looked up by
// order ID alone: payment confirmation arrives on the buyer's capability.
func (r *PostgresOrderRepository) MarkPaidIfPayable(ctx context.Context, orderID, paymentReference string) (*domain.Order, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	order, err := r.scanOrder(tx.QueryRowContext(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE id = $1 FOR UPDATE`, orderID))
	if err != nil {
		return nil, err
	}
	order.Items, err = r.loadItems(ctx, tx, "order_items", "order_id", order.ID)
	if err != nil {
		return nil, err
	}

	if err := order.MarkPaid(); err != nil { // entity decides
		return nil, apperrors.ErrConflict.Wrap(err)
	}
	order.PaymentReference = paymentReference

	if _, err := tx.ExecContext(ctx, `
		UPDATE orders SET status = $1, payment_reference = $2, updated_at = NOW() WHERE id = $3`,
		string(order.Status), order.PaymentReference, order.ID,
	); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	if r.events != nil {
		event := domain.NewOrderPaidEvent(order, time.Now().UTC())
		env, err := events.New(order.TenantID, order.ID, domain.OrderPaidEventType, event.PaidAt, event)
		if err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		if err := r.events.Record(ctx, tx, env); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return order, nil
}

// RefundIfRefundable loads the order inside a transaction, lets the entity
// decide the transition, and persists what the entity decided.
func (r *PostgresOrderRepository) RefundIfRefundable(ctx context.Context, tenantID, orderID string) (*domain.Order, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	order, err := r.scanOrder(tx.QueryRowContext(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenantID, orderID))
	if err != nil {
		return nil, err
	}
	order.Items, err = r.loadItems(ctx, tx, "order_items", "order_id", order.ID)
	if err != nil {
		return nil, err
	}

	if err := order.Refund(); err != nil { // entity decides
		return nil, apperrors.ErrConflict.Wrap(err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2`,
		string(order.Status), order.ID,
	); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	if r.events != nil {
		event := domain.NewOrderRefundedEvent(order, time.Now().UTC())
		env, err := events.New(order.TenantID, order.ID, domain.OrderRefundedEventType, event.RefundedAt, event)
		if err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		if err := r.events.Record(ctx, tx, env); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return order, nil
}

// FulfillIfFulfillable loads the order inside a transaction, lets the
// entity decide the transition, and persists what the entity decided.
func (r *PostgresOrderRepository) FulfillIfFulfillable(ctx context.Context, tenantID, orderID, trackingNumber, carrier string) (*domain.Order, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	order, err := r.scanOrder(tx.QueryRowContext(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenantID, orderID))
	if err != nil {
		return nil, err
	}
	order.Items, err = r.loadItems(ctx, tx, "order_items", "order_id", order.ID)
	if err != nil {
		return nil, err
	}

	if err := order.Fulfill(trackingNumber, carrier); err != nil { // entity decides
		return nil, apperrors.ErrConflict.Wrap(err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE orders SET status = $1, tracking_number = $2, carrier = $3, updated_at = NOW()
		WHERE id = $4`,
		string(order.Status), order.TrackingNumber, order.Carrier, order.ID,
	); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	if r.events != nil {
		event := domain.NewOrderFulfilledEvent(order, time.Now().UTC())
		env, err := events.New(order.TenantID, order.ID, domain.OrderFulfilledEventType, event.FulfilledAt, event)
		if err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		if err := r.events.Record(ctx, tx, env); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return order, nil
}

const orderColumns = `id, number, tenant_id, email, currency, total_cents, status, payment_reference, tracking_number, carrier, created_at, updated_at`

func (r *PostgresOrderRepository) GetByID(ctx context.Context, tenantID, orderID string) (*domain.Order, error) {
	order, err := r.scanOrder(r.db.QueryRowContext(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE tenant_id = $1 AND id = $2`, tenantID, orderID))
	if err != nil {
		return nil, err
	}
	order.Items, err = r.loadItems(ctx, r.db, "order_items", "order_id", order.ID)
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (r *PostgresOrderRepository) GetPublicByID(ctx context.Context, orderID string) (*domain.Order, error) {
	order, err := r.scanOrder(r.db.QueryRowContext(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE id = $1`, orderID))
	if err != nil {
		return nil, err
	}
	order.Items, err = r.loadItems(ctx, r.db, "order_items", "order_id", order.ID)
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (r *PostgresOrderRepository) ListByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Order, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM orders WHERE tenant_id = $1`, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT `+orderColumns+` FROM orders WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		tenantID, pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	orders := make([]*domain.Order, 0, pageSize)
	ids := make([]string, 0, pageSize)
	byID := make(map[string]*domain.Order, pageSize)
	for rows.Next() {
		var o domain.Order
		var status string
		if err := rows.Scan(&o.ID, &o.Number, &o.TenantID, &o.Email, &o.Currency, &o.TotalCents, &status, &o.PaymentReference, &o.TrackingNumber, &o.Carrier, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, 0, apperrors.ErrInternal.Wrap(err)
		}
		o.Status = domain.OrderStatus(status)
		o.Items = []domain.Item{}
		orders = append(orders, &o)
		ids = append(ids, o.ID)
		byID[o.ID] = &o
	}
	if err := rows.Err(); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	if len(ids) > 0 {
		itemRows, err := r.db.QueryContext(ctx, `
			SELECT order_id, variant_id, sku, title, price_cents, qty
			FROM order_items WHERE order_id = ANY($1)`,
			pq.Array(ids),
		)
		if err != nil {
			return nil, 0, apperrors.ErrInternal.Wrap(err)
		}
		defer func() { _ = itemRows.Close() }()
		for itemRows.Next() {
			var oid string
			var it domain.Item
			if err := itemRows.Scan(&oid, &it.VariantID, &it.SKU, &it.Title, &it.PriceCents, &it.Qty); err != nil {
				return nil, 0, apperrors.ErrInternal.Wrap(err)
			}
			byID[oid].Items = append(byID[oid].Items, it)
		}
		if err := itemRows.Err(); err != nil {
			return nil, 0, apperrors.ErrInternal.Wrap(err)
		}
	}
	return orders, total, nil
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (r *PostgresOrderRepository) loadItems(ctx context.Context, q querier, table, fk, id string) ([]domain.Item, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT variant_id, sku, title, price_cents, qty FROM `+table+` WHERE `+fk+` = $1 ORDER BY sku`,
		id,
	)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]domain.Item, 0, 8)
	for rows.Next() {
		var it domain.Item
		if err := rows.Scan(&it.VariantID, &it.SKU, &it.Title, &it.PriceCents, &it.Qty); err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return items, nil
}

func (r *PostgresOrderRepository) scanOrder(row *sql.Row) (*domain.Order, error) {
	var o domain.Order
	var status string
	err := row.Scan(&o.ID, &o.Number, &o.TenantID, &o.Email, &o.Currency, &o.TotalCents, &status, &o.PaymentReference, &o.TrackingNumber, &o.Carrier, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	o.Status = domain.OrderStatus(status)
	return &o, nil
}

// GetSalesSummary runs the two aggregate reads. SQL aggregates are plenty
// at this scale — a dedicated read model waits until they hurt (issue #30).
func (r *PostgresOrderRepository) GetSalesSummary(ctx context.Context, tenantID string, days int) (*domain.SalesSummary, error) {
	summary := &domain.SalesSummary{Days: []domain.DailySales{}, TopProducts: []domain.TopProduct{}}

	if err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(currency), '') FROM orders WHERE tenant_id = $1`, tenantID,
	).Scan(&summary.Currency); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT to_char(created_at::date, 'YYYY-MM-DD'), SUM(total_cents), COUNT(*)
		FROM orders
		WHERE tenant_id = $1 AND status IN ('paid', 'fulfilled')
		  AND created_at >= NOW() - ($2 || ' days')::interval
		GROUP BY created_at::date ORDER BY 1`,
		tenantID, days,
	)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var d domain.DailySales
		if err := rows.Scan(&d.Date, &d.RevenueCents, &d.Orders); err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		summary.Days = append(summary.Days, d)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	topRows, err := r.db.QueryContext(ctx, `
		SELECT oi.sku, MAX(oi.title), SUM(oi.qty), SUM(oi.qty * oi.price_cents)
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		WHERE oi.tenant_id = $1 AND o.status IN ('paid', 'fulfilled')
		  AND o.created_at >= NOW() - ($2 || ' days')::interval
		GROUP BY oi.sku ORDER BY 3 DESC LIMIT 10`,
		tenantID, days,
	)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = topRows.Close() }()
	for topRows.Next() {
		var t domain.TopProduct
		if err := topRows.Scan(&t.SKU, &t.Title, &t.Units, &t.RevenueCents); err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		summary.TopProducts = append(summary.TopProducts, t)
	}
	if err := topRows.Err(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return summary, nil
}
