package auth

import (
	"github.com/gin-gonic/gin"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
)

const (
	ctxTenantID = "auth.tenant_id"
	ctxUserID   = "auth.user_id"
	ctxEmail    = "auth.email"
)

// Middleware rejects requests without a valid platform token and injects
// the verified identity into the Gin context.
func Middleware(v *Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := BearerToken(c.GetHeader("Authorization"))
		if !ok {
			apperrors.RespondError(c, apperrors.ErrUnauthorized)
			c.Abort()
			return
		}
		claims, err := v.Verify(token)
		if err != nil {
			apperrors.RespondError(c, apperrors.ErrUnauthorized.Wrap(err))
			c.Abort()
			return
		}
		c.Set(ctxTenantID, claims.TenantID)
		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxEmail, claims.Email)
		c.Next()
	}
}

// TenantID returns the verified tenant for this request. Empty only when
// the middleware did not run — routes reading it must be mounted behind
// Middleware.
func TenantID(c *gin.Context) string { return c.GetString(ctxTenantID) }

// UserID returns the verified user for this request.
func UserID(c *gin.Context) string { return c.GetString(ctxUserID) }

// Email returns the verified email for this request.
func Email(c *gin.Context) string { return c.GetString(ctxEmail) }
