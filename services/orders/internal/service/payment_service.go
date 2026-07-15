package service

import (
	"context"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
)

// PaymentIntent is the provider handoff a buyer client needs to complete
// payment (shaped like real provider flows, ADR-008).
type PaymentIntent struct {
	Reference    string
	ClientSecret string
}

// PaymentGateway is the provider port (ADR-008). Implemented by
// gateway.FakeGateway today; the Stripe adapter (#19) replaces it without
// touching this service.
type PaymentGateway interface {
	// CreatePayment initializes payment for a pending order.
	CreatePayment(ctx context.Context, order *domain.Order) (PaymentIntent, error)
	// ConfirmPayment verifies a completed payment reference; a reference
	// that does not verify returns apperrors.ErrValidation.
	ConfirmPayment(ctx context.Context, orderID, reference string) error
}

// PaymentService is the buyer-facing payment flow.
type PaymentService interface {
	// StartPayment initializes payment for a pending order.
	StartPayment(ctx context.Context, orderID string) (PaymentIntent, error)
	// ConfirmPayment verifies the provider reference and marks the order
	// paid (the entity decides; order_paid is recorded in that transaction).
	ConfirmPayment(ctx context.Context, orderID, reference string) (*domain.Order, error)
}

type paymentService struct {
	repo    domain.OrderRepository // required
	gateway PaymentGateway         // required
}

// NewPaymentService constructs the payment application service.
func NewPaymentService(repo domain.OrderRepository, gw PaymentGateway) PaymentService {
	return &paymentService{repo: repo, gateway: gw}
}

func (s *paymentService) StartPayment(ctx context.Context, orderID string) (PaymentIntent, error) {
	order, err := s.repo.GetPublicByID(ctx, orderID)
	if err != nil {
		return PaymentIntent{}, err
	}
	if !order.CanPay() { // entity decides
		return PaymentIntent{}, apperrors.ErrConflict.Wrap(domain.ErrNotPayable)
	}
	return s.gateway.CreatePayment(ctx, order)
}

func (s *paymentService) ConfirmPayment(ctx context.Context, orderID, reference string) (*domain.Order, error) {
	if err := s.gateway.ConfirmPayment(ctx, orderID, reference); err != nil {
		return nil, err
	}
	return s.repo.MarkPaidIfPayable(ctx, orderID)
}
