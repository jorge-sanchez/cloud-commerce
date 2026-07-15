package gateway

import (
	"context"
	"fmt"
	"strings"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/paymentintent"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/service"
)

// StripeGateway is the real PaymentGateway (ADR-008, #19). CreatePayment
// opens a PaymentIntent the buyer client completes with Stripe.js;
// ConfirmPayment verifies against Stripe that the intent belongs to the
// order and actually succeeded — the reference alone proves nothing.
// Webhook-driven reconciliation is a follow-up; until then confirmation is
// buyer-initiated polling, which MarkPaidIfPayable keeps idempotent.
type StripeGateway struct{}

var _ service.PaymentGateway = (*StripeGateway)(nil)

// NewStripeGateway configures the global Stripe client with the secret key
// (test mode uses sk_test_ keys; no real money moves).
func NewStripeGateway(secretKey string) *StripeGateway {
	stripe.Key = secretKey
	return &StripeGateway{}
}

func (g *StripeGateway) CreatePayment(_ context.Context, order *domain.Order) (service.PaymentIntent, error) {
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(order.TotalCents),
		Currency: stripe.String(strings.ToLower(order.Currency)),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled:        stripe.Bool(true),
			AllowRedirects: stripe.String("never"),
		},
	}
	params.AddMetadata("order_id", order.ID)

	pi, err := paymentintent.New(params)
	if err != nil {
		return service.PaymentIntent{}, apperrors.ErrServiceUnavailable.Wrap(err)
	}
	return service.PaymentIntent{Reference: pi.ID, ClientSecret: pi.ClientSecret}, nil
}

func (g *StripeGateway) ConfirmPayment(_ context.Context, orderID, reference string) error {
	pi, err := paymentintent.Get(reference, nil)
	if err != nil {
		return apperrors.ErrValidation.Wrap(fmt.Errorf("payment reference does not resolve"))
	}
	if pi.Metadata["order_id"] != orderID {
		return apperrors.ErrValidation.Wrap(fmt.Errorf("payment reference belongs to a different order"))
	}
	if pi.Status != stripe.PaymentIntentStatusSucceeded {
		return apperrors.ErrValidation.Wrap(fmt.Errorf("payment has not succeeded (status %s)", pi.Status))
	}
	return nil
}
