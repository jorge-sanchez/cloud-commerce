package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/service"
)

// StripeWebhookHandler reconciles payments without buyer polling (#25):
// Stripe signs each delivery with the endpoint's signing secret; a verified
// payment_intent.succeeded marks the order paid. Deliveries are
// at-least-once and can race the buyer's own confirm — MarkPaidIfPayable
// makes both paths idempotent.
type StripeWebhookHandler struct {
	payments      service.PaymentService // required
	signingSecret string                 // required — empty fails closed
}

// NewStripeWebhookHandler constructs the webhook handler.
func NewStripeWebhookHandler(payments service.PaymentService, signingSecret string) *StripeWebhookHandler {
	return &StripeWebhookHandler{payments: payments, signingSecret: signingSecret}
}

// RegisterRoutes mounts the internal webhook route.
func (h *StripeWebhookHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/payments/stripe/webhook", h.Receive)
}

func (h *StripeWebhookHandler) Receive(c *gin.Context) {
	payload, err := c.GetRawData()
	if err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	// IgnoreAPIVersionMismatch: the signature is still verified; the
	// account's dashboard API version routinely differs from the SDK's
	// pinned one and must not reject genuine deliveries.
	event, err := webhook.ConstructEventWithOptions(payload, c.GetHeader("Stripe-Signature"), h.signingSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		apperrors.RespondError(c, apperrors.ErrUnauthorized.Wrap(err))
		return
	}

	if event.Type != "payment_intent.succeeded" {
		c.Status(http.StatusNoContent) // not ours — ack so Stripe stops retrying
		return
	}

	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		c.Status(http.StatusNoContent) // malformed payloads can never succeed — ack
		return
	}
	orderID := pi.Metadata["order_id"]
	if orderID == "" {
		c.Status(http.StatusNoContent) // an intent we did not create — ack
		return
	}

	if _, err := h.payments.ReconcilePayment(c.Request.Context(), orderID, pi.ID); err != nil {
		// Already paid (buyer confirm won the race) is success, not retry.
		if errors.Is(err, apperrors.ErrConflict) {
			c.Status(http.StatusNoContent)
			return
		}
		apperrors.RespondError(c, err) // real failure: non-2xx so Stripe retries
		return
	}
	c.Status(http.StatusNoContent)
}
