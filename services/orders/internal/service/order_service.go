// Package service holds the application services: orchestration only, no
// business rules. Required dependencies are positional constructor arguments;
// optional dependencies are Option functions.
package service

import (
	"context"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
)

// POSLine is one line of an in-person sale.
type POSLine struct {
	VariantID string
	Qty       int64
}

// StoreInfo is what checkout needs to know about a store.
type StoreInfo struct {
	TenantID string
	Currency string
}

// VariantSnapshot is the priced line captured when a buyer adds an item.
type VariantSnapshot struct {
	VariantID  string
	SKU        string
	Title      string
	PriceCents int64
}

// ShippingMethod is a resolved flat rate (RFC-001).
type ShippingMethod struct {
	ID         string
	Name       string
	PriceCents int64
}

// Platform is the outbound port to the other services' public APIs,
// implemented by gateway.HTTPPlatform. The service depends on the
// interface, not the concrete implementation.
type Platform interface {
	// ResolveStore turns a public store handle into tenant + currency.
	ResolveStore(ctx context.Context, handle string) (StoreInfo, error)
	// GetActiveVariant returns the snapshot for an active product's
	// variant, or apperrors.ErrNotFound (drafts are not purchasable).
	GetActiveVariant(ctx context.Context, tenantID, variantID string) (VariantSnapshot, error)
	// GetShippingMethod resolves an active flat rate for the tenant.
	GetShippingMethod(ctx context.Context, tenantID, methodID string) (ShippingMethod, error)
}

// OrderService is the application-service port consumed by the handlers.
type OrderService interface {
	CreateCart(ctx context.Context, storeHandle string) (*domain.Cart, error)
	GetCart(ctx context.Context, cartID string) (*domain.Cart, error)
	AddItem(ctx context.Context, cartID, variantID string, qty int64) (*domain.Cart, error)
	RemoveItem(ctx context.Context, cartID, variantID string) (*domain.Cart, error)
	Checkout(ctx context.Context, cartID, email string, addr domain.Address, shippingMethodID string) (*domain.Order, error)
	ListOrders(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Order, int, error)
	FulfillOrder(ctx context.Context, tenantID, orderID, trackingNumber, carrier string) (*domain.Order, error)
	// RecordPOSSale registers an in-person cash sale (ADR-010): prices
	// snapshot server-side; idempotent on the client sale ID.
	RecordPOSSale(ctx context.Context, tenantID, clientSaleID, currency, email, locationID string, lines []POSLine) (*domain.Order, error)
	GetOrder(ctx context.Context, tenantID, orderID string) (*domain.Order, error)
	GetAnalytics(ctx context.Context, tenantID string, days int) (*domain.SalesSummary, error)
}

type orderService struct {
	repo     domain.OrderRepository // required
	platform Platform               // required
}

// Option configures optional dependencies on the order service.
type Option func(*orderService)

// NewOrderService constructs the order application service.
func NewOrderService(repo domain.OrderRepository, platform Platform, opts ...Option) OrderService {
	s := &orderService{repo: repo, platform: platform}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *orderService) CreateCart(ctx context.Context, storeHandle string) (*domain.Cart, error) {
	store, err := s.platform.ResolveStore(ctx, storeHandle)
	if err != nil {
		return nil, err
	}
	return s.repo.SaveNewCart(ctx, &domain.Cart{TenantID: store.TenantID, Currency: store.Currency})
}

func (s *orderService) GetCart(ctx context.Context, cartID string) (*domain.Cart, error) {
	return s.repo.GetCart(ctx, cartID)
}

func (s *orderService) AddItem(ctx context.Context, cartID, variantID string, qty int64) (*domain.Cart, error) {
	cart, err := s.repo.GetCart(ctx, cartID)
	if err != nil {
		return nil, err
	}
	// Snapshot price and title server-side — never trust the client's.
	snap, err := s.platform.GetActiveVariant(ctx, cart.TenantID, variantID)
	if err != nil {
		return nil, err
	}
	if err := cart.AddItem(domain.Item{
		VariantID:  snap.VariantID,
		SKU:        snap.SKU,
		Title:      snap.Title,
		PriceCents: snap.PriceCents,
		Qty:        qty,
	}); err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}
	return s.repo.ReplaceItems(ctx, cart)
}

func (s *orderService) RemoveItem(ctx context.Context, cartID, variantID string) (*domain.Cart, error) {
	cart, err := s.repo.GetCart(ctx, cartID)
	if err != nil {
		return nil, err
	}
	cart.RemoveItem(variantID)
	return s.repo.ReplaceItems(ctx, cart)
}

func (s *orderService) Checkout(ctx context.Context, cartID, email string, addr domain.Address, shippingMethodID string) (*domain.Order, error) {
	cart, err := s.repo.GetCart(ctx, cartID)
	if err != nil {
		return nil, err
	}
	// Price the shipping server-side — never trust the client's number.
	method, err := s.platform.GetShippingMethod(ctx, cart.TenantID, shippingMethodID)
	if err != nil {
		return nil, err
	}
	// The entity validates inside the repository transaction; the placed
	// event is recorded there too (ADR-002).
	return s.repo.PlaceOrderFromCart(ctx, cartID, email, addr, method.Name, method.PriceCents)
}

func (s *orderService) ListOrders(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Order, int, error) {
	return s.repo.ListByTenant(ctx, tenantID, page, pageSize)
}

func (s *orderService) RecordPOSSale(ctx context.Context, tenantID, clientSaleID, currency, email, locationID string, lines []POSLine) (*domain.Order, error) {
	items := make([]domain.Item, 0, len(lines))
	for _, l := range lines {
		snap, err := s.platform.GetActiveVariant(ctx, tenantID, l.VariantID)
		if err != nil {
			return nil, err
		}
		items = append(items, domain.Item{
			VariantID: snap.VariantID, SKU: snap.SKU, Title: snap.Title,
			PriceCents: snap.PriceCents, Qty: l.Qty,
		})
	}
	sale, err := domain.NewPOSSale(tenantID, currency, email, locationID, items) // entity decides
	if err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}
	return s.repo.SavePOSSale(ctx, tenantID, clientSaleID, sale)
}

func (s *orderService) FulfillOrder(ctx context.Context, tenantID, orderID, trackingNumber, carrier string) (*domain.Order, error) {
	// The entity decides inside the repository transaction; order_fulfilled
	// is recorded there too (ADR-002).
	return s.repo.FulfillIfFulfillable(ctx, tenantID, orderID, trackingNumber, carrier)
}

func (s *orderService) GetOrder(ctx context.Context, tenantID, orderID string) (*domain.Order, error) {
	return s.repo.GetByID(ctx, tenantID, orderID)
}

func (s *orderService) GetAnalytics(ctx context.Context, tenantID string, days int) (*domain.SalesSummary, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	return s.repo.GetSalesSummary(ctx, tenantID, days)
}
