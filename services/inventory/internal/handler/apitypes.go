// apitypes.go owns the wire contract for this service's HTTP surface.
// Every 200/201 body is an exported, JSON-tagged struct declared here —
// never gin.H. gin.H stays fine for error bodies ({code, message} from
// pkg/errors).
package handler

import "time"

// LocationResponse is the single-location wire shape.
type LocationResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}

// ListLocationsResponse is the blessed offset envelope (page always 1 —
// location lists are small; the envelope keeps the shape forward-compatible).
type ListLocationsResponse struct {
	Items    []LocationResponse `json:"items"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

// StockLevelResponse is the single-stock-level wire shape.
type StockLevelResponse struct {
	ID         string    `json:"id"`
	LocationID string    `json:"location_id"`
	VariantID  string    `json:"variant_id"`
	SKU        string    `json:"sku"`
	OnHand     int64     `json:"on_hand"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ListStockResponse is the blessed offset-pagination envelope:
// {items, total, page, page_size}.
type ListStockResponse struct {
	Items    []StockLevelResponse `json:"items"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}
