// Package handler holds the HTTP adapters (Gin). Handlers translate between
// the wire contract (apitypes.go) and the application service — no business
// rules here.
package handler

import (
	"net/http"
	"strings"

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
	svc          service.ProductService
	mediaBaseURL string // public base for composing image URLs; may be empty
}

// HandlerOption configures optional dependencies on the handler.
type HandlerOption func(*ProductHandler)

// WithMediaBaseURL sets the public base URL image storage keys resolve against
// (e.g. https://storage.googleapis.com/<bucket>). Kept out of the DB so the
// serving host can move (direct GCS today, a CDN later) without a migration.
func WithMediaBaseURL(base string) HandlerOption {
	return func(h *ProductHandler) { h.mediaBaseURL = strings.TrimRight(base, "/") }
}

// NewProductHandler constructs the handler.
func NewProductHandler(svc service.ProductService, opts ...HandlerOption) *ProductHandler {
	h := &ProductHandler{svc: svc}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// RegisterRoutes mounts the product routes; the group must be wrapped with
// auth.Middleware (see cmd/main.go).
func (h *ProductHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/products", h.Create)
	rg.GET("/products", h.List)
	rg.GET("/products/:id", h.Get)
	rg.POST("/products/:id/activate", h.Activate)
	rg.POST("/products/:id/images:sign", h.SignImageUpload)
	rg.POST("/products/:id/images", h.AttachImage)
	rg.PATCH("/products/:id/images/reorder", h.ReorderImages)
	rg.DELETE("/products/:id/images/:imageId", h.RemoveImage)
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
	c.JSON(http.StatusCreated, h.toProductResponse(product))
}

func (h *ProductHandler) Get(c *gin.Context) {
	product, err := h.svc.Get(c.Request.Context(), auth.TenantID(c), c.Param("id"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toProductResponse(product))
}

func (h *ProductHandler) Activate(c *gin.Context) {
	product, err := h.svc.Activate(c.Request.Context(), auth.TenantID(c), c.Param("id"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toProductResponse(product))
}

type signImageUploadRequest struct {
	ContentType string `json:"content_type" binding:"required"`
}

func (h *ProductHandler) SignImageUpload(c *gin.Context) {
	var req signImageUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}
	key, url, err := h.svc.SignImageUpload(c.Request.Context(), auth.TenantID(c), c.Param("id"), req.ContentType)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, SignImageUploadResponse{UploadURL: url, StorageKey: key, Method: http.MethodPut})
}

type attachImageRequest struct {
	StorageKey string `json:"storage_key" binding:"required"`
	AltText    string `json:"alt_text"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

func (h *ProductHandler) AttachImage(c *gin.Context) {
	var req attachImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}
	product, err := h.svc.AttachImage(c.Request.Context(), auth.TenantID(c), c.Param("id"), service.AttachImageInput{
		StorageKey: req.StorageKey,
		AltText:    req.AltText,
		Width:      req.Width,
		Height:     req.Height,
	})
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, h.toProductResponse(product))
}

type reorderImagesRequest struct {
	ImageIDs []string `json:"image_ids" binding:"required"`
}

func (h *ProductHandler) ReorderImages(c *gin.Context) {
	var req reorderImagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}
	product, err := h.svc.ReorderImages(c.Request.Context(), auth.TenantID(c), c.Param("id"), req.ImageIDs)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toProductResponse(product))
}

func (h *ProductHandler) RemoveImage(c *gin.Context) {
	product, err := h.svc.RemoveImage(c.Request.Context(), auth.TenantID(c), c.Param("id"), c.Param("imageId"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toProductResponse(product))
}

// RegisterStorefrontRoutes mounts the buyer-facing public routes; wrap the
// group with cors.Public() (no credentials, any origin).
func (h *ProductHandler) RegisterStorefrontRoutes(rg *gin.RouterGroup) {
	rg.GET("/public/stores/:tenantId/products", h.PublicList)
	rg.GET("/public/stores/:tenantId/variants/:variantId", h.PublicVariant)
}

func (h *ProductHandler) PublicVariant(c *gin.Context) {
	v, err := h.svc.GetPublicVariant(c.Request.Context(), c.Param("tenantId"), c.Param("variantId"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	values := v.OptionValues
	if values == nil {
		values = []string{}
	}
	c.JSON(http.StatusOK, PublicVariantResponse{
		VariantID:    v.VariantID,
		ProductID:    v.ProductID,
		ProductTitle: v.ProductTitle,
		SKU:          v.SKU,
		OptionValues: values,
		PriceCents:   v.PriceCents,
	})
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
		items = append(items, h.toProductResponse(p))
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
		items = append(items, h.toProductResponse(p))
	}
	c.JSON(http.StatusOK, ListProductsResponse{
		Items:    items,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	})
}

func (h *ProductHandler) toProductResponse(p *domain.Product) ProductResponse {
	variants := make([]VariantResponse, 0, len(p.Variants))
	for _, v := range p.Variants {
		variants = append(variants, VariantResponse{
			ID: v.ID, SKU: v.SKU, OptionValues: v.OptionValues, PriceCents: v.PriceCents,
		})
	}
	images := make([]ImageResponse, 0, len(p.Images))
	for _, img := range p.Images {
		images = append(images, ImageResponse{
			ID:       img.ID,
			URL:      h.imageURL(img.StorageKey),
			AltText:  img.AltText,
			Position: img.Position,
			Width:    img.Width,
			Height:   img.Height,
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
		Images:      images,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// imageURL composes the public URL for a storage key against the configured
// media base.
func (h *ProductHandler) imageURL(storageKey string) string {
	if h.mediaBaseURL == "" {
		return "/" + storageKey
	}
	return h.mediaBaseURL + "/" + storageKey
}
