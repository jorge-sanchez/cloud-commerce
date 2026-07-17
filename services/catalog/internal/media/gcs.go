// Package media holds the object-storage adapters behind the domain.MediaStore
// port (ADR-013). Bytes never flow through this service: the browser uploads
// directly to a signed URL; the adapter only mints URLs and reads object
// metadata back.
package media

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
	iamcredentials "google.golang.org/api/iamcredentials/v1"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

// GCSStore signs uploads and reads object metadata on a Google Cloud Storage
// bucket. On Cloud Run there is no key file: V4 signing uses the runtime
// service account's IAM SignBlob (roles/iam.serviceAccountTokenCreator), so
// nothing is stored as a secret.
type GCSStore struct {
	bucket    *storage.BucketHandle // required
	signerSA  string                // required: SA email used as GoogleAccessID
	signBytes func([]byte) ([]byte, error)
	uploadTTL time.Duration
}

var _ domain.MediaStore = (*GCSStore)(nil)

// Option configures optional dependencies on the GCS store.
type Option func(*GCSStore)

// WithUploadTTL overrides how long a signed upload URL stays valid.
func WithUploadTTL(d time.Duration) Option {
	return func(s *GCSStore) { s.uploadTTL = d }
}

// NewGCSStore wires a store to a bucket, signing as signerSA via IAM SignBlob.
func NewGCSStore(ctx context.Context, bucketName, signerSA string, opts ...Option) (*GCSStore, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage client: %w", err)
	}
	iamSvc, err := iamcredentials.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("iamcredentials client: %w", err)
	}
	name := "projects/-/serviceAccounts/" + signerSA
	s := &GCSStore{
		bucket:    client.Bucket(bucketName),
		signerSA:  signerSA,
		uploadTTL: 5 * time.Minute,
		signBytes: func(b []byte) ([]byte, error) {
			resp, err := iamSvc.Projects.ServiceAccounts.SignBlob(name, &iamcredentials.SignBlobRequest{
				Payload: base64.StdEncoding.EncodeToString(b),
			}).Context(ctx).Do()
			if err != nil {
				return nil, err
			}
			return base64.StdEncoding.DecodeString(resp.SignedBlob)
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// SignUpload mints a V4 signed PUT URL. The content type is bound into the
// signature: the browser must PUT with exactly this Content-Type.
func (s *GCSStore) SignUpload(_ context.Context, key, contentType string) (string, error) {
	url, err := s.bucket.SignedURL(key, &storage.SignedURLOptions{
		Scheme:         storage.SigningSchemeV4,
		Method:         "PUT",
		GoogleAccessID: s.signerSA,
		SignBytes:      s.signBytes,
		ContentType:    contentType,
		Expires:        time.Now().Add(s.uploadTTL),
	})
	if err != nil {
		return "", apperrors.ErrInternal.Wrap(err)
	}
	return url, nil
}

// immutableCacheControl is set on promoted (permanent) objects. Keys are
// content-addressed (a UUID per upload), so an object never changes under a
// key and browsers/CDNs may cache it forever.
const immutableCacheControl = "public, max-age=31536000, immutable"

// Promote copies the staging object to its permanent key with an immutable
// cache header, then deletes the staging object. It returns the authoritative
// content type and size read from storage. A never-completed upload reads back
// as apperrors.ErrNotFound.
func (s *GCSStore) Promote(ctx context.Context, srcKey, dstKey string) (domain.ObjectInfo, error) {
	src := s.bucket.Object(srcKey)
	attrs, err := src.Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return domain.ObjectInfo{}, apperrors.ErrNotFound
	}
	if err != nil {
		return domain.ObjectInfo{}, apperrors.ErrInternal.Wrap(err)
	}

	copier := s.bucket.Object(dstKey).CopierFrom(src)
	copier.ContentType = attrs.ContentType
	copier.CacheControl = immutableCacheControl
	if _, err := copier.Run(ctx); err != nil {
		return domain.ObjectInfo{}, apperrors.ErrInternal.Wrap(err)
	}
	_ = src.Delete(ctx) // best-effort; the lifecycle rule reaps any leftover

	return domain.ObjectInfo{ContentType: attrs.ContentType, Size: attrs.Size}, nil
}

// Delete removes the object; a missing object is not an error (idempotent).
func (s *GCSStore) Delete(ctx context.Context, key string) error {
	err := s.bucket.Object(key).Delete(ctx)
	if err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
}
