// Package handler holds the HTTP adapters (Gin). Handlers translate between
// the wire contract (apitypes.go) and the application service — no business
// rules here.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jorge-sanchez/cloud-commerce/pkg/auth"
	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/pagination"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/service"
)

// ProductHandler exposes the catalog HTTP surface. Both owner and staff
// manage the catalog — no role gate here (unlike staff management).
type ProductHandler struct {
	svc service.ProductService
}

// NewProductHandler constructs the handler.
func NewProductHandler(svc service.ProductService) *ProductHandler {
	return &ProductHandler{svc: svc}
}

// RegisterRoutes mounts the product routes; the group must be wrapped with
// auth.Middleware (see cmd/main.go).
func (h *ProductHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/products", h.Create)
	rg.GET("/products", h.List)
	rg.GET("/products/:id", h.Get)
	rg.POST("/products/:id/activate", h.Activate)
}

type variantRequest struct {
	SKU          string   `json:"sku" binding:"required"`
	OptionValues []string `json:"option_values"`
	PriceCents   int64    `json:"price_cents"`
}

type createProductRequest struct {
	Title       string           `json:"title" binding:"required"`
	Description string           `json:"description"`
	Options     []string         `json:"options"`
	Variants    []variantRequest `json:"variants" binding:"required,min=1"`
}

func (h *ProductHandler) Create(c *gin.Context) {
	var req createProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	variants := make([]service.VariantInput, 0, len(req.Variants))
	for _, v := range req.Variants {
		variants = append(variants, service.VariantInput{
			SKU: v.SKU, OptionValues: v.OptionValues, PriceCents: v.PriceCents,
		})
	}

	product, err := h.svc.Create(c.Request.Context(), auth.TenantID(c), req.Title, req.Description, req.Options, variants)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toProductResponse(product))
}

func (h *ProductHandler) Get(c *gin.Context) {
	product, err := h.svc.Get(c.Request.Context(), auth.TenantID(c), c.Param("id"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toProductResponse(product))
}

func (h *ProductHandler) Activate(c *gin.Context) {
	product, err := h.svc.Activate(c.Request.Context(), auth.TenantID(c), c.Param("id"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toProductResponse(product))
}

// RegisterStorefrontRoutes mounts the buyer-facing public routes; wrap the
// group with cors.Public() (no credentials, any origin).
func (h *ProductHandler) RegisterStorefrontRoutes(rg *gin.RouterGroup) {
	rg.GET("/public/stores/:tenantId/products", h.PublicList)
}

func (h *ProductHandler) PublicList(c *gin.Context) {
	params := pagination.ParseParams(c)

	products, total, err := h.svc.ListPublic(c.Request.Context(), c.Param("tenantId"), params.Page, params.PageSize)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}

	items := make([]ProductResponse, 0, len(products))
	for _, p := range products {
		items = append(items, toProductResponse(p))
	}
	c.JSON(http.StatusOK, ListProductsResponse{
		Items:    items,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	})
}

func (h *ProductHandler) List(c *gin.Context) {
	params := pagination.ParseParams(c)

	products, total, err := h.svc.List(c.Request.Context(), auth.TenantID(c), params.Page, params.PageSize)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}

	items := make([]ProductResponse, 0, len(products))
	for _, p := range products {
		items = append(items, toProductResponse(p))
	}
	c.JSON(http.StatusOK, ListProductsResponse{
		Items:    items,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	})
}

func toProductResponse(p *domain.Product) ProductResponse {
	variants := make([]VariantResponse, 0, len(p.Variants))
	for _, v := range p.Variants {
		variants = append(variants, VariantResponse{
			ID: v.ID, SKU: v.SKU, OptionValues: v.OptionValues, PriceCents: v.PriceCents,
		})
	}
	options := p.Options
	if options == nil {
		options = []string{}
	}
	return ProductResponse{
		ID:          p.ID,
		Title:       p.Title,
		Description: p.Description,
		Status:      string(p.Status),
		Options:     options,
		Variants:    variants,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}
