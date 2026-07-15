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
