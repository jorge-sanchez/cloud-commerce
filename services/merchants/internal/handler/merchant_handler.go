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
	rg.POST("/auth/token", h.ExchangeAPIToken)
}

// RegisterStorefrontRoutes mounts the buyer-facing public routes; wrap the
// group with cors.Public() (no credentials, any origin).
func (h *MerchantHandler) RegisterStorefrontRoutes(rg *gin.RouterGroup) {
	rg.GET("/public/stores/:handle", h.PublicStore)
}

type createAPIKeyRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *MerchantHandler) CreateAPIKey(c *gin.Context) {
	var req createAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}
	key, plaintext, err := h.svc.CreateAPIKey(c.Request.Context(), auth.TenantID(c), actorRole(c), req.Name)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, APIKeyResponse{
		ID: key.ID, Name: key.Name, Key: plaintext, Revoked: false, CreatedAt: key.CreatedAt,
	})
}

func (h *MerchantHandler) ListAPIKeys(c *gin.Context) {
	keys, err := h.svc.ListAPIKeys(c.Request.Context(), auth.TenantID(c), actorRole(c))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	items := make([]APIKeyResponse, 0, len(keys))
	for _, k := range keys {
		items = append(items, APIKeyResponse{ID: k.ID, Name: k.Name, Revoked: k.Revoked(), CreatedAt: k.CreatedAt})
	}
	c.JSON(http.StatusOK, ListAPIKeysResponse{Items: items, Total: len(items), Page: 1, PageSize: len(items)})
}

func (h *MerchantHandler) RevokeAPIKey(c *gin.Context) {
	if err := h.svc.RevokeAPIKey(c.Request.Context(), auth.TenantID(c), actorRole(c), c.Param("id")); err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type apiTokenRequest struct {
	APIKey string `json:"api_key" binding:"required"`
}

// ExchangeAPIToken is public: a valid key yields a short-lived platform
// token with the api role.
func (h *MerchantHandler) ExchangeAPIToken(c *gin.Context) {
	var req apiTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}
	token, err := h.svc.ExchangeAPIKey(c.Request.Context(), req.APIKey)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, APITokenResponse{Token: token})
}

func (h *MerchantHandler) PublicStore(c *gin.Context) {
	merchant, err := h.svc.ResolveStore(c.Request.Context(), c.Param("handle"))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, PublicStoreResponse{
		ID:       merchant.ID,
		Name:     merchant.Name,
		Handle:   merchant.Handle,
		Currency: merchant.Settings.Currency,
		Timezone: merchant.Settings.Timezone,
	})
}

// RegisterAuthedRoutes mounts routes that require a platform token; the
// group must be wrapped with auth.Middleware (see cmd/main.go).
func (h *MerchantHandler) RegisterAuthedRoutes(rg *gin.RouterGroup) {
	rg.GET("/me", h.Me)
	rg.GET("/store", h.GetStore)
	rg.PUT("/store", h.UpdateStore)
	rg.POST("/staff", h.AddStaff)
	rg.GET("/staff", h.ListStaff)
	rg.DELETE("/staff/:id", h.RemoveStaff)
	rg.POST("/api-keys", h.CreateAPIKey)
	rg.GET("/api-keys", h.ListAPIKeys)
	rg.DELETE("/api-keys/:id", h.RevokeAPIKey)
}

// actorRole reads the verified role claim. Authorization decisions live in
// the service/domain — the handler only translates.
func actorRole(c *gin.Context) domain.UserRole {
	return domain.UserRole(auth.Role(c))
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

	merchant, err := h.svc.UpdateStore(c.Request.Context(), auth.TenantID(c), actorRole(c), req.Name, domain.StoreSettings{
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

type addStaffRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *MerchantHandler) AddStaff(c *gin.Context) {
	var req addStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.RespondError(c, apperrors.ErrValidation.Wrap(err))
		return
	}

	staff, err := h.svc.AddStaff(c.Request.Context(), auth.TenantID(c), actorRole(c), req.Email, req.Password)
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toUserResponse(staff))
}

func (h *MerchantHandler) ListStaff(c *gin.Context) {
	users, err := h.svc.ListStaff(c.Request.Context(), auth.TenantID(c), actorRole(c))
	if err != nil {
		apperrors.RespondError(c, err)
		return
	}

	items := make([]UserResponse, 0, len(users))
	for _, u := range users {
		items = append(items, toUserResponse(u))
	}
	c.JSON(http.StatusOK, ListStaffResponse{Items: items, Total: len(items), Page: 1, PageSize: len(items)})
}

func (h *MerchantHandler) RemoveStaff(c *gin.Context) {
	if err := h.svc.RemoveStaff(c.Request.Context(), auth.TenantID(c), actorRole(c), c.Param("id")); err != nil {
		apperrors.RespondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func toStoreResponse(m *domain.Merchant) StoreResponse {
	return StoreResponse{
		ID:           m.ID,
		Name:         m.Name,
		Handle:       m.Handle,
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
