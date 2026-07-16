package outbox

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
)

// Drainer is the port to the relay's single drain pass, implemented by Relay.
type Drainer interface {
	DrainOnce(ctx context.Context) (int, error)
}

// DrainerFunc adapts a function to the Drainer interface (the sweep
// endpoints reuse the drain contract).
type DrainerFunc func(ctx context.Context) (int, error)

// DrainOnce implements Drainer.
func (f DrainerFunc) DrainOnce(ctx context.Context) (int, error) { return f(ctx) }

// DrainResponse reports one drain pass (internal endpoint wire shape).
type DrainResponse struct {
	Delivered int `json:"delivered"`
}

// DrainHandler serves the internal outbox drain endpoint. On Cloud Run the
// relay cannot run as a background goroutine (CPU is throttled outside
// requests, ADR-003), so Cloud Scheduler invokes this endpoint instead,
// authenticated with a bearer token. An empty token fails closed.
func DrainHandler(drainer Drainer, token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		presented := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
			apperrors.RespondError(c, apperrors.ErrUnauthorized)
			return
		}

		delivered, err := drainer.DrainOnce(c.Request.Context())
		if err != nil {
			apperrors.RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, DrainResponse{Delivered: delivered})
	}
}
