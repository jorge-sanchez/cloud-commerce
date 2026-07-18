package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

// extensionFor maps a supported image content type to a file extension used in
// the object key.
var extensionFor = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/gif":  "gif",
	"image/avif": "avif",
}

// stagingPrefix is where uploads land before finalize. A storage lifecycle
// rule reaps un-promoted objects here without touching live images.
const stagingPrefix = "staging/"

// imageKeyPrefix is the permanent tenant/product namespace an image lives under
// once finalized. Signing mints a staging key under this namespace; finalize
// refuses keys outside it, so a browser cannot attach another tenant's object.
func imageKeyPrefix(tenantID, productID string) string {
	return fmt.Sprintf("t/%s/p/%s/", tenantID, productID)
}

func (s *productService) SignImageUpload(ctx context.Context, tenantID, productID, contentType string) (string, string, error) {
	if s.media == nil {
		return "", "", apperrors.ErrInternal
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if !domain.SupportedImageContentType(contentType) {
		return "", "", apperrors.ErrValidation.Wrap(fmt.Errorf("%w: %q", domain.ErrUnsupportedImage, contentType))
	}
	// The product must exist and be visible to this tenant, and have room for
	// another image, before we mint an upload URL.
	product, err := s.repo.GetByID(ctx, tenantID, productID)
	if err != nil {
		return "", "", err
	}
	if len(product.Images) >= domain.MaxImagesPerProduct {
		return "", "", apperrors.ErrValidation.Wrap(fmt.Errorf("%w: %d", domain.ErrTooManyImages, domain.MaxImagesPerProduct))
	}

	key := stagingPrefix + imageKeyPrefix(tenantID, productID) + uuid.NewString() + "." + extensionFor[contentType]
	url, err := s.media.SignUpload(ctx, key, contentType)
	if err != nil {
		return "", "", err
	}
	return key, url, nil
}

func (s *productService) AttachImage(ctx context.Context, tenantID, productID string, in AttachImageInput) (*domain.Product, error) {
	if s.media == nil {
		return nil, apperrors.ErrInternal
	}
	stagingKey := strings.TrimSpace(in.StorageKey)
	if !strings.HasPrefix(stagingKey, stagingPrefix+imageKeyPrefix(tenantID, productID)) {
		return nil, apperrors.ErrValidation.Wrap(fmt.Errorf("%w: key outside product namespace", domain.ErrEmptyStorageKey))
	}
	permanentKey := strings.TrimPrefix(stagingKey, stagingPrefix)

	// Promote the upload to its permanent key and read the authoritative
	// content type and size back from storage — the client is trusted for
	// neither. ErrNotFound when the upload never completed.
	info, err := s.media.Promote(ctx, stagingKey, permanentKey)
	if err != nil {
		return nil, err
	}

	product, err := s.repo.AttachImageToProduct(ctx, tenantID, productID, domain.ImageDraft{
		StorageKey:  permanentKey,
		AltText:     in.AltText,
		ContentType: info.ContentType,
		ByteSize:    info.Size,
		Width:       in.Width,
		Height:      in.Height,
	})
	if err != nil {
		// The promoted object is orphaned if it could not be attached; drop it.
		_ = s.media.Delete(ctx, permanentKey)
		return nil, err
	}
	return product, nil
}

func (s *productService) ReorderImages(ctx context.Context, tenantID, productID string, orderedIDs []string) (*domain.Product, error) {
	return s.repo.ReorderProductImages(ctx, tenantID, productID, orderedIDs)
}

func (s *productService) RemoveImage(ctx context.Context, tenantID, productID, imageID string) (*domain.Product, error) {
	if s.media == nil {
		return nil, apperrors.ErrInternal
	}
	// Find the object key before removing the row so we can delete the bytes.
	product, err := s.repo.GetByID(ctx, tenantID, productID)
	if err != nil {
		return nil, err
	}
	var key string
	for _, img := range product.Images {
		if img.ID == imageID {
			key = img.StorageKey
			break
		}
	}
	if key == "" {
		return nil, apperrors.ErrNotFound
	}

	updated, err := s.repo.RemoveProductImage(ctx, tenantID, productID, imageID)
	if err != nil {
		return nil, err
	}
	_ = s.media.Delete(ctx, key) // best-effort; a stray object is harmless
	return updated, nil
}
