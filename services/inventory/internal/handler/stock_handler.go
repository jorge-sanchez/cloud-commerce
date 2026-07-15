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
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/domain"
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/service"
)

// StockHandler exposes the inventory HTTP surface. Both owner and staff
// manage stock — no role gate here.
type StockHandler struct {
	svc service.StockService
}

// NewStockHandler constructs the handler.
func NewStockHandler(svc service.StockService) *StockHandler {
	return &StockHandler{svc: svc}
}

// RegisterRoutes mounts the inventory routes; the group must be wrapped
// with auth.Middleware (see cmd/main.go).
func (h *StockHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/stock", h.ListStock)
	rg.POST("/stock/adjust", h.AdjustStock)
	rg.GET("/locations", h.ListLocations)
	rg.POST("/locations", h.CreateLocation)
}

func (h *StockHandler) ListStock(c *gin.Context) {
	params := pagination.ParseParams(c)

	levels, total, err := h.svc.ListStock(c.Request.Context(), auth.TenantID(c), params.Page, params.PageSize)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}

	items := make([]StockLevelResponse, 0, len(levels))
	for _, s := range levels {
		items = append(items, toStockLevelResponse(s))
	}
	c.JSON(http.StatusOK, ListStockResponse{
		Items:    items,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	})
}

type adjustStockRequest struct {
	LocationID string `json:"location_id" binding:"required"`
	VariantID  string `json:"variant_id" binding:"required"`
	Delta      int64  `json:"delta" binding:"required"`
}

func (h *StockHandler) AdjustStock(c *gin.Context) {
	var req adjustStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	level, err := h.svc.AdjustStock(c.Request.Context(), auth.TenantID(c), req.LocationID, req.VariantID, req.Delta)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toStockLevelResponse(level))
}

type createLocationRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *StockHandler) CreateLocation(c *gin.Context) {
	var req createLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	location, err := h.svc.CreateLocation(c.Request.Context(), auth.TenantID(c), req.Name)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toLocationResponse(location))
}

func (h *StockHandler) ListLocations(c *gin.Context) {
	locations, err := h.svc.ListLocations(c.Request.Context(), auth.TenantID(c))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}

	items := make([]LocationResponse, 0, len(locations))
	for _, l := range locations {
		items = append(items, toLocationResponse(l))
	}
	c.JSON(http.StatusOK, ListLocationsResponse{Items: items, Total: len(items), Page: 1, PageSize: len(items)})
}

func toStockLevelResponse(s *domain.StockLevel) StockLevelResponse {
	return StockLevelResponse{
		ID:         s.ID,
		LocationID: s.LocationID,
		VariantID:  s.VariantID,
		SKU:        s.SKU,
		OnHand:     s.OnHand,
		UpdatedAt:  s.UpdatedAt,
	}
}

func toLocationResponse(l *domain.Location) LocationResponse {
	return LocationResponse{ID: l.ID, Name: l.Name, IsDefault: l.IsDefault, CreatedAt: l.CreatedAt}
}
