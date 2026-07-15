package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/services/notifications/internal/service"
)

// PushTokenValidator validates the Google-signed OIDC token on a Pub/Sub
// push request and returns the caller's service-account email. Implemented
// by idtokenValidator in cmd/main.go (google.golang.org/api/idtoken).
type PushTokenValidator interface {
	Validate(ctx context.Context, token, audience string) (email string, err error)
}

// PubSubHandler receives Pub/Sub push deliveries (ADR-002 amendment: Pub/Sub
// is the relay transport). Delivery is at-least-once — the service layer is
// idempotent. A non-2xx response makes Pub/Sub redeliver.
type PubSubHandler struct {
	svc       service.NotificationService // required
	validator PushTokenValidator          // required
	audience  string                      // required — this endpoint's URL
	pusherSA  string                      // required — only this SA may push
}

// NewPubSubHandler constructs the push handler. All parameters are
// required; an empty pusherSA fails closed.
func NewPubSubHandler(svc service.NotificationService, validator PushTokenValidator, audience, pusherSA string) *PubSubHandler {
	return &PubSubHandler{svc: svc, validator: validator, audience: audience, pusherSA: pusherSA}
}

// RegisterRoutes mounts the internal push route.
func (h *PubSubHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/events/pubsub", h.Receive)
}

// pushRequest is the Pub/Sub push wire format; Data is base64 in JSON,
// which encoding/json decodes into []byte transparently.
type pushRequest struct {
	Message struct {
		Data      []byte `json:"data"`
		MessageID string `json:"messageId"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

func (h *PubSubHandler) Receive(c *gin.Context) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	email, err := h.validator.Validate(c.Request.Context(), token, h.audience)
	if err != nil || h.pusherSA == "" || email != h.pusherSA {
		apperrors.RespondError(c, apperrors.ErrUnauthorized)
		return
	}

	var req pushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Malformed push bodies can never succeed — ack (2xx) instead of
		// making Pub/Sub redeliver them forever.
		c.Status(http.StatusNoContent)
		return
	}
	var env events.Envelope
	if err := json.Unmarshal(req.Message.Data, &env); err != nil {
		c.Status(http.StatusNoContent)
		return
	}

	if err := h.svc.ProcessEvent(c.Request.Context(), env); err != nil {
		// Real processing failure: non-2xx so Pub/Sub retries.
		apperrors.RespondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
