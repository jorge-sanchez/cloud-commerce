// Test Budget: 2 distinct behaviors × 2 = 4 max unit tests
// Actual: 4
//
// Behavior 1: DrainHandler — authorized request drains once and reports the
//
//	count; drain failures surface as errors
//
// Behavior 2: DrainHandler — missing or wrong bearer token is rejected
//
//	before the drainer is touched; empty configured token fails closed
package outbox_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/outbox"
)

var _ outbox.Drainer = (*fakeDrainer)(nil)

type fakeDrainer struct {
	delivered int
	err       error
	calls     int
}

func (f *fakeDrainer) DrainOnce(_ context.Context) (int, error) {
	f.calls++
	return f.delivered, f.err
}

func drainRequest(t *testing.T, drainer outbox.Drainer, token, header string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/internal/outbox/drain", outbox.DrainHandler(drainer, token))

	req := httptest.NewRequest(http.MethodPost, "/internal/outbox/drain", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Behavior 1: authorized drain
// ---------------------------------------------------------------------------

func TestDrainHandler_ValidToken_ReturnsDeliveredCount(t *testing.T) {
	drainer := &fakeDrainer{delivered: 3}

	rec := drainRequest(t, drainer, "secret", "Bearer secret")

	require.Equal(t, http.StatusOK, rec.Code)
	var resp outbox.DrainResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 3, resp.Delivered)
	assert.Equal(t, 1, drainer.calls)
}

func TestDrainHandler_DrainerFails_RespondsWithError(t *testing.T) {
	drainer := &fakeDrainer{err: apperrors.ErrInternal}

	rec := drainRequest(t, drainer, "secret", "Bearer secret")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ---------------------------------------------------------------------------
// Behavior 2: token is required
// ---------------------------------------------------------------------------

func TestDrainHandler_WrongToken_RejectsWithoutDraining(t *testing.T) {
	drainer := &fakeDrainer{}

	rec := drainRequest(t, drainer, "secret", "Bearer wrong")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, 0, drainer.calls, "the drainer must not run for unauthorized requests")
}

func TestDrainHandler_EmptyConfiguredToken_RejectsEverything(t *testing.T) {
	drainer := &fakeDrainer{}

	rec := drainRequest(t, drainer, "", "Bearer ")

	assert.Equal(t, http.StatusUnauthorized, rec.Code, "an unset token must fail closed, not open")
	assert.Equal(t, 0, drainer.calls, "the drainer must not run for unauthorized requests")
}
