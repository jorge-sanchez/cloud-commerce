// Package producer holds this service's event deliverers (the generic
// outbox machinery lives in pkg/outbox). PubSubDeliverer is the relay
// transport chosen in the ADR-002 amendment: envelopes drained from the
// outbox are published to a Pub/Sub topic; push subscriptions fan them out
// to consumers (inventory, and later others).
package producer

import (
	"context"
	"encoding/json"

	"cloud.google.com/go/pubsub/v2"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/pkg/outbox"
)

// PubSubDeliverer publishes envelopes to one Pub/Sub topic.
type PubSubDeliverer struct {
	publisher *pubsub.Publisher
}

var _ outbox.Deliverer = (*PubSubDeliverer)(nil)

// NewPubSubDeliverer wires the deliverer to a topic. The caller owns the
// client's lifecycle.
func NewPubSubDeliverer(client *pubsub.Client, topicID string) *PubSubDeliverer {
	return &PubSubDeliverer{publisher: client.Publisher(topicID)}
}

// Deliver publishes the envelope as JSON and waits for the server ack — the
// relay must not mark a row delivered before the broker owns the message.
func (d *PubSubDeliverer) Deliver(ctx context.Context, env events.Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	result := d.publisher.Publish(ctx, &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"event_id":   env.ID,
			"event_type": env.Type,
			"tenant_id":  env.TenantID,
		},
	})
	if _, err := result.Get(ctx); err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}
