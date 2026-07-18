// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 5
//
// Behavior 1: StorePage — renders store name and products; unknown handles 404
// Behavior 2: PayPage — injects order ID and the publishable key for Stripe.js
// Behavior 3: product media (RFC-003) — the product page renders the image
//
//	gallery lazily; a product with no images renders a placeholder, not a
//	broken <img>
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/storefront/internal/gateway"
)

var _ gateway.Platform = (*fakePlatform)(nil)

type fakePlatform struct {
	store    *gateway.Store
	products []gateway.Product
	err      error
}

func (f *fakePlatform) ResolveStore(_ context.Context, _ string) (*gateway.Store, error) {
	return f.store, f.err
}

func (f *fakePlatform) ListProducts(_ context.Context, _ string) ([]gateway.Product, error) {
	return f.products, f.err
}

func get(t *testing.T, platform gateway.Platform, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h, err := NewStorefrontHandler(platform, Config{OrdersURL: "https://orders.test", StripePubKey: "pk_test_abc"})
	require.NoError(t, err)
	router := gin.New()
	h.RegisterRoutes(router)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func testStore() *gateway.Store {
	return &gateway.Store{ID: "tenant-001", Name: "Tienda Jorge", Handle: "tienda-jorge", Currency: "PEN"}
}

// ---------------------------------------------------------------------------
// Behavior 1: StorePage
// ---------------------------------------------------------------------------

func TestStorefrontHandler_StorePage_RendersProducts(t *testing.T) {
	platform := &fakePlatform{store: testStore(), products: []gateway.Product{
		{ID: "p1", Title: "Polo Deportivo", Variants: []gateway.Variant{{ID: "v1", SKU: "POLO-M", PriceCents: 5990}}},
	}}

	rec := get(t, platform, "/tienda-jorge")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Tienda Jorge")
	assert.Contains(t, rec.Body.String(), "Polo Deportivo")
	assert.Contains(t, rec.Body.String(), "PEN 59.90", "prices must render in the store currency")
}

func TestStorefrontHandler_StorePage_UnknownHandle_404s(t *testing.T) {
	rec := get(t, &fakePlatform{err: apperrors.ErrNotFound}, "/no-such-store")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// Behavior 2: PayPage wiring
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Behavior 3: product media rendering
// ---------------------------------------------------------------------------

func TestStorefrontHandler_ProductPage_RendersImageGalleryLazily(t *testing.T) {
	platform := &fakePlatform{store: testStore(), products: []gateway.Product{{
		ID: "p1", Title: "Polo Deportivo",
		Variants: []gateway.Variant{{ID: "v1", SKU: "POLO-M", PriceCents: 5990}},
		Images: []gateway.Image{
			{URL: "https://cdn.test/a.png", AltText: "front", Width: 800, Height: 800},
			{URL: "https://cdn.test/b.png", AltText: "back", Width: 800, Height: 800},
		},
	}}}

	rec := get(t, platform, "/tienda-jorge/p/p1")

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "https://cdn.test/a.png", "the primary image must render")
	assert.Contains(t, body, "https://cdn.test/b.png", "additional images must render")
	assert.Contains(t, body, `alt="front"`, "alt text must render for accessibility")
	assert.Contains(t, body, `loading="lazy"`, "images must lazy-load")
	assert.Contains(t, body, `width="800"`, "intrinsic dimensions must render to avoid layout shift")
}

func TestStorefrontHandler_ProductPage_NoImages_RendersPlaceholder(t *testing.T) {
	platform := &fakePlatform{store: testStore(), products: []gateway.Product{{
		ID: "p1", Title: "Polo Deportivo",
		Variants: []gateway.Variant{{ID: "v1", SKU: "POLO-M", PriceCents: 5990}},
	}}}

	rec := get(t, platform, "/tienda-jorge/p/p1")

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "placeholder", "a product without photos renders a placeholder")
	assert.NotContains(t, body, "<img class=\"primary\" src=\"\"", "no broken image tag when there are no photos")
}

func TestStorefrontHandler_PayPage_InjectsOrderAndPublishableKey(t *testing.T) {
	rec := get(t, &fakePlatform{store: testStore()}, "/tienda-jorge/pay/order-123")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "order-123")
	assert.Contains(t, rec.Body.String(), "pk_test_abc")
	assert.Contains(t, rec.Body.String(), "js.stripe.com", "the page must load Stripe.js")
}
