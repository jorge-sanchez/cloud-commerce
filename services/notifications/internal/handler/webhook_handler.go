package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jorge-sanchez/cloud-commerce/pkg/auth"
	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/notifications/internal/service"
)

// WebhookEndpointResponse is the endpoint wire shape. Secret appears only
// in the creation response.
type WebhookEndpointResponse struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret,omitempty"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// ListWebhooksResponse is the blessed offset envelope (page always 1).
type ListWebhooksResponse struct {
	Items    []WebhookEndpointResponse `json:"items"`
	Total    int                       `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

// WebhookAdminHandler is the owner-only endpoint CRUD.
type WebhookAdminHandler struct {
	svc service.WebhookService // required
}

// NewWebhookAdminHandler constructs the handler.
func NewWebhookAdminHandler(svc service.WebhookService) *WebhookAdminHandler {
	return &WebhookAdminHandler{svc: svc}
}

// RegisterRoutes mounts the webhook management routes; the group must be
// wrapped with auth.Middleware.
func (h *WebhookAdminHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/webhooks", h.Register)
	rg.GET("/webhooks", h.List)
	rg.DELETE("/webhooks/:id", h.Remove)
}

type registerWebhookRequest struct {
	URL string `json:"url" binding:"required"`
}

func (h *WebhookAdminHandler) Register(c *gin.Context) {
	var req registerWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}
	e, err := h.svc.Register(c.Request.Context(), auth.TenantID(c), auth.Role(c), req.URL)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, WebhookEndpointResponse{
		ID: e.ID, URL: e.URL, Secret: e.Secret, Active: e.Active, CreatedAt: e.CreatedAt,
	})
}

func (h *WebhookAdminHandler) List(c *gin.Context) {
	endpoints, err := h.svc.List(c.Request.Context(), auth.TenantID(c), auth.Role(c))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	items := make([]WebhookEndpointResponse, 0, len(endpoints))
	for _, e := range endpoints {
		items = append(items, WebhookEndpointResponse{ID: e.ID, URL: e.URL, Active: e.Active, CreatedAt: e.CreatedAt})
	}
	c.JSON(http.StatusOK, ListWebhooksResponse{Items: items, Total: len(items), Page: 1, PageSize: len(items)})
}

func (h *WebhookAdminHandler) Remove(c *gin.Context) {
	if err := h.svc.Remove(c.Request.Context(), auth.TenantID(c), auth.Role(c), c.Param("id")); err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
