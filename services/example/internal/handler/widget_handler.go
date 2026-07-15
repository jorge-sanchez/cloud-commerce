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
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/domain"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/service"
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

// tenantID reads the verified tenant injected by auth.Middleware (ADR-006).
// Routes must be mounted behind the middleware — see cmd/main.go.
func tenantID(c *gin.Context) string {
	return auth.TenantID(c)
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
