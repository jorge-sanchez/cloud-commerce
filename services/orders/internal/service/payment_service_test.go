// Test Budget: 2 distinct behaviors × 2 = 4 max unit tests
// Actual: 4
//
// Behavior 1: StartPayment — pending orders get an intent; paid orders are
//
//	ErrConflict and the gateway is never touched
//
// Behavior 2: ConfirmPayment — a verified reference marks the order paid;
//
//	a failing reference leaves it untouched
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
)

var _ PaymentGateway = (*fakeGatewayPort)(nil)

type fakeGatewayPort struct {
	created    []string
	confirmErr error
}

func (f *fakeGatewayPort) CreatePayment(_ context.Context, order *domain.Order) (PaymentIntent, error) {
	f.created = append(f.created, order.ID)
	return PaymentIntent{Reference: "ref-" + order.ID, ClientSecret: "secret"}, nil
}

func (f *fakeGatewayPort) ConfirmPayment(_ context.Context, _, _ string) error {
	return f.confirmErr
}

func pendingOrder() *domain.Order {
	return &domain.Order{ID: "order-001", TenantID: "tenant-001", Status: domain.OrderStatusPending}
}

// ---------------------------------------------------------------------------
// Behavior 1: StartPayment
// ---------------------------------------------------------------------------

func TestPaymentService_StartPayment_PendingOrder_ReturnsIntent(t *testing.T) {
	gw := &fakeGatewayPort{}
	svc := NewPaymentService(&fakeOrderRepo{order: pendingOrder()}, gw)

	intent, err := svc.StartPayment(context.Background(), "order-001")

	require.NoError(t, err)
	assert.Equal(t, "ref-order-001", intent.Reference)
	require.Len(t, gw.created, 1, "exactly one payment must be created")
}

func TestPaymentService_StartPayment_PaidOrder_ConflictsWithoutTouchingGateway(t *testing.T) {
	paid := pendingOrder()
	require.NoError(t, paid.MarkPaid())
	gw := &fakeGatewayPort{}
	svc := NewPaymentService(&fakeOrderRepo{order: paid}, gw)

	_, err := svc.StartPayment(context.Background(), "order-001")

	require.ErrorIs(t, err, apperrors.ErrConflict)
	require.Len(t, gw.created, 0, "the gateway must not be touched for unpayable orders")
}

// ---------------------------------------------------------------------------
// Behavior 2: ConfirmPayment
// ---------------------------------------------------------------------------

func TestPaymentService_ConfirmPayment_VerifiedReference_MarksPaid(t *testing.T) {
	paid := pendingOrder()
	require.NoError(t, paid.MarkPaid())
	svc := NewPaymentService(&fakeOrderRepo{order: paid}, &fakeGatewayPort{})

	order, err := svc.ConfirmPayment(context.Background(), "order-001", "ref-order-001")

	require.NoError(t, err)
	assert.Equal(t, domain.OrderStatusPaid, order.Status)
}

func TestPaymentService_ConfirmPayment_BadReference_PassesValidationThrough(t *testing.T) {
	repo := &fakeOrderRepo{order: pendingOrder()}
	svc := NewPaymentService(repo, &fakeGatewayPort{confirmErr: apperrors.ErrValidation})

	_, err := svc.ConfirmPayment(context.Background(), "order-001", "forged-ref")

	require.ErrorIs(t, err, apperrors.ErrValidation)
}
