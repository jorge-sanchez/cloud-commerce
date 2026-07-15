package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
)

// Deliverer is the transport port the relay hands envelopes to. The initial
// implementation is Postgres-backed delivery (ADR-002); swapping in a broker
// later replaces only this interface's implementation.
type Deliverer interface {
	Deliver(ctx context.Context, env events.Envelope) error
}

// DelivererFunc adapts a function to the Deliverer interface.
type DelivererFunc func(ctx context.Context, env events.Envelope) error

func (f DelivererFunc) Deliver(ctx context.Context, env events.Envelope) error {
	return f(ctx, env)
}

// Relay is the recovery process CLAUDE.md promises: it drains undelivered
// outbox rows in insertion order and delivers them at-least-once. Consumers
// must therefore be idempotent (dedupe on envelope ID).
type Relay struct {
	db        *sql.DB   // required
	deliverer Deliverer // required
	interval  time.Duration
	batchSize int
	log       *zap.Logger // may be nop
}

// Option configures optional relay behavior.
type Option func(*Relay)

// WithPollInterval overrides how often the relay scans for undelivered rows.
func WithPollInterval(d time.Duration) Option {
	return func(r *Relay) { r.interval = d }
}

// WithBatchSize overrides how many rows one drain pass claims.
func WithBatchSize(n int) Option {
	return func(r *Relay) { r.batchSize = n }
}

// WithLogger wires a logger for per-pass delivery errors.
func WithLogger(log *zap.Logger) Option {
	return func(r *Relay) { r.log = log }
}

// NewRelay constructs a relay draining db through deliverer.
func NewRelay(db *sql.DB, deliverer Deliverer, opts ...Option) *Relay {
	r := &Relay{
		db:        db,
		deliverer: deliverer,
		interval:  time.Second,
		batchSize: 100,
		log:       zap.NewNop(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run drains the outbox until ctx is cancelled. Delivery errors are logged
// and retried on the next pass — they never stop the relay.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.DrainOnce(ctx); err != nil && ctx.Err() == nil {
				r.log.Warn("outbox drain pass failed; will retry", zap.Error(err))
			}
		}
	}
}

// DrainOnce claims one batch of undelivered rows (FOR UPDATE SKIP LOCKED, so
// concurrent relays never double-claim), delivers them in insertion order,
// and marks the delivered prefix. A delivery failure stops the batch — the
// failed row and everything after it stay undelivered to preserve per-outbox
// ordering. Returns how many envelopes were delivered.
func (r *Relay) DrainOnce(ctx context.Context) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT position, id, tenant_id, aggregate_id, event_type, occurred_at, payload
		FROM outbox WHERE delivered_at IS NULL
		ORDER BY position
		LIMIT $1
		FOR UPDATE SKIP LOCKED`,
		r.batchSize,
	)
	if err != nil {
		return 0, apperrors.ErrInternal.Wrap(err)
	}

	type claimed struct {
		position int64
		env      events.Envelope
	}
	batch := make([]claimed, 0, r.batchSize)
	for rows.Next() {
		var c claimed
		var payload []byte
		if err := rows.Scan(&c.position, &c.env.ID, &c.env.TenantID, &c.env.AggregateID, &c.env.Type, &c.env.OccurredAt, &payload); err != nil {
			_ = rows.Close()
			return 0, apperrors.ErrInternal.Wrap(err)
		}
		c.env.Payload = json.RawMessage(payload)
		batch = append(batch, c)
	}
	if err := rows.Close(); err != nil {
		return 0, apperrors.ErrInternal.Wrap(err)
	}

	delivered := 0
	var deliveryErr error
	for _, c := range batch {
		if err := r.deliverer.Deliver(ctx, c.env); err != nil {
			deliveryErr = err
			break
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE outbox SET delivered_at = NOW() WHERE position = $1`, c.position,
		); err != nil {
			return delivered, apperrors.ErrInternal.Wrap(err)
		}
		delivered++
	}

	if err := tx.Commit(); err != nil {
		return 0, apperrors.ErrInternal.Wrap(err)
	}
	if deliveryErr != nil {
		return delivered, apperrors.ErrInternal.Wrap(deliveryErr)
	}
	return delivered, nil
}
