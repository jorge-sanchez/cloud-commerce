// Package repository holds the persistence adapters. Repositories load and
// save — the entity decides. No business logic lives here.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/domain"
)

// EventRecorder writes an event envelope inside the caller's transaction —
// the transactional-outbox port (ADR-002). Implemented by outbox.Recorder.
type EventRecorder interface {
	Record(ctx context.Context, tx *sql.Tx, env events.Envelope) error
}

// PostgresWidgetRepository implements domain.WidgetRepository on PostgreSQL.
type PostgresWidgetRepository struct {
	db     *sql.DB       // required
	events EventRecorder // may be nil
}

var _ domain.WidgetRepository = (*PostgresWidgetRepository)(nil)

// Option configures optional dependencies on the repository.
type Option func(*PostgresWidgetRepository)

// WithEventRecorder wires the outbox recorder. Without it, state changes
// persist but no events are recorded.
func WithEventRecorder(rec EventRecorder) Option {
	return func(r *PostgresWidgetRepository) { r.events = rec }
}

// NewPostgresWidgetRepository wires the repository to an open *sql.DB.
func NewPostgresWidgetRepository(db *sql.DB, opts ...Option) *PostgresWidgetRepository {
	r := &PostgresWidgetRepository{db: db}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// widgetRow mirrors the widgets table.
type widgetRow struct {
	ID        string
	TenantID  string
	Name      string
	Status    string
	CreatedAt sql.NullTime
	UpdatedAt sql.NullTime
}

func (r widgetRow) toDomain() *domain.Widget {
	return &domain.Widget{
		ID:        r.ID,
		TenantID:  r.TenantID,
		Name:      r.Name,
		Status:    domain.WidgetStatus(r.Status),
		CreatedAt: r.CreatedAt.Time,
		UpdatedAt: r.UpdatedAt.Time,
	}
}

func (r *PostgresWidgetRepository) SaveNew(ctx context.Context, tenantID string, w *domain.Widget) (*domain.Widget, error) {
	var row widgetRow
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO widgets (tenant_id, name, status)
		VALUES ($1, $2, $3)
		RETURNING id, tenant_id, name, status, created_at, updated_at`,
		tenantID, w.Name, string(w.Status),
	).Scan(&row.ID, &row.TenantID, &row.Name, &row.Status, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return row.toDomain(), nil
}

func (r *PostgresWidgetRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.Widget, error) {
	row, err := r.scanOne(ctx, r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, status, created_at, updated_at
		FROM widgets WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	))
	if err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}

// PublishIfPublishable loads the widget inside a transaction, lets the entity
// decide the transition, and persists what the entity decided.
func (r *PostgresWidgetRepository) PublishIfPublishable(ctx context.Context, tenantID, id string) (*domain.Widget, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	row, err := r.scanOne(ctx, tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, status, created_at, updated_at
		FROM widgets WHERE tenant_id = $1 AND id = $2 FOR UPDATE`,
		tenantID, id,
	))
	if err != nil {
		return nil, err
	}

	widget := row.toDomain()
	if err := widget.Publish(); err != nil { // entity decides
		return nil, apperrors.ErrConflict.Wrap(err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE widgets SET status = $1, updated_at = NOW()
		WHERE tenant_id = $2 AND id = $3`,
		string(widget.Status), tenantID, id,
	); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	// Record the event in the same transaction as the state change (ADR-002):
	// either both commit or neither does. Delivery is the relay's job.
	if r.events != nil {
		event := domain.NewWidgetPublishedEvent(widget, time.Now().UTC())
		env, err := events.New(widget.TenantID, widget.ID, domain.WidgetPublishedEventType, event.PublishedAt, event)
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
	return widget, nil
}

func (r *PostgresWidgetRepository) ListByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Widget, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM widgets WHERE tenant_id = $1`, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, status, created_at, updated_at
		FROM widgets WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		tenantID, pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	widgets := make([]*domain.Widget, 0, pageSize)
	for rows.Next() {
		var row widgetRow
		if err := rows.Scan(&row.ID, &row.TenantID, &row.Name, &row.Status, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, 0, apperrors.ErrInternal.Wrap(err)
		}
		widgets = append(widgets, row.toDomain())
	}
	if err := rows.Err(); err != nil {
		return nil, 0, apperrors.ErrInternal.Wrap(err)
	}
	return widgets, total, nil
}

func (r *PostgresWidgetRepository) scanOne(_ context.Context, qr *sql.Row) (widgetRow, error) {
	var row widgetRow
	err := qr.Scan(&row.ID, &row.TenantID, &row.Name, &row.Status, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return widgetRow{}, apperrors.ErrNotFound
	}
	if err != nil {
		return widgetRow{}, apperrors.ErrInternal.Wrap(err)
	}
	return row, nil
}
