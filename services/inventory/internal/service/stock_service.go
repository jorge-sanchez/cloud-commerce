// Package service holds the application services: orchestration only, no
// business rules. Required dependencies are positional constructor arguments;
// optional dependencies are Option functions.
package service

import (
	"context"
	"encoding/json"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/domain"
)

// CatalogProductCreatedType is the consumed cross-service event type. The
// payload shape below mirrors the catalog service's contract — kept minimal
// to the fields inventory needs. (A shared pkg/ contract module becomes
// worthwhile at the third consumer; until then this duplication is deliberate.)
const CatalogProductCreatedType = "catalog.product_created"

// OrdersOrderPaidType is the consumed order-paid event type (issue #18).
const OrdersOrderPaidType = "orders.order_paid"

type orderPaidPayload struct {
	OrderID string `json:"order_id"`
	Items   []struct {
		VariantID string `json:"variant_id"`
		Qty       int64  `json:"qty"`
	} `json:"items"`
}

type productCreatedPayload struct {
	ProductID string `json:"product_id"`
	Variants  []struct {
		VariantID string `json:"variant_id"`
		SKU       string `json:"sku"`
	} `json:"variants"`
}

// StockService is the application-service port consumed by the handlers.
type StockService interface {
	// ProcessEvent consumes a cross-service envelope. Unknown event types
	// are ignored (acked): the topic will carry more than we consume.
	// Idempotent — Pub/Sub delivers at-least-once.
	ProcessEvent(ctx context.Context, env events.Envelope) error
	ListStock(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.StockLevel, int, error)
	AdjustStock(ctx context.Context, tenantID, locationID, variantID string, delta int64) (*domain.StockLevel, error)
	CreateLocation(ctx context.Context, tenantID, name string) (*domain.Location, error)
	ListLocations(ctx context.Context, tenantID string) ([]*domain.Location, error)
}

type stockService struct {
	repo domain.StockRepository // required
}

// Option configures optional dependencies on the stock service.
type Option func(*stockService)

// NewStockService constructs the inventory application service.
func NewStockService(repo domain.StockRepository, opts ...Option) StockService {
	s := &stockService{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *stockService) ProcessEvent(ctx context.Context, env events.Envelope) error {
	switch env.Type {
	case CatalogProductCreatedType:
		return s.initializeFromProductCreated(ctx, env)
	case OrdersOrderPaidType:
		return s.deductFromOrderPaid(ctx, env)
	default:
		return nil // not ours — ack so the broker stops redelivering
	}
}

func (s *stockService) initializeFromProductCreated(ctx context.Context, env events.Envelope) error {
	var payload productCreatedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return apperrors.ErrValidation.Wrap(err)
	}

	location, err := s.repo.EnsureDefaultLocation(ctx, env.TenantID)
	if err != nil {
		return err
	}

	items := make([]domain.StockInit, 0, len(payload.Variants))
	for _, v := range payload.Variants {
		items = append(items, domain.StockInit{VariantID: v.VariantID, SKU: v.SKU})
	}
	return s.repo.InitializeStock(ctx, env.TenantID, location.ID, items)
}

func (s *stockService) deductFromOrderPaid(ctx context.Context, env events.Envelope) error {
	var payload orderPaidPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return apperrors.ErrValidation.Wrap(err)
	}
	items := make([]domain.StockDeduction, 0, len(payload.Items))
	for _, it := range payload.Items {
		items = append(items, domain.StockDeduction{VariantID: it.VariantID, Qty: it.Qty})
	}
	// Deduped by envelope ID inside the repository transaction.
	return s.repo.ApplyStockDeduction(ctx, env.TenantID, env.ID, items)
}

func (s *stockService) ListStock(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.StockLevel, int, error) {
	return s.repo.ListStockByTenant(ctx, tenantID, page, pageSize)
}

func (s *stockService) AdjustStock(ctx context.Context, tenantID, locationID, variantID string, delta int64) (*domain.StockLevel, error) {
	// The entity enforces the non-negative rule inside the repository
	// transaction; the adjusted event is recorded there too (ADR-002).
	return s.repo.AdjustIfSufficient(ctx, tenantID, locationID, variantID, delta)
}

func (s *stockService) CreateLocation(ctx context.Context, tenantID, name string) (*domain.Location, error) {
	location, err := domain.NewLocation(tenantID, name)
	if err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}
	return s.repo.SaveNewLocation(ctx, tenantID, location)
}

func (s *stockService) ListLocations(ctx context.Context, tenantID string) ([]*domain.Location, error) {
	return s.repo.ListLocations(ctx, tenantID)
}
