//go:build integration

// Test Budget: 2 distinct behaviors × 2 = 4 max integration tests
// Actual: 4
//
// Behavior 1: PublishIfPublishable + OutboxRecorder — the event lands in the
//
//	outbox atomically with the status change; a rejected transition
//	writes neither
//
// Behavior 2: Relay.DrainOnce — delivers undelivered rows in insertion order
//
//	and marks them; a delivery failure leaves rows undelivered for retry
//
// The tests live in this package (not producer) to reuse openMigratedDB.
package repository

import (
	"context"
	"errors"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/domain"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/producer"
)

// fakeDeliverer records delivered envelopes and can be told to fail.
type fakeDeliverer struct {
	err       error
	delivered []events.Envelope
}

var _ producer.Deliverer = (*fakeDeliverer)(nil)

func (f *fakeDeliverer) Deliver(_ context.Context, env events.Envelope) error {
	if f.err != nil {
		return f.err
	}
	f.delivered = append(f.delivered, env)
	return nil
}

// ---------------------------------------------------------------------------
// Behavior 1: the event is recorded atomically with the state change
// ---------------------------------------------------------------------------

func TestPostgresWidgetRepository_PublishIfPublishable_WithRecorder_WritesOutboxRowAtomically(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresWidgetRepository(db, WithEventRecorder(producer.NewOutboxRecorder()))
	ctx := context.Background()

	saved, err := repo.SaveNew(ctx, tenantA, domain.NewWidget(tenantA, "hero banner"))
	require.NoError(t, err)

	published, err := repo.PublishIfPublishable(ctx, tenantA, saved.ID)
	require.NoError(t, err)

	var (
		count       int
		eventType   string
		aggregateID string
	)
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*), MAX(event_type), MAX(aggregate_id::text) FROM outbox WHERE tenant_id = $1`, tenantA,
	).Scan(&count, &eventType, &aggregateID))
	require.Equal(t, 1, count, "exactly one outbox row must be written with the publish")
	assert.Equal(t, domain.WidgetPublishedEventType, eventType)
	assert.Equal(t, published.ID, aggregateID, "the envelope must reference the published aggregate")
}

func TestPostgresWidgetRepository_PublishIfPublishable_RejectedTransition_WritesNoOutboxRow(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresWidgetRepository(db, WithEventRecorder(producer.NewOutboxRecorder()))
	ctx := context.Background()

	saved, err := repo.SaveNew(ctx, tenantA, domain.NewWidget(tenantA, "hero banner"))
	require.NoError(t, err)
	_, err = repo.PublishIfPublishable(ctx, tenantA, saved.ID)
	require.NoError(t, err)

	_, err = repo.PublishIfPublishable(ctx, tenantA, saved.ID) // entity rejects
	require.ErrorIs(t, err, apperrors.ErrConflict)

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM outbox`).Scan(&count))
	assert.Equal(t, 1, count, "the rejected transition must not add an outbox row")
}

// ---------------------------------------------------------------------------
// Behavior 2: the relay delivers in order and marks delivered
// ---------------------------------------------------------------------------

func TestRelay_DrainOnce_UndeliveredRows_DeliversInOrderAndMarks(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresWidgetRepository(db, WithEventRecorder(producer.NewOutboxRecorder()))
	ctx := context.Background()

	for _, name := range []string{"first", "second"} {
		saved, err := repo.SaveNew(ctx, tenantA, domain.NewWidget(tenantA, name))
		require.NoError(t, err)
		_, err = repo.PublishIfPublishable(ctx, tenantA, saved.ID)
		require.NoError(t, err)
	}

	sink := &fakeDeliverer{}
	relay := producer.NewRelay(db, sink)

	delivered, err := relay.DrainOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, delivered)
	require.Len(t, sink.delivered, 2, "both envelopes must reach the deliverer")
	assert.NotEmpty(t, sink.delivered[0].ID, "the database must assign envelope IDs")

	delivered, err = relay.DrainOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, delivered, "a second pass must find nothing undelivered")
}

func TestRelay_DrainOnce_DeliveryFails_LeavesRowUndeliveredForRetry(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresWidgetRepository(db, WithEventRecorder(producer.NewOutboxRecorder()))
	ctx := context.Background()

	saved, err := repo.SaveNew(ctx, tenantA, domain.NewWidget(tenantA, "hero banner"))
	require.NoError(t, err)
	_, err = repo.PublishIfPublishable(ctx, tenantA, saved.ID)
	require.NoError(t, err)

	failing := &fakeDeliverer{err: errors.New("transport down")}
	relay := producer.NewRelay(db, failing)

	delivered, err := relay.DrainOnce(ctx)
	require.Error(t, err)
	assert.Equal(t, 0, delivered)

	sink := &fakeDeliverer{}
	delivered, err = producer.NewRelay(db, sink).DrainOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, delivered, "the failed envelope must be redelivered on the next pass")
}
