// Package service holds the notification consumer: orchestration only.
// This service has no aggregate — it turns order events into emails, with
// the sent log as its only state (dedupe + audit).
package service

import (
	"context"
	"encoding/json"
	"fmt"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
)

// Consumed cross-service event types (orders service contract).
const (
	OrderPaidType      = "orders.order_paid"
	OrderFulfilledType = "orders.order_fulfilled"
)

// EmailSender is the provider port (Resend today; the port outlives it).
type EmailSender interface {
	Send(ctx context.Context, to, subject, html string) error
}

// SentLog is the persistence port: at-least-once dedupe plus audit trail.
type SentLog interface {
	// AlreadySent reports whether this envelope was handled.
	AlreadySent(ctx context.Context, eventID string) (bool, error)
	// Record logs a delivered email (unique on event ID).
	Record(ctx context.Context, eventID, tenantID, orderID, kind, recipient, subject string) error
}

// NotificationService consumes order events and sends buyer email.
type NotificationService interface {
	// ProcessEvent handles one envelope. Unknown types and events without
	// a recipient are acked untouched; replays are no-ops.
	ProcessEvent(ctx context.Context, env events.Envelope) error
}

type notificationService struct {
	log    SentLog     // required
	sender EmailSender // required
}

// Option configures optional dependencies on the notification service.
type Option func(*notificationService)

// NewNotificationService constructs the consumer.
func NewNotificationService(log SentLog, sender EmailSender, opts ...Option) NotificationService {
	s := &notificationService{log: log, sender: sender}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type paidPayload struct {
	OrderID    string `json:"order_id"`
	Number     int64  `json:"number"`
	Email      string `json:"email"`
	TotalCents int64  `json:"total_cents"`
	Currency   string `json:"currency"`
}

type fulfilledPayload struct {
	OrderID        string `json:"order_id"`
	Number         int64  `json:"number"`
	Email          string `json:"email"`
	TrackingNumber string `json:"tracking_number"`
	Carrier        string `json:"carrier"`
}

func (s *notificationService) ProcessEvent(ctx context.Context, env events.Envelope) error {
	var kind, recipient, subject, html string

	switch env.Type {
	case OrderPaidType:
		var p paidPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return apperrors.ErrValidation.Wrap(err)
		}
		if p.Email == "" {
			return nil // pre-enrichment event — nothing to send
		}
		kind, recipient = "order_paid", p.Email
		subject = fmt.Sprintf("Payment received — order #%d", p.Number)
		html = fmt.Sprintf("<p>Thanks! We received your payment of <b>%s %.2f</b> for order <b>#%d</b>.</p><p>We'll let you know when it ships.</p>",
			p.Currency, float64(p.TotalCents)/100, p.Number)
	case OrderFulfilledType:
		var p fulfilledPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return apperrors.ErrValidation.Wrap(err)
		}
		if p.Email == "" {
			return nil
		}
		kind, recipient = "order_fulfilled", p.Email
		subject = fmt.Sprintf("Your order #%d is on its way", p.Number)
		tracking := ""
		if p.TrackingNumber != "" {
			tracking = fmt.Sprintf("<p>Tracking: <b>%s</b> (%s)</p>", p.TrackingNumber, p.Carrier)
		}
		html = fmt.Sprintf("<p>Order <b>#%d</b> has shipped.</p>%s", p.Number, tracking)
	default:
		return nil // not ours — ack
	}

	sent, err := s.log.AlreadySent(ctx, env.ID)
	if err != nil {
		return err
	}
	if sent {
		return nil // replay
	}
	if err := s.sender.Send(ctx, recipient, subject, html); err != nil {
		return err // non-2xx upstream: let the broker retry
	}
	return s.log.Record(ctx, env.ID, env.TenantID, env.AggregateID, kind, recipient, subject)
}
