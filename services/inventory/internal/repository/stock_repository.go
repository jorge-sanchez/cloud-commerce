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
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/domain"
)

// EventRecorder writes an event envelope inside the caller's transaction —
// the transactional-outbox port (ADR-002). Implemented by outbox.Recorder.
type EventRecorder interface {
	Record(ctx context.Context, tx *sql.Tx, env events.Envelope) error
}

// PostgresStockRepository implements domain.StockRepository on PostgreSQL.
type PostgresStockRepository struct {
	db     *sql.DB       // required
	events EventRecorder // may be nil
}

var _ domain.StockRepository = (*PostgresStockRepository)(nil)

// Option configures optional dependencies on the repository.
type Option func(*PostgresStockRepository)

// WithEventRecorder wires the outbox recorder. Without it, state changes
// persist but no events are recorded.
func WithEventRecorder(rec EventRecorder) Option {
	return func(r *PostgresStockRepository) { r.events = rec }
}

// NewPostgresStockRepository wires the repository to an open *sql.DB.
func NewPostgresStockRepository(db *sql.DB, opts ...Option) *PostgresStockRepository {
	r := &PostgresStockRepository{db: db}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *PostgresStockRepository) EnsureDefaultLocation(ctx context.Context, tenantID string) (*domain.Location, error) {
	// Idempotent: the partial unique index (one default per tenant) makes
	// concurrent first-writes safe.
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO locations (tenant_id, name, is_default)
		VALUES ($1, $2, TRUE)
		ON CONFLICT (tenant_id) WHERE is_default DO NOTHING`,
		tenantID, domain.DefaultLocationName,
	)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return r.scanLocation(r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, is_default, created_at
		FROM locations WHERE tenant_id = $1 AND is_default`,
		tenantID,
	))
}

func (r *PostgresStockRepository) SaveNewLocation(ctx context.Context, tenantID string, l *domain.Location) (*domain.Location, error) {
	location := *l
	location.TenantID = tenantID
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO locations (tenant_id, name, is_default)
		VALUES ($1, $2, FALSE)
		RETURNING id, created_at`,
		tenantID, l.Name,
	).Scan(&location.ID, &location.CreatedAt)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return &location, nil
}

func (r *PostgresStockRepository) ListLocations(ctx context.Context, tenantID string) ([]*domain.Location, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, is_default, created_at
		FROM locations WHERE tenant_id = $1
		ORDER BY is_default DESC, created_at`,
		tenantID,
	)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	locations := make([]*domain.Location, 0, 4)
	for rows.Next() {
		var l domain.Location
		if err := rows.Scan(&l.ID, &l.TenantID, &l.Name, &l.IsDefault, &l.CreatedAt); err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		locations = append(locations, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return locations, nil
}

func (r *PostgresStockRepository) InitializeStock(ctx context.Context, tenantID, locationID string, items []domain.StockInit) error {
	if len(items) == 0 {
		return nil
	}
	variantIDs := make([]string, 0, len(items))
	skus := make([]string, 0, len(items))
	for _, it := range items {
		variantIDs = append(variantIDs, it.VariantID)
		skus = append(skus, it.SKU)
	}
	// Idempotent by design: the consumer that calls this receives events
	// at-least-once (ADR-002), so replays must be no-ops.
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO stock_levels (tenant_id, location_id, variant_id, sku)
		SELECT $1, $2, v, s FROM UNNEST($3::uuid[], $4::text[]) AS t(v, s)
		ON CONFLICT (tenant_id, location_id, variant_id) DO NOTHING`,
		tenantID, locationID, pq.Array(variantIDs), pq.Array(skus),
	)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}

func (r *PostgresStockRepository) ListStockByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.StockLevel, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM stock_levels WHERE tenant_id = $1`, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, location_id, variant_id, sku, on_hand, updated_at
		FROM stock_levels WHERE tenant_id = $1
		ORDER BY sku, location_id
		LIMIT $2 OFFSET $3`,
		tenantID, pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	levels := make([]*domain.StockLevel, 0, pageSize)
	for rows.Next() {
		var s domain.StockLevel
		if err := rows.Scan(&s.ID, &s.TenantID, &s.LocationID, &s.VariantID, &s.SKU, &s.OnHand, &s.UpdatedAt); err != nil {
			return nil, 0, apperrors.ErrInternal.Wrap(err)
		}
		levels = append(levels, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	return levels, total, nil
}

// AdjustIfSufficient loads the stock level inside a transaction, lets the
// entity apply the delta, and persists what the entity decided.
func (r *PostgresStockRepository) AdjustIfSufficient(ctx context.Context, tenantID, locationID, variantID string, delta int64) (*domain.StockLevel, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	var s domain.StockLevel
	err = tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, location_id, variant_id, sku, on_hand, updated_at
		FROM stock_levels
		WHERE tenant_id = $1 AND location_id = $2 AND variant_id = $3
		FOR UPDATE`,
		tenantID, locationID, variantID,
	).Scan(&s.ID, &s.TenantID, &s.LocationID, &s.VariantID, &s.SKU, &s.OnHand, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	if err := s.Adjust(delta); err != nil { // entity decides
		return nil, apperrors.ErrConflict.Wrap(err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE stock_levels SET on_hand = $1, updated_at = NOW()
		WHERE id = $2`,
		s.OnHand, s.ID,
	); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	// Record the event in the same transaction as the state change (ADR-002).
	if r.events != nil {
		event := domain.NewStockAdjustedEvent(&s, delta, time.Now().UTC())
		env, err := events.New(tenantID, s.VariantID, domain.StockAdjustedEventType, event.AdjustedAt, event)
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
	return &s, nil
}

func (r *PostgresStockRepository) scanLocation(row *sql.Row) (*domain.Location, error) {
	var l domain.Location
	err := row.Scan(&l.ID, &l.TenantID, &l.Name, &l.IsDefault, &l.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return &l, nil
}
