package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Media limits. Byte size and content type are re-checked server-side against
// the stored object (the client cannot be trusted); dimensions are cosmetic
// (layout hints) and bounded here only to reject the absurd.
const (
	MaxImagesPerProduct = 10
	MaxImageBytes       = 5 << 20 // 5 MiB
	MaxImageDimension   = 4096    // px, longest side we accept
)

// supportedImageTypes are the content types the storefront can render.
var supportedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
	"image/avif": true,
}

// Domain sentinel errors for media transitions.
var (
	ErrTooManyImages    = errors.New("product has reached its image limit")
	ErrImageTooLarge    = errors.New("image exceeds the maximum allowed size")
	ErrImageDimensions  = errors.New("image exceeds the maximum allowed dimensions")
	ErrUnsupportedImage = errors.New("object is not a supported image type")
	ErrEmptyStorageKey  = errors.New("image storage key must not be empty")
	ErrImageNotFound    = errors.New("image does not belong to this product")
	ErrReorderMismatch  = errors.New("reorder must list exactly the product's current images")
)

// Image is a product photo. The bytes live in object storage under StorageKey
// (ADR-013); this value object holds only the metadata. Position 0 is the
// primary/thumbnail image. VariantID is reserved (per-variant imagery is not
// wired at launch). The public URL is composed at read time from a configured
// base, so the serving host can move without a data migration.
type Image struct {
	ID          string
	ProductID   string
	TenantID    string
	VariantID   string // reserved; empty at launch
	StorageKey  string
	AltText     string
	Position    int
	ContentType string
	ByteSize    int64
	Width       int
	Height      int
	CreatedAt   time.Time
}

// ImageDraft is a finalized-but-unattached image: the browser has already
// uploaded the bytes, and the service has read the authoritative content type
// and size back from storage. The aggregate decides whether it may be attached.
type ImageDraft struct {
	StorageKey  string
	AltText     string
	ContentType string
	ByteSize    int64
	Width       int
	Height      int
}

// SupportedImageContentType reports whether a content type is a renderable
// image. Used when minting an upload URL and when finalizing.
func SupportedImageContentType(ct string) bool {
	return supportedImageTypes[strings.ToLower(strings.TrimSpace(ct))]
}

// AttachImage validates a finalized upload and appends it as the last (highest
// position) image. The aggregate owns its own limits — callers never insert an
// image row directly.
func (p *Product) AttachImage(d ImageDraft) (*Image, error) {
	if strings.TrimSpace(d.StorageKey) == "" {
		return nil, ErrEmptyStorageKey
	}
	if len(p.Images) >= MaxImagesPerProduct {
		return nil, fmt.Errorf("%w: %d", ErrTooManyImages, MaxImagesPerProduct)
	}
	if !SupportedImageContentType(d.ContentType) {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedImage, d.ContentType)
	}
	if d.ByteSize > MaxImageBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrImageTooLarge, d.ByteSize)
	}
	if d.Width > MaxImageDimension || d.Height > MaxImageDimension {
		return nil, fmt.Errorf("%w: %dx%d", ErrImageDimensions, d.Width, d.Height)
	}
	img := &Image{
		ProductID:   p.ID,
		TenantID:    p.TenantID,
		StorageKey:  strings.TrimSpace(d.StorageKey),
		AltText:     strings.TrimSpace(d.AltText),
		Position:    len(p.Images),
		ContentType: strings.ToLower(strings.TrimSpace(d.ContentType)),
		ByteSize:    d.ByteSize,
		Width:       d.Width,
		Height:      d.Height,
	}
	p.Images = append(p.Images, img)
	return img, nil
}

