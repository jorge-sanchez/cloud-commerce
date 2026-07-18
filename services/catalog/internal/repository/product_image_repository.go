package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

const imageColumns = `id, product_id, tenant_id, variant_id, storage_key, alt_text, position, content_type, byte_size, width, height, created_at`

// AttachImageToProduct appends a finalized upload to the product's gallery.
func (r *PostgresProductRepository) AttachImageToProduct(ctx context.Context, tenantID, productID string, draft domain.ImageDraft) (*domain.Product, error) {
	return r.mutateImages(ctx, tenantID, productID, func(p *domain.Product) error {
		if _, err := p.AttachImage(draft); err != nil {
			return apperrors.ErrValidation.Wrap(err)
		}
		return nil
	})
}

// ReorderProductImages reorders the gallery to match orderedIDs.
func (r *PostgresProductRepository) ReorderProductImages(ctx context.Context, tenantID, productID string, orderedIDs []string) (*domain.Product, error) {
	return r.mutateImages(ctx, tenantID, productID, func(p *domain.Product) error {
		if err := p.ReorderImages(orderedIDs); err != nil {
			return apperrors.ErrValidation.Wrap(err)
		}
		return nil
	})
}

// RemoveProductImage removes one image from the gallery.
func (r *PostgresProductRepository) RemoveProductImage(ctx context.Context, tenantID, productID, imageID string) (*domain.Product, error) {
	return r.mutateImages(ctx, tenantID, productID, func(p *domain.Product) error {
		if err := p.RemoveImage(imageID); err != nil {
			if errors.Is(err, domain.ErrImageNotFound) {
				return apperrors.ErrNotFound
			}
			return apperrors.ErrValidation.Wrap(err)
		}
		return nil
	})
}

// SetProductImageAlt updates one image's alt text.
func (r *PostgresProductRepository) SetProductImageAlt(ctx context.Context, tenantID, productID, imageID, alt string) (*domain.Product, error) {
	return r.mutateImages(ctx, tenantID, productID, func(p *domain.Product) error {
		if err := p.SetImageAlt(imageID, alt); err != nil {
			if errors.Is(err, domain.ErrImageNotFound) {
				return apperrors.ErrNotFound
			}
			return apperrors.ErrValidation.Wrap(err)
		}
		return nil
	})
}

// mutateImages loads the product and its gallery FOR UPDATE, lets the aggregate
// decide the change, reconciles the persisted collection against the entity's
// post-state, and records a product_media_updated event — all in one
// transaction (ADR-002). Image IDs are preserved across updates so the admin
// and storefront can key on them.
func (r *PostgresProductRepository) mutateImages(ctx context.Context, tenantID, productID string, mutate func(*domain.Product) error) (*domain.Product, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	product, err := scanProduct(tx.QueryRowContext(ctx,
		`SELECT `+productColumns+` FROM products WHERE tenant_id = $1 AND id = $2 FOR UPDATE`,
		tenantID, productID,
	))
	if err != nil {
		return nil, err
	}
	if err := r.loadImagesTx(ctx, tx, tenantID, product); err != nil {
		return nil, err
	}

	if err := mutate(product); err != nil { // aggregate decides
		return nil, err
	}

	// Delete rows the entity dropped: any persisted image whose ID is not in
	// the entity's retained (already-persisted) set.
	retained := make([]string, 0, len(product.Images))
	for _, img := range product.Images {
		if img.ID != "" {
			retained = append(retained, img.ID)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM product_images WHERE tenant_id = $1 AND product_id = $2 AND id <> ALL($3)`,
		tenantID, productID, pq.Array(retained),
	); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	// Insert new images, update positions/alt of existing ones.
	for _, img := range product.Images {
		if img.ID == "" {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO product_images
					(product_id, tenant_id, storage_key, alt_text, position, content_type, byte_size, width, height)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				RETURNING id, created_at`,
				productID, tenantID, img.StorageKey, img.AltText, img.Position,
				img.ContentType, img.ByteSize, img.Width, img.Height,
			).Scan(&img.ID, &img.CreatedAt)
			if err != nil {
				var pqErr *pq.Error
				if errors.As(err, &pqErr) && string(pqErr.Code) == uniqueViolation {
					return nil, apperrors.ErrConflict.Wrap(err)
				}
				return nil, apperrors.ErrInternal.Wrap(err)
			}
			img.ProductID = productID
			img.TenantID = tenantID
		} else {
			if _, err := tx.ExecContext(ctx, `
				UPDATE product_images SET position = $1, alt_text = $2
				WHERE tenant_id = $3 AND id = $4`,
				img.Position, img.AltText, tenantID, img.ID,
			); err != nil {
				return nil, apperrors.ErrInternal.Wrap(err)
			}
		}
	}

	if r.events != nil {
		event := domain.NewProductMediaUpdatedEvent(product, time.Now().UTC())
		env, err := events.New(tenantID, product.ID, domain.ProductMediaUpdatedEventType, event.UpdatedAt, event)
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

// loadImages attaches images to the given products with one query, ordered by
// position (0 = primary first).
func (r *PostgresProductRepository) loadImages(ctx context.Context, tenantID string, products []*domain.Product) error {
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
		SELECT `+imageColumns+` FROM product_images
		WHERE tenant_id = $1 AND product_id = ANY($2)
		ORDER BY product_id, position`,
		tenantID, pq.Array(ids),
	)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		img, err := scanImageRows(rows)
		if err != nil {
			return err
		}
		byID[img.ProductID].Images = append(byID[img.ProductID].Images, img)
	}
	if err := rows.Err(); err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}

func (r *PostgresProductRepository) loadImagesTx(ctx context.Context, tx *sql.Tx, tenantID string, p *domain.Product) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT `+imageColumns+` FROM product_images
		WHERE tenant_id = $1 AND product_id = $2
		ORDER BY position`,
		tenantID, p.ID,
	)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		img, err := scanImageRows(rows)
		if err != nil {
			return err
		}
		p.Images = append(p.Images, img)
	}
	if err := rows.Err(); err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}

func scanImageRows(rows *sql.Rows) (*domain.Image, error) {
	var img domain.Image
	var variantID sql.NullString
	if err := rows.Scan(
		&img.ID, &img.ProductID, &img.TenantID, &variantID, &img.StorageKey,
		&img.AltText, &img.Position, &img.ContentType, &img.ByteSize,
		&img.Width, &img.Height, &img.CreatedAt,
	); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	img.VariantID = variantID.String
	return &img, nil
}
