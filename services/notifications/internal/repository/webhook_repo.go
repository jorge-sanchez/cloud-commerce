package repository

import (
	"context"
	"database/sql"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/notifications/internal/service"
)

// PostgresWebhookRepo implements service.WebhookRepo.
type PostgresWebhookRepo struct {
	db *sql.DB // required
}

// NewPostgresWebhookRepo wires the repo to an open *sql.DB.
func NewPostgresWebhookRepo(db *sql.DB) *PostgresWebhookRepo {
	return &PostgresWebhookRepo{db: db}
}

func (r *PostgresWebhookRepo) SaveEndpoint(ctx context.Context, e *service.WebhookEndpoint) (*service.WebhookEndpoint, error) {
	stored := *e
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO webhook_endpoints (tenant_id, url, secret, active)
		VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		e.TenantID, e.URL, e.Secret, e.Active,
	).Scan(&stored.ID, &stored.CreatedAt)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return &stored, nil
}

func (r *PostgresWebhookRepo) list(ctx context.Context, tenantID string, activeOnly bool) ([]*service.WebhookEndpoint, error) {
	q := `SELECT id, tenant_id, url, secret, active, created_at
		FROM webhook_endpoints WHERE tenant_id = $1`
	if activeOnly {
		q += ` AND active`
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]*service.WebhookEndpoint, 0, 4)
	for rows.Next() {
		var e service.WebhookEndpoint
		if err := rows.Scan(&e.ID, &e.TenantID, &e.URL, &e.Secret, &e.Active, &e.CreatedAt); err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return out, nil
}

func (r *PostgresWebhookRepo) ListEndpoints(ctx context.Context, tenantID string) ([]*service.WebhookEndpoint, error) {
	return r.list(ctx, tenantID, false)
}

func (r *PostgresWebhookRepo) ListActiveEndpoints(ctx context.Context, tenantID string) ([]*service.WebhookEndpoint, error) {
	return r.list(ctx, tenantID, true)
}

func (r *PostgresWebhookRepo) DeleteEndpoint(ctx context.Context, tenantID, id string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM webhook_endpoints WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return apperrors.ErrInternal.Wrap(err)
	} else if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *PostgresWebhookRepo) AlreadyDelivered(ctx context.Context, eventID, endpointID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM webhook_deliveries WHERE event_id = $1 AND endpoint_id = $2)`,
		eventID, endpointID,
	).Scan(&exists)
	if err != nil {
		return false, apperrors.ErrInternal.Wrap(err)
	}
	return exists, nil
}

func (r *PostgresWebhookRepo) MarkDelivered(ctx context.Context, eventID, endpointID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO webhook_deliveries (event_id, endpoint_id)
		VALUES ($1, $2) ON CONFLICT DO NOTHING`, eventID, endpointID)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}