// ReorderImages reorders the collection to match orderedIDs exactly (position
// by index). Every current image must appear once and no unknown IDs may
// appear — a mismatch is a conflict, not a partial reorder.
func (p *Product) ReorderImages(orderedIDs []string) error {
	if len(orderedIDs) != len(p.Images) {
		return fmt.Errorf("%w: got %d ids for %d images", ErrReorderMismatch, len(orderedIDs), len(p.Images))
	}
	byID := make(map[string]*Image, len(p.Images))
	for _, img := range p.Images {
		byID[img.ID] = img
	}
	reordered := make([]*Image, 0, len(p.Images))
	seen := make(map[string]bool, len(p.Images))
	for i, id := range orderedIDs {
		img, ok := byID[id]
		if !ok || seen[id] {
			return fmt.Errorf("%w: %q", ErrReorderMismatch, id)
		}
		seen[id] = true
		img.Position = i
		reordered = append(reordered, img)
	}
	p.Images = reordered
	return nil
}

// RemoveImage drops one image and re-densifies positions so 0..n-1 stays
// contiguous with position 0 the primary.
func (p *Product) RemoveImage(imageID string) error {
	idx := -1
	for i, img := range p.Images {
		if img.ID == imageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("%w: %q", ErrImageNotFound, imageID)
	}
	p.Images = append(p.Images[:idx], p.Images[idx+1:]...)
	for i, img := range p.Images {
		img.Position = i
	}
	return nil
}

// PrimaryImage returns the position-0 image, or nil when the product has none.
func (p *Product) PrimaryImage() *Image {
	if len(p.Images) == 0 {
		return nil
	}
	return p.Images[0]
}

// Event type for media changes on the product aggregate.
const ProductMediaUpdatedEventType = "catalog.product_media_updated"

// ImagePayload is the wire shape of an image inside media events. It carries
// the storage key (not a URL): consumers compose the URL from the same
// configured base the read API uses, so the serving host stays swappable.
type ImagePayload struct {
	ImageID    string `json:"image_id"`
	StorageKey string `json:"storage_key"`
	AltText    string `json:"alt_text"`
	Position   int    `json:"position"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

// ProductMediaUpdatedEvent is emitted whenever a product's image collection
// changes (attach/reorder/remove). Additive: existing consumers ignore it.
type ProductMediaUpdatedEvent struct {
	ProductID string         `json:"product_id"`
	TenantID  string         `json:"tenant_id"`
	Images    []ImagePayload `json:"images"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// NewProductMediaUpdatedEvent builds the event from the persisted aggregate.
func NewProductMediaUpdatedEvent(p *Product, at time.Time) ProductMediaUpdatedEvent {
	images := make([]ImagePayload, 0, len(p.Images))
	for _, img := range p.Images {
		images = append(images, ImagePayload{
			ImageID:    img.ID,
			StorageKey: img.StorageKey,
			AltText:    img.AltText,
			Position:   img.Position,
			Width:      img.Width,
			Height:     img.Height,
		})
	}
	return ProductMediaUpdatedEvent{
		ProductID: p.ID,
		TenantID:  p.TenantID,
		Images:    images,
		UpdatedAt: at,
	}
}

// ObjectInfo is what object storage reports back about a stored object.
type ObjectInfo struct {
	ContentType string
	Size        int64
}

// MediaStore is the object-storage port for product media (ADR-013). The GCS
// adapter implements it; the seam keeps S3/R2/Cloudinary swappable, and the
// storefront logo/hero work (Tier-2) can reuse it. Bytes never flow through the
// service: the browser uploads directly to the signed URL.
//
// Uploads land under a staging prefix and are promoted to their permanent key
// only on finalize. Un-finalized objects therefore stay under staging, where a
// storage lifecycle rule can reap them without touching live images.
type MediaStore interface {
	// SignUpload returns a short-lived URL the browser PUTs bytes to for the
	// given (staging) object key and content type.
	SignUpload(ctx context.Context, key, contentType string) (string, error)
	// Promote moves a freshly uploaded object from its staging key to its
	// permanent key, returning the object's authoritative content type and
	// size. A never-completed upload is apperrors.ErrNotFound.
	Promote(ctx context.Context, srcKey, dstKey string) (ObjectInfo, error)
	// Delete removes the object; deleting a missing object is not an error.
	Delete(ctx context.Context, key string) error
}
