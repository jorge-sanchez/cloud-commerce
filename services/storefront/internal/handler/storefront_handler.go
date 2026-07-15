// Package handler renders the buyer-facing pages (ADR-009): server-rendered
// html/template fed by the public APIs, with a thin vanilla-JS layer for
// cart, checkout, and Stripe payment against the same public endpoints.
package handler

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/storefront/internal/gateway"
)

//go:embed templates/*.html
var templateFS embed.FS

// Theme is the data-driven presentation document (ADR-009). Today it holds
// defaults; the Phase 4 builder will edit per-tenant instances of it.
type Theme struct {
	AccentColor string
	Layout      string // "grid" is the only layout the starter theme ships
}

// DefaultTheme is applied until per-tenant themes exist.
func DefaultTheme() Theme {
	return Theme{AccentColor: "#1a1a2e", Layout: "grid"}
}

// Config carries the client-side wiring injected into templates.
type Config struct {
	OrdersURL    string // buyer JS talks to the orders public API
	StripePubKey string // publishable key for Stripe.js (not a secret)
}

// StorefrontHandler renders the buyer pages.
type StorefrontHandler struct {
	platform gateway.Platform // required
	cfg      Config           // required
	tmpl     *template.Template
}

// NewStorefrontHandler parses the embedded templates.
func NewStorefrontHandler(platform gateway.Platform, cfg Config) (*StorefrontHandler, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"money": func(cents int64, currency string) string {
			return fmt.Sprintf("%s %.2f", currency, float64(cents)/100)
		},
	}).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &StorefrontHandler{platform: platform, cfg: cfg, tmpl: tmpl}, nil
}

// RegisterRoutes mounts the storefront pages.
func (h *StorefrontHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/:handle", h.StorePage)
	r.GET("/:handle/p/:productId", h.ProductPage)
	r.GET("/:handle/cart", h.CartPage)
	r.GET("/:handle/pay/:orderId", h.PayPage)
}

type pageData struct {
	Store    *gateway.Store
	Theme    Theme
	Config   Config
	Products []gateway.Product
	Product  *gateway.Product
	OrderID  string
}

func (h *StorefrontHandler) render(c *gin.Context, name string, data pageData) {
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(c.Writer, name, data); err != nil {
		_ = c.Error(err)
	}
}

func (h *StorefrontHandler) storeData(c *gin.Context) (*gateway.Store, bool) {
	store, err := h.platform.ResolveStore(c.Request.Context(), c.Param("handle"))
	if err != nil {
		c.String(http.StatusNotFound, "store not found")
		return nil, false
	}
	return store, true
}

func (h *StorefrontHandler) StorePage(c *gin.Context) {
	store, ok := h.storeData(c)
	if !ok {
		return
	}
	products, err := h.platform.ListProducts(c.Request.Context(), store.ID)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	h.render(c, "store.html", pageData{Store: store, Theme: DefaultTheme(), Config: h.cfg, Products: products})
}

func (h *StorefrontHandler) ProductPage(c *gin.Context) {
	store, ok := h.storeData(c)
	if !ok {
		return
	}
	products, err := h.platform.ListProducts(c.Request.Context(), store.ID)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	for i := range products {
		if products[i].ID == c.Param("productId") {
			h.render(c, "product.html", pageData{Store: store, Theme: DefaultTheme(), Config: h.cfg, Product: &products[i]})
			return
		}
	}
	c.String(http.StatusNotFound, "product not found")
}

func (h *StorefrontHandler) CartPage(c *gin.Context) {
	store, ok := h.storeData(c)
	if !ok {
		return
	}
	h.render(c, "cart.html", pageData{Store: store, Theme: DefaultTheme(), Config: h.cfg})
}

func (h *StorefrontHandler) PayPage(c *gin.Context) {
	store, ok := h.storeData(c)
	if !ok {
		return
	}
	h.render(c, "pay.html", pageData{Store: store, Theme: DefaultTheme(), Config: h.cfg, OrderID: c.Param("orderId")})
}
