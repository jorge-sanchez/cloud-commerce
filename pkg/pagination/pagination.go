package pagination

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Params holds the parsed page-based pagination parameters from a request.
type Params struct {
	Page     int
	PageSize int
}

// ParseParams reads page and page_size from the gin query string and returns
// validated Params. Defaults: page=1, page_size=20. page_size is capped at 100.
// Invalid or missing values fall back to their defaults.
func ParseParams(c *gin.Context) Params {
	page := parseIntParam(c.Query("page"), DefaultPage)
	if page < 1 {
		page = DefaultPage
	}
	pageSize := parseIntParam(c.Query("page_size"), DefaultPageSize)
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return Params{Page: page, PageSize: pageSize}
}

// Response is the standard paginated JSON envelope returned by list endpoints.
type Response struct {
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Items    any `json:"items"`
}

// NewResponse constructs a Response with the provided pagination metadata and items slice.
func NewResponse(total, page, pageSize int, items any) Response {
	return Response{Total: total, Page: page, PageSize: pageSize, Items: items}
}

// parseIntParam converts a raw query string value to int. Returns defaultVal when
// the string is empty, non-numeric, or less than zero.
func parseIntParam(v string, defaultVal int) int {
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultVal
	}
	return n
}
