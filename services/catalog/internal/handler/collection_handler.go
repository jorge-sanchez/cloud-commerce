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

// CollectionHandler exposes the collection HTTP surface.
type CollectionHandler struct {
	svc service.CollectionService
}

// NewCollectionHandler constructs the handler.
func NewCollectionHandler(svc service.CollectionService) *CollectionHandler {
	return &CollectionHandler{svc: svc}
}

// RegisterRoutes mounts the collection routes; the group must be wrapped
// with auth.Middleware (see cmd/main.go).
func (h *CollectionHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/collections", h.Create)
	rg.GET("/collections", h.List)
	rg.GET("/collections/:id", h.Get)
	rg.PUT("/collections/:id/products/:productId", h.AddProduct)
	rg.DELETE("/collections/:id/products/:productId", h.RemoveProduct)
}

type createCollectionRequest struct {
	Title  string `json:"title" binding:"required"`
	Handle string `json:"handle"`
}

func (h *CollectionHandler) Create(c *gin.Context) {
	var req createCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	collection, err := h.svc.Create(c.Request.Context(), auth.TenantID(c), req.Title, req.Handle)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toCollectionResponse(collection))
}

func (h *CollectionHandler) Get(c *gin.Context) {
	collection, err := h.svc.Get(c.Request.Context(), auth.TenantID(c), c.Param("id"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toCollectionResponse(collection))
}

func (h *CollectionHandler) List(c *gin.Context) {
	params := pagination.ParseParams(c)

	collections, total, err := h.svc.List(c.Request.Context(), auth.TenantID(c), params.Page, params.PageSize)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}

	items := make([]CollectionResponse, 0, len(collections))
	for _, col := range collections {
		items = append(items, toCollectionResponse(col))
	}
	c.JSON(http.StatusOK, ListCollectionsResponse{
		Items:    items,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	})
}

func (h *CollectionHandler) AddProduct(c *gin.Context) {
	err := h.svc.AddProduct(c.Request.Context(), auth.TenantID(c), c.Param("id"), c.Param("productId"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CollectionHandler) RemoveProduct(c *gin.Context) {
	err := h.svc.RemoveProduct(c.Request.Context(), auth.TenantID(c), c.Param("id"), c.Param("productId"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func toCollectionResponse(col *domain.Collection) CollectionResponse {
	productIDs := col.ProductIDs
	if productIDs == nil {
		productIDs = []string{}
	}
	return CollectionResponse{
		ID:         col.ID,
		Title:      col.Title,
		Handle:     col.Handle,
		ProductIDs: productIDs,
		CreatedAt:  col.CreatedAt,
		UpdatedAt:  col.UpdatedAt,
	}
}
