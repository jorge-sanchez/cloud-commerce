// apitypes.go owns the wire contract for this service's HTTP surface.
// Every 200/201 body is an exported, JSON-tagged struct declared here —
// never gin.H. gin.H stays fine for error bodies ({code, message} from
// pkg/errors).
package handler

import "time"

// VariantResponse is the single-variant wire shape. Prices are integer
// minor units; the display currency comes from store settings.
type VariantResponse struct {
	ID           string   `json:"id"`
	SKU          string   `json:"sku"`
	OptionValues []string `json:"option_values"`
	PriceCents   int64    `json:"price_cents"`
}

// ProductResponse is the single-product wire shape.
type ProductResponse struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Options     []string          `json:"options"`
	Variants    []VariantResponse `json:"variants"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ListProductsResponse is the blessed offset-pagination envelope:
// {items, total, page, page_size}.
type ListProductsResponse struct {
	Items    []ProductResponse `json:"items"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}
