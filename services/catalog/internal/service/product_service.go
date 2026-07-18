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
	// ListPublic is the storefront read: active products only, no auth.
	ListPublic(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Product, int, error)
	// GetPublicVariant is the storefront's purchasable-variant lookup.
	GetPublicVariant(ctx context.Context, tenantID, variantID string) (*domain.VariantLookup, error)
	Activate(ctx context.Context, tenantID, id string) (*domain.Product, error)

	// SignImageUpload mints a short-lived direct-to-storage upload URL for a
	// new product image, returning the object key to finalize with.
	SignImageUpload(ctx context.Context, tenantID, productID, contentType string) (key, url string, err error)
	// AttachImage finalizes an uploaded object onto the product's gallery.
	AttachImage(ctx context.Context, tenantID, productID string, in AttachImageInput) (*domain.Product, error)
	// ReorderImages reorders the product's gallery (position 0 = primary).
	ReorderImages(ctx context.Context, tenantID, productID string, orderedIDs []string) (*domain.Product, error)
	// RemoveImage removes one image from the gallery and its stored object.
	RemoveImage(ctx context.Context, tenantID, productID, imageID string) (*domain.Product, error)
	// SetImageAlt updates one image's alt text.
	SetImageAlt(ctx context.Context, tenantID, productID, imageID, alt string) (*domain.Product, error)
}

// AttachImageInput is a finalize request: the object the browser uploaded plus
// its cosmetic dimensions (read client-side). The authoritative content type
// and size are re-read from storage server-side.
type AttachImageInput struct {
	StorageKey string
	AltText    string
	Width      int
	Height     int
}

type productService struct {
	repo  domain.ProductRepository // required
	media domain.MediaStore        // may be nil (image endpoints disabled)
}

// Option configures optional dependencies on the product service.
type Option func(*productService)

// WithMediaStore wires the object-storage port for product images.
func WithMediaStore(m domain.MediaStore) Option {
	return func(s *productService) { s.media = m }
}

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

func (s *productService) ListPublic(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Product, int, error) {
	return s.repo.ListActiveByTenant(ctx, tenantID, page, pageSize)
}

func (s *productService) GetPublicVariant(ctx context.Context, tenantID, variantID string) (*domain.VariantLookup, error) {
	return s.repo.GetActiveVariant(ctx, tenantID, variantID)
}

func (s *productService) Activate(ctx context.Context, tenantID, id string) (*domain.Product, error) {
	return s.repo.ActivateIfActivatable(ctx, tenantID, id)
}
