// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 5
//
// Behavior 1: Receive — a validated push from the pusher SA reaches the
//
//	service; a processing failure returns non-2xx so Pub/Sub retries
//
// Behavior 2: Receive — wrong SA or failed validation is 401 untouched
// Behavior 3: Receive — malformed bodies are acked (2xx) so they are not
//
//	redelivered forever
package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/domain"
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/service"
)

const pusherSA = "pubsub-pusher@project.iam.gserviceaccount.com"

var _ PushTokenValidator = (*fakeValidator)(nil)

type fakeValidator struct {
	email string
	err   error
}

func (f *fakeValidator) Validate(_ context.Context, _, _ string) (string, error) {
	return f.email, f.err
}

// fakeStockService implements service.StockService; only ProcessEvent is
// exercised by the push handler.
var _ service.StockService = (*fakeStockService)(nil)

type fakeStockService struct {
	processed []events.Envelope
	err       error
}

func (f *fakeStockService) ProcessEvent(_ context.Context, env events.Envelope) error {
	f.processed = append(f.processed, env)
	return f.err
}

func (f *fakeStockService) ListStock(context.Context, string, int, int) ([]*domain.StockLevel, int, error) {
	return nil, 0, nil
}

func (f *fakeStockService) AdjustStock(context.Context, string, string, string, int64) (*domain.StockLevel, error) {
	return nil, nil
}

func (f *fakeStockService) CreateLocation(context.Context, string, string) (*domain.Location, error) {
	return nil, nil
}

func (f *fakeStockService) ListLocations(context.Context, string) ([]*domain.Location, error) {
	return nil, nil
}

func pushBody(t *testing.T, envType string) []byte {
	t.Helper()
	env, err := events.New("tenant-001", "product-001", envType, time.Now(), map[string]string{"k": "v"})
	require.NoError(t, err)
	raw, err := json.Marshal(env)
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{
		"message":      map[string]any{"data": base64.StdEncoding.EncodeToString(raw), "messageId": "m-1"},
		"subscription": "projects/p/subscriptions/s",
	})
	require.NoError(t, err)
	return body
}

func pushRequestRecorder(t *testing.T, svc *fakeStockService, v PushTokenValidator, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewPubSubHandler(svc, v, "https://inventory.test/internal/events/pubsub", pusherSA).
		RegisterRoutes(router.Group("/internal"))

	req := httptest.NewRequest(http.MethodPost, "/internal/events/pubsub", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer some-oidc-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Behavior 1: validated pushes reach the service
// ---------------------------------------------------------------------------

func TestPubSubHandler_Receive_ValidPush_ProcessesAndAcks(t *testing.T) {
	svc := &fakeStockService{}
	rec := pushRequestRecorder(t, svc, &fakeValidator{email: pusherSA}, pushBody(t, "catalog.product_created"))

	assert.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, svc.processed, 1, "the envelope must reach the service")
	assert.Equal(t, "catalog.product_created", svc.processed[0].Type)
}

func TestPubSubHandler_Receive_ProcessingFails_ReturnsNon2xxForRetry(t *testing.T) {
	svc := &fakeStockService{err: apperrors.ErrInternal}
	rec := pushRequestRecorder(t, svc, &fakeValidator{email: pusherSA}, pushBody(t, "catalog.product_created"))

	assert.Equal(t, http.StatusInternalServerError, rec.Code, "failures must be non-2xx so Pub/Sub redelivers")
}

// ---------------------------------------------------------------------------
// Behavior 2: authentication
// ---------------------------------------------------------------------------

func TestPubSubHandler_Receive_WrongServiceAccount_Rejects(t *testing.T) {
	svc := &fakeStockService{}
	rec := pushRequestRecorder(t, svc, &fakeValidator{email: "attacker@evil.test"}, pushBody(t, "catalog.product_created"))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Len(t, svc.processed, 0, "unauthenticated pushes must not reach the service")
}

func TestPubSubHandler_Receive_ValidationFails_Rejects(t *testing.T) {
	svc := &fakeStockService{}
	rec := pushRequestRecorder(t, svc, &fakeValidator{err: errors.New("bad token")}, pushBody(t, "catalog.product_created"))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ---------------------------------------------------------------------------
// Behavior 3: malformed bodies are acked
// ---------------------------------------------------------------------------

func TestPubSubHandler_Receive_MalformedBody_AcksWithoutProcessing(t *testing.T) {
	svc := &fakeStockService{}
	rec := pushRequestRecorder(t, svc, &fakeValidator{email: pusherSA}, []byte("{not json"))

	assert.Equal(t, http.StatusNoContent, rec.Code, "a poison message must be acked, not redelivered forever")
	require.Len(t, svc.processed, 0)
}
