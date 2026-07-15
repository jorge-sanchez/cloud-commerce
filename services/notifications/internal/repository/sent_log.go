// Package repository holds the persistence adapter for the sent log.
package repository

import (
	"context"
	"database/sql"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
)

// PostgresSentLog implements service.SentLog on PostgreSQL.
type PostgresSentLog struct {
	db *sql.DB // required
}

// NewPostgresSentLog wires the log to an open *sql.DB.
func NewPostgresSentLog(db *sql.DB) *PostgresSentLog {
	return &PostgresSentLog{db: db}
}

func (r *PostgresSentLog) AlreadySent(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM notifications WHERE event_id = $1)`, eventID,
	).Scan(&exists)
	if err != nil {
		return false, apperrors.ErrInternal.Wrap(err)
	}
	return exists, nil
}

func (r *PostgresSentLog) Record(ctx context.Context, eventID, tenantID, orderID, kind, recipient, subject string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO notifications (event_id, tenant_id, order_id, kind, recipient, subject)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id) DO NOTHING`,
		eventID, tenantID, orderID, kind, recipient, subject,
	)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}
