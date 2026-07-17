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

// ImageResponse is the single-image wire shape. URL is composed from the
// configured media base at read time (position 0 is the primary/thumbnail).
type ImageResponse struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	AltText  string `json:"alt_text"`
	Position int    `json:"position"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

// ProductResponse is the single-product wire shape.
type ProductResponse struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Options     []string          `json:"options"`
	Variants    []VariantResponse `json:"variants"`
	Images      []ImageResponse   `json:"images"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// SignImageUploadResponse hands the admin a direct-to-storage upload URL and
// the object key it must send back to finalize the image.
type SignImageUploadResponse struct {
	UploadURL  string `json:"upload_url"`
	StorageKey string `json:"storage_key"`
	Method     string `json:"method"`
}

// ListProductsResponse is the blessed offset-pagination envelope:
// {items, total, page, page_size}.
type ListProductsResponse struct {
	Items    []ProductResponse `json:"items"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// PublicVariantResponse is the storefront's purchasable-variant lookup —
// what a cart needs to snapshot a line.
type PublicVariantResponse struct {
	VariantID    string   `json:"variant_id"`
	ProductID    string   `json:"product_id"`
	ProductTitle string   `json:"product_title"`
	SKU          string   `json:"sku"`
	OptionValues []string `json:"option_values"`
	PriceCents   int64    `json:"price_cents"`
}

// CollectionResponse is the single-collection wire shape. ProductIDs is
// populated on single fetches, empty in list responses.
type CollectionResponse struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Handle     string    `json:"handle"`
	ProductIDs []string  `json:"product_ids"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ListCollectionsResponse is the blessed offset-pagination envelope.
type ListCollectionsResponse struct {
	Items    []CollectionResponse `json:"items"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}
