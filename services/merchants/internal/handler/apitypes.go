// apitypes.go owns the wire contract for this service's HTTP surface.
// Every 200/201 body is an exported, JSON-tagged struct declared here —
// never gin.H. gin.H stays fine for error bodies ({code, message} from
// pkg/errors).
package handler

import "time"

// MerchantResponse is the single-merchant wire shape.
type MerchantResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// UserResponse is the merchant-user wire shape. Never carries the hash.
type UserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// SessionResponse is returned by sign-up (201) and login (200).
type SessionResponse struct {
	Token    string           `json:"token"`
	Merchant MerchantResponse `json:"merchant"`
	User     UserResponse     `json:"user"`
}

// MeResponse is the authenticated identity surface.
type MeResponse struct {
	Merchant MerchantResponse `json:"merchant"`
	User     UserResponse     `json:"user"`
}

// ListStaffResponse is the blessed offset envelope for the staff list
// (page is always 1 — staff lists are small; the envelope keeps the shape
// forward-compatible).
type ListStaffResponse struct {
	Items    []UserResponse `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// APIKeyResponse is the key metadata shape. Key (plaintext) is present
// only in the creation response — it is never retrievable again.
type APIKeyResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key,omitempty"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
}

// ListAPIKeysResponse is the blessed offset envelope (page always 1).
type ListAPIKeysResponse struct {
	Items    []APIKeyResponse `json:"items"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// ShippingMethodResponse is a flat shipping rate.
type ShippingMethodResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PriceCents int64  `json:"price_cents"`
	Active     bool   `json:"active"`
}

// ListShippingMethodsResponse is the blessed offset envelope (page 1).
type ListShippingMethodsResponse struct {
	Items    []ShippingMethodResponse `json:"items"`
	Total    int                      `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

// APITokenResponse is the key-exchange result.
type APITokenResponse struct {
	Token string `json:"token"`
}

// PublicStoreResponse is the unauthenticated storefront lookup shape —
// only what a buyer client needs to browse and display prices.
type PublicStoreResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Handle   string `json:"handle"`
	Currency string `json:"currency"`
	Timezone string `json:"timezone"`
	Country  string `json:"country"`
	TaxMode  string `json:"tax_mode"`
}

// TaxRateResponse is a merchant-defined jurisdiction rate (RFC-002).
type TaxRateResponse struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Country           string `json:"country"`
	Region            string `json:"region"`
	RateBps           int    `json:"rate_bps"`
	AppliesToShipping bool   `json:"applies_to_shipping"`
}

// ListTaxRatesResponse is the blessed offset envelope (page always 1).
type ListTaxRatesResponse struct {
	Items    []TaxRateResponse `json:"items"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// StoreResponse is the store profile wire shape.
type StoreResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Handle       string    `json:"handle"`
	Status       string    `json:"status"`
	Currency     string    `json:"currency"`
	Timezone     string    `json:"timezone"`
	SupportEmail string    `json:"support_email"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
