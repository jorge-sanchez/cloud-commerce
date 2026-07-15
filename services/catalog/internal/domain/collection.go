package domain

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Collection domain sentinel errors.
var (
	ErrEmptyCollectionTitle = errors.New("collection title must not be empty")
	ErrBadHandle            = errors.New("handle must be lowercase letters, digits and single hyphens")
)

var handlePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Collection groups products for storefront navigation. Handle is the URL
// slug, unique per tenant. Membership is an association to the Product
// aggregate, not an owned child — products exist independently.
type Collection struct {
	ID         string
	TenantID   string
	Title      string
	Handle     string
	ProductIDs []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewCollection constructs a collection. An empty handle is derived from
// the title; an explicit handle must already be slug-shaped.
func NewCollection(tenantID, title, handle string) (*Collection, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, ErrEmptyCollectionTitle
	}
	if handle == "" {
		handle = Slugify(title)
	}
	if !handlePattern.MatchString(handle) {
		return nil, fmt.Errorf("%w: %q", ErrBadHandle, handle)
	}
	return &Collection{TenantID: tenantID, Title: title, Handle: handle}, nil
}

// Slugify derives a URL handle from a title: lowercase, non-alphanumerics
// collapsed to single hyphens.
func Slugify(title string) string {
	var b strings.Builder
	lastHyphen := true // suppress leading hyphen
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// CollectionRepository is the persistence port for collections.
type CollectionRepository interface {
	// SaveNew persists a collection. A handle already used by the tenant
	// returns apperrors.ErrConflict.
	SaveNew(ctx context.Context, tenantID string, c *Collection) (*Collection, error)
	// GetByID returns the collection with its product IDs, or
	// apperrors.ErrNotFound.
	GetByID(ctx context.Context, tenantID, id string) (*Collection, error)
	// ListByTenant returns one page of collections plus the total, newest
	// first (without product IDs — fetch one collection for those).
	ListByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*Collection, int, error)
	// AddProduct associates a product with a collection. Both must belong
	// to the tenant (apperrors.ErrNotFound otherwise); re-adding is a no-op.
	AddProduct(ctx context.Context, tenantID, collectionID, productID string) error
	// RemoveProduct removes the association; removing a non-member is a no-op.
	RemoveProduct(ctx context.Context, tenantID, collectionID, productID string) error
}
