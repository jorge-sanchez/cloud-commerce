// Test Budget: 2 distinct behaviors × 2 = 4 max unit tests
// Actual: 4
//
// Behavior 1: New — wraps a payload with UTC timestamp; missing identity fields are rejected
// Behavior 2: New — unserializable payloads are rejected with the sentinel
package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
)

// ---------------------------------------------------------------------------
// Behavior 1: New wraps a payload; identity fields are required
// ---------------------------------------------------------------------------

func TestNew_ValidPayload_ReturnsEnvelopeWithUTCTimestamp(t *testing.T) {
	occurred := time.Date(2026, 7, 15, 12, 0, 0, 0, time.FixedZone("CET", 3600))

	env, err := events.New("tenant-001", "widget-001", "widget.published", occurred, map[string]string{"name": "hero banner"})

	require.NoError(t, err)
	assert.Empty(t, env.ID, "ID is assigned by the database on outbox insert")
	assert.Equal(t, "tenant-001", env.TenantID)
	assert.Equal(t, time.UTC, env.OccurredAt.Location(), "occurred_at must be normalized to UTC")

	var payload map[string]string
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, "hero banner", payload["name"])
}

func TestNew_EmptyTenantID_ReturnsErrMissingField(t *testing.T) {
	_, err := events.New("", "widget-001", "widget.published", time.Now(), nil)

	require.ErrorIs(t, err, events.ErrMissingField)
}

// ---------------------------------------------------------------------------
// Behavior 2: unserializable payloads are rejected
// ---------------------------------------------------------------------------

func TestNew_UnserializablePayload_ReturnsSentinel(t *testing.T) {
	_, err := events.New("tenant-001", "widget-001", "widget.published", time.Now(), func() {})

	require.ErrorIs(t, err, events.ErrUnserializablePayload)
}

func TestNew_NilPayload_IsAllowed(t *testing.T) {
	env, err := events.New("tenant-001", "widget-001", "widget.published", time.Now(), nil)

	require.NoError(t, err)
	assert.Equal(t, "null", string(env.Payload))
}
