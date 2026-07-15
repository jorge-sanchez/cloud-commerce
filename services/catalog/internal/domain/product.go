// Package domain holds the Product aggregate: product, variants, domain
// events, and the repository interface. Business rules live here — services
// orchestrate, repositories persist.
package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ProductStatus is the lifecycle state of a product.
type ProductStatus string

const (
	ProductStatusDraft    ProductStatus = "draft"
	ProductStatusActive   ProductStatus = "active"
	ProductStatusArchived ProductStatus = "archived"
)

// MaxOptions caps how many option axes a product may declare (Size, Color, …).
const MaxOptions = 3

// Domain sentinel errors for entity-level failures.
var (
	ErrEmptyTitle       = errors.New("product title must not be empty")
	ErrTooManyOptions   = errors.New("a product may declare at most 3 options")
	ErrBadOptionName    = errors.New("option names must be non-empty and unique")
	ErrEmptySKU         = errors.New("variant SKU must not be empty")
	ErrDuplicateSKU     = errors.New("variant SKU is already used in this product")
	ErrOptionArity      = errors.New("variant must supply one value per product option")
	ErrDuplicateVariant = errors.New("a variant with these option values already exists")
	ErrNegativePrice    = errors.New("variant price must not be negative")
	ErrNotActivatable   = errors.New("product cannot be activated in its current status")
	ErrNoVariants       = errors.New("product needs at least one variant to be activated")
)

// Variant is a purchasable version of a product. Price is in integer minor
// units (cents); the display currency comes from the store settings.
type Variant struct {
	ID           string
	ProductID    string
	SKU          string
	OptionValues []string
	PriceCents   int64
	CreatedAt    time.Time
}

// Product is the aggregate root. Variants never exist without it and are
// persisted with it atomically (SaveNewWithVariants).
type Product struct {
	ID          string
	TenantID    string
	Title       string
	Description string
	Status      ProductStatus
	Options     []string
	Variants    []*Variant
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewProduct constructs a draft product. IDs and timestamps are assigned by
// the repository on save.
func NewProduct(tenantID, title, description string, options []string) (*Product, error) {
	if strings.TrimSpace(title) == "" {
		return nil, ErrEmptyTitle
	}
	if len(options) > MaxOptions {
		return nil, fmt.Errorf("%w: got %d", ErrTooManyOptions, len(options))
	}
	seen := make(map[string]bool, len(options))
	for _, o := range options {
		name := strings.TrimSpace(o)
		if name == "" || seen[strings.ToLower(name)] {
			return nil, fmt.Errorf("%w: %q", ErrBadOptionName, o)
		}
		seen[strings.ToLower(name)] = true
	}
	return &Product{
		TenantID:    tenantID,
		Title:       strings.TrimSpace(title),
		Description: description,
		Status:      ProductStatusDraft,
		Options:     options,
	}, nil
}

// AddVariant validates a variant against the product's options and appends
// it. The aggregate enforces SKU and option-combination uniqueness.
func (p *Product) AddVariant(sku string, optionValues []string, priceCents int64) error {
	sku = strings.TrimSpace(sku)
	if sku == "" {
		return ErrEmptySKU
	}
	if len(optionValues) != len(p.Options) {
		return fmt.Errorf("%w: product has %d options, variant supplies %d values",
			ErrOptionArity, len(p.Options), len(optionValues))
	}
	if priceCents < 0 {
		return fmt.Errorf("%w: %d", ErrNegativePrice, priceCents)
	}
	combo := strings.ToLower(strings.Join(optionValues, "\x1f"))
	for _, v := range p.Variants {
		if strings.EqualFold(v.SKU, sku) {
			return fmt.Errorf("%w: %q", ErrDuplicateSKU, sku)
		}
		if strings.ToLower(strings.Join(v.OptionValues, "\x1f")) == combo {
			return fmt.Errorf("%w: %v", ErrDuplicateVariant, optionValues)
		}
	}
	p.Variants = append(p.Variants, &Variant{
		SKU:          sku,
		OptionValues: optionValues,
		PriceCents:   priceCents,
	})
	return nil
}

// CanActivate reports whether the product may transition to active.
func (p *Product) CanActivate() bool {
	return p.Status == ProductStatusDraft && len(p.Variants) > 0
}

// Activate transitions the product to active. The entity decides its own
// transitions — callers must not check or set Status directly.
func (p *Product) Activate() error {
	if p.Status != ProductStatusDraft {
		return fmt.Errorf("%w: status is %q", ErrNotActivatable, p.Status)
	}
	if len(p.Variants) == 0 {
		return ErrNoVariants
	}
	p.Status = ProductStatusActive
	return nil
}

// Event types for the product aggregate.
const (
	ProductCreatedEventType   = "catalog.product_created"
	ProductActivatedEventType = "catalog.product_activated"
)

// VariantPayload is the wire shape of a variant inside product events.
type VariantPayload struct {
	VariantID  string `json:"variant_id"`
	SKU        string `json:"sku"`
	PriceCents int64  `json:"price_cents"`
}

// ProductCreatedEvent is emitted when a product is created; inventory
// consumes it to initialize stock records per variant.
type ProductCreatedEvent struct {
	ProductID string           `json:"product_id"`
	TenantID  string           `json:"tenant_id"`
	Title     string           `json:"title"`
	Variants  []VariantPayload `json:"variants"`
	CreatedAt time.Time        `json:"created_at"`
}

// ProductActivatedEvent is emitted when a product goes live.
type ProductActivatedEvent struct {
	ProductID   string    `json:"product_id"`
	TenantID    string    `json:"tenant_id"`
	Title       string    `json:"title"`
	ActivatedAt time.Time `json:"activated_at"`
}

// NewProductCreatedEvent builds the event from the persisted aggregate.
func NewProductCreatedEvent(p *Product, at time.Time) ProductCreatedEvent {
	variants := make([]VariantPayload, 0, len(p.Variants))
	for _, v := range p.Variants {
		variants = append(variants, VariantPayload{VariantID: v.ID, SKU: v.SKU, PriceCents: v.PriceCents})
	}
	return ProductCreatedEvent{
		ProductID: p.ID,
		TenantID:  p.TenantID,
		Title:     p.Title,
		Variants:  variants,
		CreatedAt: at,
	}
}

// NewProductActivatedEvent builds the event from the persisted aggregate.
func NewProductActivatedEvent(p *Product, at time.Time) ProductActivatedEvent {
	return ProductActivatedEvent{ProductID: p.ID, TenantID: p.TenantID, Title: p.Title, ActivatedAt: at}
}

// ProductRepository is the persistence port for the Product aggregate.
type ProductRepository interface {
	// SaveNewWithVariants persists the product and all its variants in one
	// transaction (aggregate rule: no separate variant insert). A SKU
	// already used by the tenant returns apperrors.ErrConflict.
	SaveNewWithVariants(ctx context.Context, tenantID string, p *Product) (*Product, error)
	// GetByID returns the product with its variants, or apperrors.ErrNotFound.
	GetByID(ctx context.Context, tenantID, id string) (*Product, error)
	// ListByTenant returns one page of products (with variants) plus the
	// total count, newest first.
	ListByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*Product, int, error)
	// ListActiveByTenant is the storefront read: active products only.
	ListActiveByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*Product, int, error)
	// ActivateIfActivatable loads the product, lets the entity decide the
	// transition, and persists the result. Returns apperrors.ErrConflict
	// when the entity rejects the transition.
	ActivateIfActivatable(ctx context.Context, tenantID, id string) (*Product, error)
}
