// Package cors is the shared CORS middleware (ADR-007): the admin SPA calls
// service APIs cross-origin with a Bearer token. Origins are an explicit
// allowlist — never "*" — configured per deployment.
package cors

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Middleware allows cross-origin requests from the given comma-separated
// origins. An empty allowlist disables CORS entirely (same-origin only),
// which is the safe default for services without a browser client.
func Middleware(allowedOrigins string) gin.HandlerFunc {
	allowed := make(map[string]bool)
	for _, o := range strings.Split(allowedOrigins, ",") {
		if o = strings.TrimSpace(strings.TrimSuffix(o, "/")); o != "" {
			allowed[o] = true
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" || !allowed[origin] {
			// Not a cross-origin request we allow: preflights get an
			// explicit refusal; other requests proceed without CORS
			// headers and the browser blocks the response.
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Vary", "Origin")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		h.Set("Access-Control-Max-Age", "3600")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// Public allows any origin for credential-less public read surfaces
// (storefront browsing). Only safe on routes that require no Authorization.
func Public() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type")
		h.Set("Access-Control-Max-Age", "3600")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
