// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 5
//
// Behavior 1: Receive — a signed payment_intent.succeeded marks the order
//
//	paid; an already-paid conflict is acked (buyer confirm won the race)
//
// Behavior 2: Receive — bad signatures are rejected untouched
// Behavior 3: Receive — foreign event types and intents without our
//
//	metadata are acked without touching the service
package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82/webhook"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/service"
)

const whSecret = "whsec_test_secret"

var _ service.PaymentService = (*fakePayments)(nil)

type fakePayments struct {
	reconciled []string
	err        error
}

func (f *fakePayments) ReconcilePayment(_ context.Context, orderID, _ string) (*domain.Order, error) {
	f.reconciled = append(f.reconciled, orderID)
	return &domain.Order{ID: orderID, Status: domain.OrderStatusPaid}, f.err
}

func (f *fakePayments) StartPayment(context.Context, string) (service.PaymentIntent, error) {
	return service.PaymentIntent{}, nil
}

func (f *fakePayments) ConfirmPayment(context.Context, string, string) (*domain.Order, error) {
	return nil, nil
}

func (f *fakePayments) RefundOrder(context.Context, string, string, string) (*domain.Order, error) {
	return nil, nil
}

func eventBody(eventType, orderID string) []byte {
	return []byte(fmt.Sprintf(`{
		"id": "evt_1", "type": %q,
		"data": {"object": {"id": "pi_123", "metadata": {"order_id": %q}}}
	}`, eventType, orderID))
}

func deliver(t *testing.T, payments service.PaymentService, body []byte, sign bool) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewStripeWebhookHandler(payments, whSecret).RegisterRoutes(router.Group("/internal"))

	req := httptest.NewRequest(http.MethodPost, "/internal/payments/stripe/webhook", bytes.NewReader(body))
	if sign {
		signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
			Payload: body, Secret: whSecret, Timestamp: time.Now(),
		})
		req.Header.Set("Stripe-Signature", signed.Header)
	} else {
		req.Header.Set("Stripe-Signature", "t=1,v1=forged")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Behavior 1: signed succeeded events reconcile
// ---------------------------------------------------------------------------

func TestStripeWebhook_Receive_SignedSucceeded_ReconcilesOrder(t *testing.T) {
	payments := &fakePayments{}

	rec := deliver(t, payments, eventBody("payment_intent.succeeded", "order-001"), true)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, payments.reconciled, 1, "the order must be reconciled exactly once")
	assert.Equal(t, "order-001", payments.reconciled[0])
}

func TestStripeWebhook_Receive_AlreadyPaidConflict_AcksInsteadOfRetrying(t *testing.T) {
	payments := &fakePayments{err: apperrors.ErrConflict}

	rec := deliver(t, payments, eventBody("payment_intent.succeeded", "order-001"), true)

	assert.Equal(t, http.StatusNoContent, rec.Code, "a race the buyer confirm won is success, not a retry")
}

// ---------------------------------------------------------------------------
// Behavior 2: signature verification
// ---------------------------------------------------------------------------

func TestStripeWebhook_Receive_ForgedSignature_RejectsUntouched(t *testing.T) {
	payments := &fakePayments{}

	rec := deliver(t, payments, eventBody("payment_intent.succeeded", "order-001"), false)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Len(t, payments.reconciled, 0, "unverified deliveries must not reach the service")
}

// ---------------------------------------------------------------------------
// Behavior 3: foreign events are acked untouched
// ---------------------------------------------------------------------------

func TestStripeWebhook_Receive_OtherEventType_AcksUntouched(t *testing.T) {
	payments := &fakePayments{}

	rec := deliver(t, payments, eventBody("charge.updated", "order-001"), true)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, payments.reconciled, 0)
}

func TestStripeWebhook_Receive_IntentWithoutOurMetadata_AcksUntouched(t *testing.T) {
	payments := &fakePayments{}

	rec := deliver(t, payments, eventBody("payment_intent.succeeded", ""), true)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, payments.reconciled, 0, "intents we did not create must be ignored")
}
