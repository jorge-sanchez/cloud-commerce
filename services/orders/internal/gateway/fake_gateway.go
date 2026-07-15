package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/service"
)

// FakeGateway is the pre-Stripe PaymentGateway (ADR-008): references are
// an HMAC of the order ID, so only a reference issued by CreatePayment
// confirms. It never moves money — wired only while PAYMENT_PROVIDER=fake;
// the Stripe adapter (#19) replaces it.
type FakeGateway struct {
	secret []byte
}

var _ service.PaymentGateway = (*FakeGateway)(nil)

// NewFakeGateway constructs the fake with a signing secret (any stable
// string; OUTBOX_DRAIN_TOKEN-grade secrecy is plenty for a fake).
func NewFakeGateway(secret string) *FakeGateway {
	return &FakeGateway{secret: []byte(secret)}
}

func (g *FakeGateway) reference(orderID string) string {
	mac := hmac.New(sha256.New, g.secret)
	mac.Write([]byte(orderID))
	return "fake_" + hex.EncodeToString(mac.Sum(nil)[:16])
}

func (g *FakeGateway) CreatePayment(_ context.Context, order *domain.Order) (service.PaymentIntent, error) {
	ref := g.reference(order.ID)
	return service.PaymentIntent{
		Reference:    ref,
		ClientSecret: ref + "_secret",
	}, nil
}

func (g *FakeGateway) ConfirmPayment(_ context.Context, orderID, reference string) error {
	if !hmac.Equal([]byte(reference), []byte(g.reference(orderID))) {
		return apperrors.ErrValidation.Wrap(fmt.Errorf("payment reference does not verify for this order"))
	}
	return nil
}

// RefundPayment on the fake verifies the reference like ConfirmPayment.
func (g *FakeGateway) RefundPayment(_ context.Context, orderID, reference string) error {
	if !hmac.Equal([]byte(reference), []byte(g.reference(orderID))) {
		return apperrors.ErrValidation.Wrap(fmt.Errorf("payment reference does not verify for this order"))
	}
	return nil
}
