// Package repository holds the persistence adapters. Repositories load and
// save — the entity decides. No business logic lives here.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

// EventRecorder writes an event envelope inside the caller's transaction —
// the transactional-outbox port (ADR-002). Implemented by outbox.Recorder.
type EventRecorder interface {
	Record(ctx context.Context, tx *sql.Tx, env events.Envelope) error
}

// PostgresProductRepository implements domain.ProductRepository on PostgreSQL.
type PostgresProductRepository struct {
	db     *sql.DB       // required
	events EventRecorder // may be nil
}

var _ domain.ProductRepository = (*PostgresProductRepository)(nil)

// Option configures optional dependencies on the repository.
type Option func(*PostgresProductRepository)

// WithEventRecorder wires the outbox recorder. Without it, state changes
// persist but no events are recorded.
func WithEventRecorder(rec EventRecorder) Option {
	return func(r *PostgresProductRepository) { r.events = rec }
}

// NewPostgresProductRepository wires the repository to an open *sql.DB.
func NewPostgresProductRepository(db *sql.DB, opts ...Option) *PostgresProductRepository {
	r := &PostgresProductRepository{db: db}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

const uniqueViolation = "23505"

func (r *PostgresProductRepository) SaveNewWithVariants(ctx context.Context, tenantID string, p *domain.Product) (*domain.Product, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	optionsJSON, err := json.Marshal(p.Options)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	product := *p
	product.TenantID = tenantID
	err = tx.QueryRowContext(ctx, `
		INSERT INTO products (tenant_id, title, description, status, options)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`,
		tenantID, p.Title, p.Description, string(p.Status), optionsJSON,
	).Scan(&product.ID, &product.CreatedAt, &product.UpdatedAt)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	product.Variants = make([]*domain.Variant, 0, len(p.Variants))
	for _, v := range p.Variants {
		valuesJSON, err := json.Marshal(v.OptionValues)
		if err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		variant := *v
		variant.ProductID = product.ID
		err = tx.QueryRowContext(ctx, `
			INSERT INTO variants (product_id, tenant_id, sku, option_values, price_cents)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at`,
			product.ID, tenantID, v.SKU, valuesJSON, v.PriceCents,
		).Scan(&variant.ID, &variant.CreatedAt)
		if err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && string(pqErr.Code) == uniqueViolation {
				return nil, apperrors.ErrConflict.Wrap(err)
			}
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		product.Variants = append(product.Variants, &variant)
	}

	// Record the event in the same transaction as the aggregate (ADR-002).
	if r.events != nil {
		event := domain.NewProductCreatedEvent(&product, time.Now().UTC())
		env, err := events.New(tenantID, product.ID, domain.ProductCreatedEventType, event.CreatedAt, event)
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
	return &product, nil
}

const productColumns = `id, tenant_id, title, description, status, options, created_at, updated_at`

func (r *PostgresProductRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.Product, error) {
	product, err := scanProduct(r.db.QueryRowContext(ctx,
		`SELECT `+productColumns+` FROM products WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	))
	if err != nil {
		return nil, err
	}
	if err := r.loadVariants(ctx, tenantID, []*domain.Product{product}); err != nil {
		return nil, err
	}
	return product, nil
}

func (r *PostgresProductRepository) ListByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Product, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM products WHERE tenant_id = $1`, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT `+productColumns+` FROM products WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		tenantID, pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	products := make([]*domain.Product, 0, pageSize)
	for rows.Next() {
		p, err := scanProductRows(rows)
		if err != nil {
			return nil, 0, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	if err := r.loadVariants(ctx, tenantID, products); err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

func (r *PostgresProductRepository) ListActiveByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Product, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM products WHERE tenant_id = $1 AND status = 'active'`, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT `+productColumns+` FROM products
		WHERE tenant_id = $1 AND status = 'active'
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		tenantID, pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	products := make([]*domain.Product, 0, pageSize)
	for rows.Next() {
		p, err := scanProductRows(rows)
		if err != nil {
			return nil, 0, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	if err := r.loadVariants(ctx, tenantID, products); err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

// ActivateIfActivatable loads the product inside a transaction, lets the
// entity decide the transition, and persists what the entity decided.
func (r *PostgresProductRepository) ActivateIfActivatable(ctx context.Context, tenantID, id string) (*domain.Product, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	product, err := scanProduct(tx.QueryRowContext(ctx,
		`SELECT `+productColumns+` FROM products WHERE tenant_id = $1 AND id = $2 FOR UPDATE`,
		tenantID, id,
	))
	if err != nil {
		return nil, err
	}
	if err := r.loadVariantsTx(ctx, tx, tenantID, product); err != nil {
		return nil, err
	}

	if err := product.Activate(); err != nil { // entity decides
		return nil, apperrors.ErrConflict.Wrap(err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE products SET status = $1, updated_at = NOW()
		WHERE tenant_id = $2 AND id = $3`,
		string(product.Status), tenantID, id,
	); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	// Record the event in the same transaction as the state change (ADR-002).
	if r.events != nil {
		event := domain.NewProductActivatedEvent(product, time.Now().UTC())
		env, err := events.New(tenantID, product.ID, domain.ProductActivatedEventType, event.ActivatedAt, event)
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
	return product, nil
}

// loadVariants attaches variants to the given products with one query.
func (r *PostgresProductRepository) loadVariants(ctx context.Context, tenantID string, products []*domain.Product) error {
	if len(products) == 0 {
		return nil
	}
	byID := make(map[string]*domain.Product, len(products))
	ids := make([]string, 0, len(products))
	for _, p := range products {
		byID[p.ID] = p
		ids = append(ids, p.ID)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, product_id, sku, option_values, price_cents, created_at
		FROM variants WHERE tenant_id = $1 AND product_id = ANY($2)
		ORDER BY created_at`,
		tenantID, pq.Array(ids),
	)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		v, err := scanVariantRows(rows)
		if err != nil {
			return err
		}
		byID[v.ProductID].Variants = append(byID[v.ProductID].Variants, v)
	}
	if err := rows.Err(); err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}

func (r *PostgresProductRepository) loadVariantsTx(ctx context.Context, tx *sql.Tx, tenantID string, p *domain.Product) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, product_id, sku, option_values, price_cents, created_at
		FROM variants WHERE tenant_id = $1 AND product_id = $2
		ORDER BY created_at`,
		tenantID, p.ID,
	)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		v, err := scanVariantRows(rows)
		if err != nil {
			return err
		}
		p.Variants = append(p.Variants, v)
	}
	if err := rows.Err(); err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProductFrom(s rowScanner) (*domain.Product, error) {
	var p domain.Product
	var status string
	var optionsJSON []byte
	err := s.Scan(&p.ID, &p.TenantID, &p.Title, &p.Description, &status, &optionsJSON, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	if err := json.Unmarshal(optionsJSON, &p.Options); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	p.Status = domain.ProductStatus(status)
	return &p, nil
}

func scanProduct(row *sql.Row) (*domain.Product, error)       { return scanProductFrom(row) }
func scanProductRows(rows *sql.Rows) (*domain.Product, error) { return scanProductFrom(rows) }

func scanVariantRows(rows *sql.Rows) (*domain.Variant, error) {
	var v domain.Variant
	var valuesJSON []byte
	if err := rows.Scan(&v.ID, &v.ProductID, &v.SKU, &valuesJSON, &v.PriceCents, &v.CreatedAt); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	if err := json.Unmarshal(valuesJSON, &v.OptionValues); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return &v, nil
}
