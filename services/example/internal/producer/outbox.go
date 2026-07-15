// Package producer holds the event-publishing adapters (ADR-002): the
// OutboxRecorder writes envelopes to the outbox table inside the caller's
// transaction, and the Relay drains undelivered rows and owns delivery
// retries. Domain and service code never see either — repositories record,
// the relay delivers.
package producer

import (
	"context"
	"database/sql"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
)

// OutboxRecorder writes event envelopes to the outbox table. Record must be
// called with the same transaction that persists the state change the event
// describes — that atomicity is the entire point of the outbox.
type OutboxRecorder struct{}

// NewOutboxRecorder constructs the recorder. It is stateless: the transaction
// arrives with every Record call.
func NewOutboxRecorder() *OutboxRecorder {
	return &OutboxRecorder{}
}

// Record inserts the envelope into the outbox inside tx. The database assigns
// the envelope ID and insertion position.
func (r *OutboxRecorder) Record(ctx context.Context, tx *sql.Tx, env events.Envelope) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO outbox (tenant_id, aggregate_id, event_type, occurred_at, payload)
		VALUES ($1, $2, $3, $4, $5)`,
		env.TenantID, env.AggregateID, env.Type, env.OccurredAt, []byte(env.Payload),
	)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}
