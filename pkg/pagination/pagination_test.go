package pagination_test

// Unit tests for pkg/pagination.
//
// Driving port:  ParseParams(*gin.Context) — package-level function
// Observable outcomes: Params.Page, Params.PageSize fields
//
// Test budget: 5 distinct behaviors × 2 = 10 max unit tests. Actual: 5.
//
//   Behavior 1: No query params → defaults (page=1, page_size=20)
//   Behavior 2: Custom valid values → returned as-is
//   Behavior 3: page_size > 100 → capped at 100
//   Behavior 4: Invalid non-numeric values → fall back to defaults
//   Behavior 5: page_size=0 → falls back to default (20)

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/jorge-sanchez/cloud-commerce/pkg/pagination"
)

// captureParams builds a minimal gin.Context from a raw URL query string and
// calls ParseParams, returning the result. It does not start a real server.
func captureParams(query string) pagination.Params {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return pagination.ParseParams(c)
}

// ---------------------------------------------------------------------------
// Behavior 1: No query params → defaults
// ---------------------------------------------------------------------------

func TestParseParams_Defaults(t *testing.T) {
	p := captureParams("")

	assert.Equal(t, 1, p.Page, "default page must be 1")
	assert.Equal(t, 20, p.PageSize, "default page_size must be 20")
}

// ---------------------------------------------------------------------------
// Behavior 2: Custom valid values → returned as-is
// ---------------------------------------------------------------------------

func TestParseParams_CustomValues(t *testing.T) {
	p := captureParams("page=3&page_size=50")

	assert.Equal(t, 3, p.Page)
	assert.Equal(t, 50, p.PageSize)
}

// ---------------------------------------------------------------------------
// Behavior 3: page_size capped at 100
// ---------------------------------------------------------------------------

func TestParseParams_CapsPageSize(t *testing.T) {
	p := captureParams("page_size=200")

	assert.Equal(t, 100, p.PageSize, "page_size must be capped at MaxPageSize=100")
}

// ---------------------------------------------------------------------------
// Behavior 4: Invalid (non-numeric) values → fall back to defaults
// ---------------------------------------------------------------------------

func TestParseParams_InvalidFallsToDefault(t *testing.T) {
	p := captureParams("page=abc&page_size=xyz")

	assert.Equal(t, 1, p.Page, "non-numeric page must fall back to default")
	assert.Equal(t, 20, p.PageSize, "non-numeric page_size must fall back to default")
}

// ---------------------------------------------------------------------------
// Behavior 5: page_size=0 → falls back to default (20)
// ---------------------------------------------------------------------------

func TestParseParams_PageSizeMin(t *testing.T) {
	p := captureParams("page_size=0")

	assert.Equal(t, 20, p.PageSize, "page_size=0 must fall back to default")
}
