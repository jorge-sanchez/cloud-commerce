// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 6
//
// Behavior 1: order_paid — sends a receipt and records it; replays send nothing
// Behavior 2: order_fulfilled — sends shipping email with tracking
// Behavior 3: unknown types and events without a recipient are acked untouched
package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
)

var _ SentLog = (*fakeLog)(nil)

type fakeLog struct {
	sent     map[string]bool
	recorded []string
}

func (f *fakeLog) AlreadySent(_ context.Context, eventID string) (bool, error) {
	return f.sent[eventID], nil
}

func (f *fakeLog) Record(_ context.Context, eventID, _, _, _, _, _ string) error {
	f.recorded = append(f.recorded, eventID)
	return nil
}

var _ EmailSender = (*fakeSender)(nil)

type fakeSender struct {
	subjects []string
	bodies   []string
	to       []string
}

func (f *fakeSender) Send(_ context.Context, to, subject, html string) error {
	f.to = append(f.to, to)
	f.subjects = append(f.subjects, subject)
	f.bodies = append(f.bodies, html)
	return nil
}

func envelope(t *testing.T, evType string, payload map[string]any) events.Envelope {
	t.Helper()
	env, err := events.New("tenant-001", "order-001", evType, time.Now(), payload)
	require.NoError(t, err)
	env.ID = "env-001"
	return env
}

// ---------------------------------------------------------------------------
// Behavior 1: order_paid receipt with dedupe
// ---------------------------------------------------------------------------

func TestNotificationService_ProcessEvent_OrderPaid_SendsReceiptAndRecords(t *testing.T) {
	log := &fakeLog{sent: map[string]bool{}}
	sender := &fakeSender{}
	svc := NewNotificationService(log, sender)

	err := svc.ProcessEvent(context.Background(), envelope(t, OrderPaidType, map[string]any{
		"order_id": "order-001", "number": 7, "email": "buyer@example.test",
		"total_cents": 5990, "currency": "PEN",
	}))

	require.NoError(t, err)
	require.Len(t, sender.to, 1, "exactly one receipt must be sent")
	assert.Equal(t, "buyer@example.test", sender.to[0])
	assert.Contains(t, sender.subjects[0], "#7")
	require.Len(t, log.recorded, 1, "the send must be recorded for dedupe")
}

func TestNotificationService_ProcessEvent_Replay_SendsNothing(t *testing.T) {
	log := &fakeLog{sent: map[string]bool{"env-001": true}}
	sender := &fakeSender{}
	svc := NewNotificationService(log, sender)

	err := svc.ProcessEvent(context.Background(), envelope(t, OrderPaidType, map[string]any{
		"number": 7, "email": "buyer@example.test",
	}))

	require.NoError(t, err)
	require.Len(t, sender.to, 0, "a replayed envelope must not email twice")
}

// ---------------------------------------------------------------------------
// Behavior 2: order_fulfilled shipping email
// ---------------------------------------------------------------------------

func TestNotificationService_ProcessEvent_Fulfilled_SendsTracking(t *testing.T) {
	log := &fakeLog{sent: map[string]bool{}}
	sender := &fakeSender{}
	svc := NewNotificationService(log, sender)

	err := svc.ProcessEvent(context.Background(), envelope(t, OrderFulfilledType, map[string]any{
		"number": 7, "email": "buyer@example.test",
		"tracking_number": "TRK-9", "carrier": "olva",
	}))

	require.NoError(t, err)
	require.Len(t, sender.bodies, 1, "exactly one shipping email must be sent")
	assert.Contains(t, sender.bodies[0], "TRK-9")
}

func TestNotificationService_ProcessEvent_FulfilledWithoutEmail_AcksUntouched(t *testing.T) {
	log := &fakeLog{sent: map[string]bool{}}
	sender := &fakeSender{}
	svc := NewNotificationService(log, sender)

	err := svc.ProcessEvent(context.Background(), envelope(t, OrderFulfilledType, map[string]any{
		"number": 7,
	}))

	require.NoError(t, err, "legacy events without email must be acked, not retried")
	require.Len(t, sender.to, 0)
}

// ---------------------------------------------------------------------------
// Behavior 3: foreign events
// ---------------------------------------------------------------------------

func TestNotificationService_ProcessEvent_UnknownType_AcksUntouched(t *testing.T) {
	log := &fakeLog{sent: map[string]bool{}}
	sender := &fakeSender{}
	svc := NewNotificationService(log, sender)

	err := svc.ProcessEvent(context.Background(), envelope(t, "orders.order_placed", map[string]any{
		"email": "buyer@example.test",
	}))

	require.NoError(t, err)
	require.Len(t, sender.to, 0, "unconsumed types must not email")
}

func TestNotificationService_ProcessEvent_MalformedPayload_ReturnsValidation(t *testing.T) {
	svc := NewNotificationService(&fakeLog{sent: map[string]bool{}}, &fakeSender{})
	env := envelope(t, OrderPaidType, nil)
	env.Payload = []byte(`"not-an-object"`)

	err := svc.ProcessEvent(context.Background(), env)

	require.Error(t, err)
}
