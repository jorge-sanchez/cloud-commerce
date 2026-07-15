// Package handler holds the HTTP adapters (Gin). Handlers translate between
// the wire contract (apitypes.go) and the application service — no business
// rules here.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jorge-sanchez/cloud-commerce/pkg/auth"
	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/merchants/internal/domain"
	"github.com/jorge-sanchez/cloud-commerce/services/merchants/internal/service"
)

// MerchantHandler exposes the identity HTTP surface.
type MerchantHandler struct {
	svc service.MerchantService
}

// NewMerchantHandler constructs the handler.
func NewMerchantHandler(svc service.MerchantService) *MerchantHandler {
	return &MerchantHandler{svc: svc}
}

// RegisterPublicRoutes mounts the unauthenticated identity routes.
func (h *MerchantHandler) RegisterPublicRoutes(rg *gin.RouterGroup) {
	rg.POST("/auth/signup", h.SignUp)
	rg.POST("/auth/login", h.LogIn)
}

// RegisterAuthedRoutes mounts routes that require a platform token; the
// group must be wrapped with auth.Middleware (see cmd/main.go).
func (h *MerchantHandler) RegisterAuthedRoutes(rg *gin.RouterGroup) {
	rg.GET("/me", h.Me)
	rg.GET("/store", h.GetStore)
	rg.PUT("/store", h.UpdateStore)
}

type signUpRequest struct {
	StoreName string `json:"store_name" binding:"required"`
	Email     string `json:"email" binding:"required"`
	Password  string `json:"password" binding:"required"`
}

func (h *MerchantHandler) SignUp(c *gin.Context) {
	var req signUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	session, err := h.svc.SignUp(c.Request.Context(), req.StoreName, req.Email, req.Password)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toSessionResponse(session))
}

type logInRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *MerchantHandler) LogIn(c *gin.Context) {
	var req logInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	session, err := h.svc.LogIn(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toSessionResponse(session))
}

func (h *MerchantHandler) Me(c *gin.Context) {
	merchant, user, err := h.svc.Me(c.Request.Context(), auth.TenantID(c), auth.UserID(c))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, MeResponse{
		Merchant: toMerchantResponse(merchant),
		User:     toUserResponse(user),
	})
}

func (h *MerchantHandler) GetStore(c *gin.Context) {
	merchant, err := h.svc.GetStore(c.Request.Context(), auth.TenantID(c))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toStoreResponse(merchant))
}

type updateStoreRequest struct {
	Name         string `json:"name" binding:"required"`
	Currency     string `json:"currency" binding:"required"`
	Timezone     string `json:"timezone" binding:"required"`
	SupportEmail string `json:"support_email"`
}

func (h *MerchantHandler) UpdateStore(c *gin.Context) {
	var req updateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	merchant, err := h.svc.UpdateStore(c.Request.Context(), auth.TenantID(c), req.Name, domain.StoreSettings{
		Currency:     req.Currency,
		Timezone:     req.Timezone,
		SupportEmail: req.SupportEmail,
	})
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toStoreResponse(merchant))
}

func toStoreResponse(m *domain.Merchant) StoreResponse {
	return StoreResponse{
		ID:           m.ID,
		Name:         m.Name,
		Status:       string(m.Status),
		Currency:     m.Settings.Currency,
		Timezone:     m.Settings.Timezone,
		SupportEmail: m.Settings.SupportEmail,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func toSessionResponse(s *service.Session) SessionResponse {
	return SessionResponse{
		Token:    s.Token,
		Merchant: toMerchantResponse(s.Merchant),
		User:     toUserResponse(s.User),
	}
}

func toMerchantResponse(m *domain.Merchant) MerchantResponse {
	return MerchantResponse{ID: m.ID, Name: m.Name, Status: string(m.Status), CreatedAt: m.CreatedAt}
}

func toUserResponse(u *domain.User) UserResponse {
	return UserResponse{ID: u.ID, Email: u.Email, Role: string(u.Role)}
}
