// Package handler holds the HTTP adapters (Gin). Handlers translate between
// the wire contract (apitypes.go) and the application service — no business
// rules here.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/jorge-sanchez/go-service-template/pkg/errors"
	"github.com/jorge-sanchez/go-service-template/pkg/pagination"
	"github.com/jorge-sanchez/go-service-template/services/example/internal/domain"
	"github.com/jorge-sanchez/go-service-template/services/example/internal/service"
)

// WidgetHandler exposes the widget HTTP surface.
type WidgetHandler struct {
	svc service.WidgetService
}

// NewWidgetHandler constructs the handler.
func NewWidgetHandler(svc service.WidgetService) *WidgetHandler {
	return &WidgetHandler{svc: svc}
}

// RegisterRoutes mounts the widget routes on the given router group.
func (h *WidgetHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/widgets", h.Create)
	rg.GET("/widgets", h.List)
	rg.GET("/widgets/:id", h.Get)
	rg.POST("/widgets/:id/publish", h.Publish)
}

// tenantID extracts the tenant from the request. The template trusts the
// X-Tenant-ID header — replace this with your real auth middleware.
func tenantID(c *gin.Context) string {
	return c.GetHeader("X-Tenant-ID")
}

type createWidgetRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *WidgetHandler) Create(c *gin.Context) {
	var req createWidgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	w, err := h.svc.Create(c.Request.Context(), tenantID(c), req.Name)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toWidgetResponse(w))
}

func (h *WidgetHandler) Get(c *gin.Context) {
	w, err := h.svc.Get(c.Request.Context(), tenantID(c), c.Param("id"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toWidgetResponse(w))
}

func (h *WidgetHandler) Publish(c *gin.Context) {
	w, err := h.svc.Publish(c.Request.Context(), tenantID(c), c.Param("id"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toWidgetResponse(w))
}

func (h *WidgetHandler) List(c *gin.Context) {
	params := pagination.ParseParams(c)

	widgets, total, err := h.svc.List(c.Request.Context(), tenantID(c), params.Page, params.PageSize)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}

	items := make([]WidgetResponse, 0, len(widgets))
	for _, w := range widgets {
		items = append(items, toWidgetResponse(w))
	}
	c.JSON(http.StatusOK, ListWidgetsResponse{
		Items:    items,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	})
}

func toWidgetResponse(w *domain.Widget) WidgetResponse {
	return WidgetResponse{
		ID:        w.ID,
		TenantID:  w.TenantID,
		Name:      w.Name,
		Status:    string(w.Status),
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
	}
}
