// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 5
//
// Behavior 1: allowed origins get CORS headers; preflights short-circuit 204
// Behavior 2: unknown origins get no CORS headers; empty allowlist fails closed
// Behavior 3: Public — any origin is allowed on credential-less surfaces
package cors_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/jorge-sanchez/cloud-commerce/pkg/cors"
)

func request(t *testing.T, allowlist, method, origin string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(cors.Middleware(allowlist))
	router.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(method, "/probe", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Behavior 1: allowed origins
// ---------------------------------------------------------------------------

func TestMiddleware_AllowedOrigin_SetsCORSHeaders(t *testing.T) {
	rec := request(t, "https://admin.test, https://other.test", http.MethodGet, "https://admin.test")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "https://admin.test", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestMiddleware_AllowedOriginPreflight_ShortCircuits204(t *testing.T) {
	rec := request(t, "https://admin.test", http.MethodOptions, "https://admin.test")

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "https://admin.test", rec.Header().Get("Access-Control-Allow-Origin"))
}

// ---------------------------------------------------------------------------
// Behavior 2: everything else fails closed
// ---------------------------------------------------------------------------

func TestMiddleware_UnknownOrigin_GetsNoCORSHeaders(t *testing.T) {
	rec := request(t, "https://admin.test", http.MethodGet, "https://evil.test")

	assert.Equal(t, http.StatusOK, rec.Code, "the request itself proceeds; the browser blocks the response")
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestMiddleware_EmptyAllowlist_RejectsPreflights(t *testing.T) {
	rec := request(t, "", http.MethodOptions, "https://admin.test")

	assert.Equal(t, http.StatusForbidden, rec.Code, "no configuration must mean no cross-origin access")
}

// ---------------------------------------------------------------------------
// Behavior 3: Public allows any origin
// ---------------------------------------------------------------------------

func TestPublic_AnyOrigin_GetsWildcardCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(cors.Public())
	router.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Origin", "https://any-storefront.example")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}
