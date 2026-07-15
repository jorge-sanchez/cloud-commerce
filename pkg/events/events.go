// Package events defines the shared event envelope every service wraps its
// domain events in before writing them to its transactional outbox (ADR-002).
// The envelope is the wire contract between producers, the relay, and
// consumers — domain event payloads stay service-owned, the envelope does not.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for envelope construction failures.
var (
	ErrUnserializablePayload = errors.New("event payload cannot be serialized to JSON")
	ErrMissingField          = errors.New("event envelope field must not be empty")
)

// Envelope wraps a domain event for the outbox. ID is assigned by the
// database when the envelope is written to the outbox table — the same
// convention as entity IDs, which repositories assign on save.
type Envelope struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	AggregateID string          `json:"aggregate_id"`
	Type        string          `json:"type"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Payload     json.RawMessage `json:"payload"`
}

// New builds an envelope around a domain event payload. Build payloads from
// the persisted domain object, not from the raw request.
func New(tenantID, aggregateID, eventType string, occurredAt time.Time, payload any) (Envelope, error) {
	if tenantID == "" || aggregateID == "" || eventType == "" {
		return Envelope{}, fmt.Errorf("%w: tenant_id, aggregate_id and type are required", ErrMissingField)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrUnserializablePayload, err)
	}
	return Envelope{
		TenantID:    tenantID,
		AggregateID: aggregateID,
		Type:        eventType,
		OccurredAt:  occurredAt.UTC(),
		Payload:     raw,
	}, nil
}
