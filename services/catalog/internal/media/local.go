package media

import (
	"context"
	"sync"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

// LocalStore is a no-network stand-in for GCS used in local development
// (MEDIA_BUCKET unset). It fakes signing and remembers the content type minted
// for each key so finalize can complete without a real upload. Bytes are never
// stored — the composed public URLs will 404 locally, which is expected.
type LocalStore struct {
	baseURL string
	mu      sync.Mutex
	seen    map[string]string // key -> content type
}

var _ domain.MediaStore = (*LocalStore)(nil)

// NewLocalStore returns a dev store that composes fake signed URLs under
// baseURL.
func NewLocalStore(baseURL string) *LocalStore {
	return &LocalStore{baseURL: baseURL, seen: make(map[string]string)}
}

func (s *LocalStore) SignUpload(_ context.Context, key, contentType string) (string, error) {
	s.mu.Lock()
	s.seen[key] = contentType
	s.mu.Unlock()
	return s.baseURL + "/" + key + "?dev-signed=1", nil
}

func (s *LocalStore) Promote(_ context.Context, srcKey, dstKey string) (domain.ObjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ct, ok := s.seen[srcKey]
	if !ok {
		return domain.ObjectInfo{}, apperrors.ErrNotFound
	}
	delete(s.seen, srcKey)
	s.seen[dstKey] = ct
	return domain.ObjectInfo{ContentType: ct, Size: 1024}, nil
}

func (s *LocalStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	delete(s.seen, key)
	s.mu.Unlock()
	return nil
}
