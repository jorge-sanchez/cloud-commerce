// Package gateway holds outbound HTTP adapters to the other services'
// public APIs. Reads only — writes between services go through events
// (ADR-002).
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/service"
)

// HTTPPlatform implements service.Platform against the merchants and
// catalog public endpoints.
type HTTPPlatform struct {
	merchantsURL string
	catalogURL   string
	client       *http.Client
}

var _ service.Platform = (*HTTPPlatform)(nil)

// NewHTTPPlatform wires the gateway to the two public API base URLs.
func NewHTTPPlatform(merchantsURL, catalogURL string) *HTTPPlatform {
	return &HTTPPlatform{
		merchantsURL: merchantsURL,
		catalogURL:   catalogURL,
		client:       &http.Client{Timeout: 5 * time.Second},
	}
}

func (p *HTTPPlatform) ResolveStore(ctx context.Context, handle string) (service.StoreInfo, error) {
	var body struct {
		ID       string `json:"id"`
		Currency string `json:"currency"`
	}
	err := p.getJSON(ctx, fmt.Sprintf("%s/v1/public/stores/%s", p.merchantsURL, url.PathEscape(handle)), &body)
	if err != nil {
		return service.StoreInfo{}, err
	}
	return service.StoreInfo{TenantID: body.ID, Currency: body.Currency}, nil
}

func (p *HTTPPlatform) GetActiveVariant(ctx context.Context, tenantID, variantID string) (service.VariantSnapshot, error) {
	var body struct {
		VariantID    string `json:"variant_id"`
		SKU          string `json:"sku"`
		ProductTitle string `json:"product_title"`
		PriceCents   int64  `json:"price_cents"`
	}
	err := p.getJSON(ctx, fmt.Sprintf("%s/v1/public/stores/%s/variants/%s",
		p.catalogURL, url.PathEscape(tenantID), url.PathEscape(variantID)), &body)
	if err != nil {
		return service.VariantSnapshot{}, err
	}
	return service.VariantSnapshot{
		VariantID:  body.VariantID,
		SKU:        body.SKU,
		Title:      body.ProductTitle,
		PriceCents: body.PriceCents,
	}, nil
}

// GetShippingMethod resolves an active flat rate from the merchants
// public list (RFC-001 part 1).
func (p *HTTPPlatform) GetShippingMethod(ctx context.Context, tenantID, methodID string) (service.ShippingMethod, error) {
	var body struct {
		Items []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			PriceCents int64  `json:"price_cents"`
		} `json:"items"`
	}
	err := p.getJSON(ctx, fmt.Sprintf("%s/v1/public/tenants/%s/shipping-methods",
		p.merchantsURL, url.PathEscape(tenantID)), &body)
	if err != nil {
		return service.ShippingMethod{}, err
	}
	for _, m := range body.Items {
		if m.ID == methodID {
			return service.ShippingMethod{ID: m.ID, Name: m.Name, PriceCents: m.PriceCents}, nil
		}
	}
	return service.ShippingMethod{}, apperrors.ErrNotFound
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

	switch {
	case res.StatusCode == http.StatusNotFound:
		return apperrors.ErrNotFound
	case res.StatusCode != http.StatusOK:
		return apperrors.ErrServiceUnavailable.Wrap(fmt.Errorf("upstream returned %d", res.StatusCode))
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}
