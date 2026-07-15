// Package service holds the application services: orchestration only, no
// business rules. Required dependencies are positional constructor arguments;
// optional dependencies are Option functions.
package service

import (
	"context"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

// VariantInput is one variant in a create-product request.
type VariantInput struct {
	SKU          string
	OptionValues []string
	PriceCents   int64
}

// ProductService is the application-service port consumed by the handlers.
type ProductService interface {
	Create(ctx context.Context, tenantID, title, description string, options []string, variants []VariantInput) (*domain.Product, error)
	Get(ctx context.Context, tenantID, id string) (*domain.Product, error)
	List(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Product, int, error)
	Activate(ctx context.Context, tenantID, id string) (*domain.Product, error)
}

type productService struct {
	repo domain.ProductRepository // required
}

// Option configures optional dependencies on the product service.
type Option func(*productService)

// NewProductService constructs the product application service.
func NewProductService(repo domain.ProductRepository, opts ...Option) ProductService {
	s := &productService{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *productService) Create(ctx context.Context, tenantID, title, description string, options []string, variants []VariantInput) (*domain.Product, error) {
	product, err := domain.NewProduct(tenantID, title, description, options)
	if err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}
	for _, v := range variants {
		if err := product.AddVariant(v.SKU, v.OptionValues, v.PriceCents); err != nil {
			return nil, apperrors.ErrValidation.Wrap(err)
		}
	}
	// The aggregate persists atomically; the created event is recorded in
	// the same transaction (ADR-002).
	return s.repo.SaveNewWithVariants(ctx, tenantID, product)
}

func (s *productService) Get(ctx context.Context, tenantID, id string) (*domain.Product, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *productService) List(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Product, int, error) {
	return s.repo.ListByTenant(ctx, tenantID, page, pageSize)
}

func (s *productService) Activate(ctx context.Context, tenantID, id string) (*domain.Product, error) {
	return s.repo.ActivateIfActivatable(ctx, tenantID, id)
}
