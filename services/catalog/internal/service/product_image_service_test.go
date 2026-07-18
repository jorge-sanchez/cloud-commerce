// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 5
//
// Behavior 1: SignImageUpload — unsupported content type is a validation error
//
//	before any storage call; a supported type mints a key under the
//	tenant/product namespace
//
// Behavior 2: AttachImage — a key outside the product namespace is rejected
//
//	without touching storage or the repo
//
// Behavior 3: AttachImage/RemoveImage — orphaned/removed objects are cleaned up
package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

var _ domain.MediaStore = (*fakeMediaStore)(nil)

type fakeMediaStore struct {
	signedURL    string
	signCalls    int
	info         domain.ObjectInfo
	promoteErr   error
	promoteCalls int
	promoted     [][2]string // {src, dst}
	deleted      []string
}

func (f *fakeMediaStore) SignUpload(_ context.Context, _, _ string) (string, error) {
	f.signCalls++
	return f.signedURL, nil
}

func (f *fakeMediaStore) Promote(_ context.Context, src, dst string) (domain.ObjectInfo, error) {
	f.promoteCalls++
	f.promoted = append(f.promoted, [2]string{src, dst})
	return f.info, f.promoteErr
}

func (f *fakeMediaStore) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

// ---------------------------------------------------------------------------
// Behavior 1: SignImageUpload
// ---------------------------------------------------------------------------

func TestProductService_SignImageUpload_UnsupportedType_ReturnsValidationAndNoStorageCall(t *testing.T) {
	media := &fakeMediaStore{signedURL: "https://signed"}
	svc := NewProductService(&fakeProductRepo{result: &domain.Product{ID: "p1"}}, WithMediaStore(media))

	_, _, err := svc.SignImageUpload(context.Background(), "tenant-1", "p1", "application/pdf")

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
	assert.Zero(t, media.signCalls, "storage must not be called for an unsupported type")
}

func TestProductService_SignImageUpload_Supported_ReturnsKeyUnderNamespace(t *testing.T) {
	media := &fakeMediaStore{signedURL: "https://signed"}
	svc := NewProductService(&fakeProductRepo{result: &domain.Product{ID: "p1"}}, WithMediaStore(media))

	key, url, err := svc.SignImageUpload(context.Background(), "tenant-1", "p1", "image/png")

	require.NoError(t, err)
	assert.Equal(t, "https://signed", url)
	assert.True(t, strings.HasPrefix(key, "staging/t/tenant-1/p/p1/"), "key must live under the staging tenant/product namespace, got %q", key)
	assert.True(t, strings.HasSuffix(key, ".png"), "key must carry the mapped extension, got %q", key)
}

// ---------------------------------------------------------------------------
// Behavior 2: AttachImage namespace guard
// ---------------------------------------------------------------------------

func TestProductService_AttachImage_KeyOutsideNamespace_ReturnsValidationAndNoStorageCall(t *testing.T) {
	media := &fakeMediaStore{info: domain.ObjectInfo{ContentType: "image/png", Size: 1000}}
	svc := NewProductService(&fakeProductRepo{}, WithMediaStore(media))

	_, err := svc.AttachImage(context.Background(), "tenant-1", "p1", AttachImageInput{
		StorageKey: "staging/t/other-tenant/p/p1/abc.png",
	})

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
	assert.Zero(t, media.promoteCalls, "a foreign key must be rejected before any storage call")
}

// ---------------------------------------------------------------------------
// Behavior 3: object cleanup
// ---------------------------------------------------------------------------

func TestProductService_AttachImage_RepoRejects_DeletesOrphanedObject(t *testing.T) {
	media := &fakeMediaStore{info: domain.ObjectInfo{ContentType: "image/png", Size: 1000}}
	repo := &fakeProductRepo{err: apperrors.ErrConflict}
	svc := NewProductService(repo, WithMediaStore(media))

	_, err := svc.AttachImage(context.Background(), "tenant-1", "p1", AttachImageInput{
		StorageKey: "staging/t/tenant-1/p/p1/abc.png",
	})

	require.Error(t, err)
	require.Len(t, media.deleted, 1, "the orphaned (promoted) object must be deleted")
	assert.Equal(t, "t/tenant-1/p/p1/abc.png", media.deleted[0], "cleanup targets the permanent key")
}
