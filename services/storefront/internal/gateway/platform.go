// Package gateway holds read adapters over the platform's public APIs.
// The storefront is a client of the platform, never a backdoor (ADR-009).
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
)

// Store is the public store identity.
type Store struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Handle   string `json:"handle"`
	Currency string `json:"currency"`
}

// Variant is a purchasable option of a product.
type Variant struct {
	ID           string   `json:"id"`
	SKU          string   `json:"sku"`
	OptionValues []string `json:"option_values"`
	PriceCents   int64    `json:"price_cents"`
}

// Product is a storefront-visible product.
type Product struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Options     []string  `json:"options"`
	Variants    []Variant `json:"variants"`
}

type productList struct {
	Items []Product `json:"items"`
}

// Platform reads the public APIs, implemented by HTTPPlatform.
type Platform interface {
	ResolveStore(ctx context.Context, handle string) (*Store, error)
	ListProducts(ctx context.Context, tenantID string) ([]Product, error)
}

// HTTPPlatform is the production Platform.
type HTTPPlatform struct {
	merchantsURL string
	catalogURL   string
	client       *http.Client
}

var _ Platform = (*HTTPPlatform)(nil)

// NewHTTPPlatform wires the gateway to the public API base URLs.
func NewHTTPPlatform(merchantsURL, catalogURL string) *HTTPPlatform {
	return &HTTPPlatform{
		merchantsURL: merchantsURL,
		catalogURL:   catalogURL,
		client:       &http.Client{Timeout: 5 * time.Second},
	}
}

func (p *HTTPPlatform) ResolveStore(ctx context.Context, handle string) (*Store, error) {
	var s Store
	if err := p.getJSON(ctx, fmt.Sprintf("%s/v1/public/stores/%s", p.merchantsURL, url.PathEscape(handle)), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (p *HTTPPlatform) ListProducts(ctx context.Context, tenantID string) ([]Product, error) {
	var list productList
	err := p.getJSON(ctx, fmt.Sprintf("%s/v1/public/stores/%s/products?page_size=50", p.catalogURL, url.PathEscape(tenantID)), &list)
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (p *HTTPPlatform) getJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	res, err := p.client.Do(req)
	if err != nil {
		return apperrors.ErrServiceUnavailable.Wrap(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode == http.StatusNotFound {
		return apperrors.ErrNotFound
	}
	if res.StatusCode != http.StatusOK {
		return apperrors.ErrServiceUnavailable.Wrap(fmt.Errorf("upstream returned %d", res.StatusCode))
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}
