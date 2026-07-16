// apitypes.go owns the wire contract for this service's HTTP surface.
// Every 200/201 body is an exported, JSON-tagged struct declared here —
// never gin.H. gin.H stays fine for error bodies ({code, message} from
// pkg/errors).
package handler

import "time"

// ItemResponse is a priced cart/order line (snapshot, minor units).
type ItemResponse struct {
	VariantID  string `json:"variant_id"`
	SKU        string `json:"sku"`
	Title      string `json:"title"`
	PriceCents int64  `json:"price_cents"`
	Qty        int64  `json:"qty"`
}

// CartResponse is the buyer's cart wire shape. The cart ID is the buyer's
// capability — treat it like a secret.
type CartResponse struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenant_id"`
	Currency   string         `json:"currency"`
	Items      []ItemResponse `json:"items"`
	TotalCents int64          `json:"total_cents"`
}

// AddressResponse is the shipping address snapshot (RFC-001).
type AddressResponse struct {
	Name    string `json:"name"`
	Line1   string `json:"line1"`
	Line2   string `json:"line2"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Postal  string `json:"postal"`
	Country string `json:"country"`
	Phone   string `json:"phone"`
}

// OrderResponse is the order wire shape (buyer checkout result and
// merchant order views).
type OrderResponse struct {
	ID              string          `json:"id"`
	Number          int64           `json:"number"`
	Email           string          `json:"email"`
	Currency        string          `json:"currency"`
	ShippingMethod  string          `json:"shipping_method"`
	ShippingCents   int64           `json:"shipping_cents"`
	TaxCents        int64           `json:"tax_cents"`
	TaxName         string          `json:"tax_name"`
	TaxRateBps      int             `json:"tax_rate_bps"`
	TaxInclusive    bool            `json:"tax_inclusive"`
	LocationID      string          `json:"location_id"`
	ShippingAddress AddressResponse `json:"shipping_address"`
	Items           []ItemResponse  `json:"items"`
	TotalCents      int64           `json:"total_cents"`
	Status          string          `json:"status"`
	TrackingNumber  string          `json:"tracking_number"`
	Carrier         string          `json:"carrier"`
	CreatedAt       time.Time       `json:"created_at"`
}

// PaymentIntentResponse is the provider handoff for the buyer client.
type PaymentIntentResponse struct {
	Reference    string `json:"reference"`
	ClientSecret string `json:"client_secret"`
}

// ListOrdersResponse is the blessed offset-pagination envelope:
// {items, total, page, page_size}.
type ListOrdersResponse struct {
	Items    []OrderResponse `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

// DailySalesResponse is one day of revenue.
type DailySalesResponse struct {
	Date         string `json:"date"`
	RevenueCents int64  `json:"revenue_cents"`
	Orders       int    `json:"orders"`
}

// TopProductResponse is a best-selling variant.
type TopProductResponse struct {
	SKU          string `json:"sku"`
	Title        string `json:"title"`
	Units        int64  `json:"units"`
	RevenueCents int64  `json:"revenue_cents"`
}

// AnalyticsSummaryResponse is the merchant analytics read.
type AnalyticsSummaryResponse struct {
	Currency    string               `json:"currency"`
	Days        []DailySalesResponse `json:"days"`
	TopProducts []TopProductResponse `json:"top_products"`
}
