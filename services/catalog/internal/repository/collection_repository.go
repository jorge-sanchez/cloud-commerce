package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

// PostgresCollectionRepository implements domain.CollectionRepository.
type PostgresCollectionRepository struct {
	db *sql.DB // required
}

var _ domain.CollectionRepository = (*PostgresCollectionRepository)(nil)

// NewPostgresCollectionRepository wires the repository to an open *sql.DB.
func NewPostgresCollectionRepository(db *sql.DB) *PostgresCollectionRepository {
	return &PostgresCollectionRepository{db: db}
}

func (r *PostgresCollectionRepository) SaveNew(ctx context.Context, tenantID string, c *domain.Collection) (*domain.Collection, error) {
	collection := *c
	collection.TenantID = tenantID
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO collections (tenant_id, title, handle)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`,
		tenantID, c.Title, c.Handle,
	).Scan(&collection.ID, &collection.CreatedAt, &collection.UpdatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == uniqueViolation {
			return nil, apperrors.ErrConflict.Wrap(err)
		}
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	collection.ProductIDs = []string{}
	return &collection, nil
}

func (r *PostgresCollectionRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.Collection, error) {
	var c domain.Collection
	err := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, title, handle, created_at, updated_at
		FROM collections WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	).Scan(&c.ID, &c.TenantID, &c.Title, &c.Handle, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT product_id FROM collection_products
		WHERE tenant_id = $1 AND collection_id = $2
		ORDER BY added_at`,
		tenantID, id,
	)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	c.ProductIDs = []string{}
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		c.ProductIDs = append(c.ProductIDs, pid)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return &c, nil
}

func (r *PostgresCollectionRepository) ListByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Collection, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collections WHERE tenant_id = $1`, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, title, handle, created_at, updated_at
		FROM collections WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		tenantID, pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	collections := make([]*domain.Collection, 0, pageSize)
	for rows.Next() {
		var c domain.Collection
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Title, &c.Handle, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, apperrors.ErrInternal.Wrap(err)
		}
		collections = append(collections, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	return collections, total, nil
}

// AddProduct verifies both sides belong to the tenant in one statement:
// the INSERT selects from the tenant-scoped product and collection rows, so
// a foreign or missing ID inserts nothing and reports ErrNotFound.
func (r *PostgresCollectionRepository) AddProduct(ctx context.Context, tenantID, collectionID, productID string) error {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO collection_products (collection_id, product_id, tenant_id)
		SELECT c.id, p.id, $1
		FROM collections c, products p
		WHERE c.tenant_id = $1 AND c.id = $2 AND p.tenant_id = $1 AND p.id = $3
		ON CONFLICT (collection_id, product_id) DO NOTHING`,
		tenantID, collectionID, productID,
	)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	if affected == 0 {
		// Distinguish "already a member" (fine) from "not found".
		var exists bool
		err := r.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM collection_products
				WHERE tenant_id = $1 AND collection_id = $2 AND product_id = $3
			)`,
			tenantID, collectionID, productID,
		).Scan(&exists)
		if err != nil {
			return apperrors.ErrInternal.Wrap(err)
		}
		if !exists {
			return apperrors.ErrNotFound
		}
	}
	return nil
}

func (r *PostgresCollectionRepository) RemoveProduct(ctx context.Context, tenantID, collectionID, productID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM collection_products
		WHERE tenant_id = $1 AND collection_id = $2 AND product_id = $3`,
		tenantID, collectionID, productID,
	)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}
