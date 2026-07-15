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
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/domain"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/service"
)

// OrderHandler exposes both surfaces: the buyer cart/checkout routes
// (public, capability-based) and the merchant order views (authed).
type OrderHandler struct {
	svc service.OrderService
}

// NewOrderHandler constructs the handler.
func NewOrderHandler(svc service.OrderService) *OrderHandler {
	return &OrderHandler{svc: svc}
}

// RegisterBuyerRoutes mounts the public cart/checkout routes; the group
// must carry public CORS (buyers have no platform token).
func (h *OrderHandler) RegisterBuyerRoutes(rg *gin.RouterGroup) {
	rg.POST("/public/carts", h.CreateCart)
	rg.GET("/public/carts/:id", h.GetCart)
	rg.POST("/public/carts/:id/items", h.AddItem)
	rg.DELETE("/public/carts/:id/items/:variantId", h.RemoveItem)
	rg.POST("/public/carts/:id/checkout", h.Checkout)
}

// RegisterMerchantRoutes mounts the authed order views; the group must be
// wrapped with auth.Middleware (see cmd/main.go).
func (h *OrderHandler) RegisterMerchantRoutes(rg *gin.RouterGroup) {
	rg.GET("/orders", h.ListOrders)
	rg.GET("/orders/:id", h.GetOrder)
}

type createCartRequest struct {
	StoreHandle string `json:"store_handle" binding:"required"`
}

func (h *OrderHandler) CreateCart(c *gin.Context) {
	var req createCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	cart, err := h.svc.CreateCart(c.Request.Context(), req.StoreHandle)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toCartResponse(cart))
}

func (h *OrderHandler) GetCart(c *gin.Context) {
	cart, err := h.svc.GetCart(c.Request.Context(), c.Param("id"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toCartResponse(cart))
}

type addItemRequest struct {
	VariantID string `json:"variant_id" binding:"required"`
	Qty       int64  `json:"qty" binding:"required"`
}

func (h *OrderHandler) AddItem(c *gin.Context) {
	var req addItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	cart, err := h.svc.AddItem(c.Request.Context(), c.Param("id"), req.VariantID, req.Qty)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toCartResponse(cart))
}

func (h *OrderHandler) RemoveItem(c *gin.Context) {
	cart, err := h.svc.RemoveItem(c.Request.Context(), c.Param("id"), c.Param("variantId"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toCartResponse(cart))
}

type checkoutRequest struct {
	Email string `json:"email" binding:"required"`
}

func (h *OrderHandler) Checkout(c *gin.Context) {
	var req checkoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	order, err := h.svc.Checkout(c.Request.Context(), c.Param("id"), req.Email)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toOrderResponse(order))
}

func (h *OrderHandler) ListOrders(c *gin.Context) {
	params := pagination.ParseParams(c)

	orders, total, err := h.svc.ListOrders(c.Request.Context(), auth.TenantID(c), params.Page, params.PageSize)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}

	items := make([]OrderResponse, 0, len(orders))
	for _, o := range orders {
		items = append(items, toOrderResponse(o))
	}
	c.JSON(http.StatusOK, ListOrdersResponse{
		Items:    items,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	})
}

func (h *OrderHandler) GetOrder(c *gin.Context) {
	order, err := h.svc.GetOrder(c.Request.Context(), auth.TenantID(c), c.Param("id"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toOrderResponse(order))
}

func toItemResponses(items []domain.Item) []ItemResponse {
	out := make([]ItemResponse, 0, len(items))
	for _, it := range items {
		out = append(out, ItemResponse{
			VariantID: it.VariantID, SKU: it.SKU, Title: it.Title, PriceCents: it.PriceCents, Qty: it.Qty,
		})
	}
	return out
}

func toCartResponse(cart *domain.Cart) CartResponse {
	return CartResponse{
		ID:         cart.ID,
		TenantID:   cart.TenantID,
		Currency:   cart.Currency,
		Items:      toItemResponses(cart.Items),
		TotalCents: cart.TotalCents(),
	}
}

func toOrderResponse(o *domain.Order) OrderResponse {
	return OrderResponse{
		ID:         o.ID,
		Number:     o.Number,
		Email:      o.Email,
		Currency:   o.Currency,
		Items:      toItemResponses(o.Items),
		TotalCents: o.TotalCents,
		Status:     string(o.Status),
		CreatedAt:  o.CreatedAt,
	}
}
