package handler

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
)

// OutboxDrainer is the port to the relay's single drain pass.
// Implemented by producer.Relay.
type OutboxDrainer interface {
	DrainOnce(ctx context.Context) (int, error)
}

// OutboxHandler exposes the internal outbox drain endpoint. On Cloud Run the
// relay cannot run as a background goroutine (CPU is throttled outside
// requests, ADR-003), so Cloud Scheduler invokes this endpoint on a fixed
// cadence instead.
type OutboxHandler struct {
	drainer OutboxDrainer // required
	token   string        // required — requests must present it as a Bearer token
}

// NewOutboxHandler constructs the handler. token guards the endpoint: pair
// it with a Cloud Scheduler job sending "Authorization: Bearer <token>".
func NewOutboxHandler(drainer OutboxDrainer, token string) *OutboxHandler {
	return &OutboxHandler{drainer: drainer, token: token}
}

// RegisterRoutes mounts the internal outbox routes.
func (h *OutboxHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/outbox/drain", h.Drain)
}

func (h *OutboxHandler) Drain(c *gin.Context) {
	presented := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if h.token == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(h.token)) != 1 {
		apperrors.RespondError(c, apperrors.ErrUnauthorized)
		return
	}

	delivered, err := h.drainer.DrainOnce(c.Request.Context())
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, DrainOutboxResponse{Delivered: delivered})
}
