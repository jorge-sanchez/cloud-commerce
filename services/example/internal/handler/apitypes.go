// apitypes.go owns the wire contract for this service's HTTP surface.
// Every 200/201 body is an exported, JSON-tagged struct declared here —
// never gin.H (the ratchet check in scripts/check_ginh_success_ratchet.sh
// fails the build otherwise). gin.H stays fine for error bodies, which are
// the uniform {code, message} shape from pkg/errors.
package handler

import "time"

// WidgetResponse is the single-widget wire shape.
type WidgetResponse struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListWidgetsResponse is the blessed offset-pagination envelope:
// {items, total, page, page_size}.
type ListWidgetsResponse struct {
	Items    []WidgetResponse `json:"items"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}
