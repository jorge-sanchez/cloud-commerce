package service

import (
	"context"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

// CollectionService is the application-service port for collections.
type CollectionService interface {
	Create(ctx context.Context, tenantID, title, handle string) (*domain.Collection, error)
	Get(ctx context.Context, tenantID, id string) (*domain.Collection, error)
	List(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Collection, int, error)
	AddProduct(ctx context.Context, tenantID, collectionID, productID string) error
	RemoveProduct(ctx context.Context, tenantID, collectionID, productID string) error
}

type collectionService struct {
	repo domain.CollectionRepository // required
}

// CollectionOption configures optional dependencies on the collection service.
type CollectionOption func(*collectionService)

// NewCollectionService constructs the collection application service.
func NewCollectionService(repo domain.CollectionRepository, opts ...CollectionOption) CollectionService {
	s := &collectionService{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *collectionService) Create(ctx context.Context, tenantID, title, handle string) (*domain.Collection, error) {
	collection, err := domain.NewCollection(tenantID, title, handle)
	if err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}
	return s.repo.SaveNew(ctx, tenantID, collection)
}

func (s *collectionService) Get(ctx context.Context, tenantID, id string) (*domain.Collection, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *collectionService) List(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Collection, int, error) {
	return s.repo.ListByTenant(ctx, tenantID, page, pageSize)
}

func (s *collectionService) AddProduct(ctx context.Context, tenantID, collectionID, productID string) error {
	return s.repo.AddProduct(ctx, tenantID, collectionID, productID)
}

func (s *collectionService) RemoveProduct(ctx context.Context, tenantID, collectionID, productID string) error {
	return s.repo.RemoveProduct(ctx, tenantID, collectionID, productID)
}
