// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 5
//
// Behavior 1: Create — builds the aggregate through the entity and persists
//
//	it once; an entity-rejected variant is a validation error with no write
//
// Behavior 2: Create — entity-rejected product options are a validation error
// Behavior 3: Activate/Get — delegate to repo and pass errors through
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

// ---------------------------------------------------------------------------
// Hand-rolled fakes at the port boundaries — no gomock, no testify/mock.
// ---------------------------------------------------------------------------

var _ domain.ProductRepository = (*fakeProductRepo)(nil)

type fakeProductRepo struct {
	saved  []*domain.Product
	result *domain.Product
	err    error
}

func (f *fakeProductRepo) SaveNewWithVariants(_ context.Context, tenantID string, p *domain.Product) (*domain.Product, error) {
	stored := *p
	stored.ID = "product-001"
	stored.TenantID = tenantID
	f.saved = append(f.saved, &stored)
	return &stored, f.err
}

func (f *fakeProductRepo) GetByID(_ context.Context, _, _ string) (*domain.Product, error) {
	return f.result, f.err
}

func (f *fakeProductRepo) ListByTenant(_ context.Context, _ string, _, _ int) ([]*domain.Product, int, error) {
	return nil, 0, f.err
}

func (f *fakeProductRepo) ActivateIfActivatable(_ context.Context, _, _ string) (*domain.Product, error) {
	return f.result, f.err
}

// ---------------------------------------------------------------------------
// Behavior 1: Create builds the aggregate and persists once
// ---------------------------------------------------------------------------

func TestProductService_Create_ValidInput_PersistsAggregateOnce(t *testing.T) {
	repo := &fakeProductRepo{}
	svc := NewProductService(repo)

	p, err := svc.Create(context.Background(), "tenant-001", "T-Shirt", "soft", []string{"Size"},
		[]VariantInput{{SKU: "TS-S", OptionValues: []string{"S"}, PriceCents: 1990}})

	require.NoError(t, err)
	assert.Equal(t, domain.ProductStatusDraft, p.Status)
	require.Len(t, repo.saved, 1, "exactly one aggregate must be written")
	require.Len(t, repo.saved[0].Variants, 1, "the variant must travel inside the aggregate")
}

func TestProductService_Create_DuplicateSKU_ReturnsValidationErrorAndNoWrite(t *testing.T) {
	repo := &fakeProductRepo{}
	svc := NewProductService(repo)

	_, err := svc.Create(context.Background(), "tenant-001", "T-Shirt", "", []string{"Size"},
		[]VariantInput{
			{SKU: "TS-S", OptionValues: []string{"S"}, PriceCents: 1990},
			{SKU: "TS-S", OptionValues: []string{"M"}, PriceCents: 1990},
		})

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
	require.Len(t, repo.saved, 0, "no aggregate may be written on validation failure")
}

// ---------------------------------------------------------------------------
// Behavior 2: entity-rejected options
// ---------------------------------------------------------------------------

func TestProductService_Create_TooManyOptions_ReturnsValidationError(t *testing.T) {
	svc := NewProductService(&fakeProductRepo{})

	_, err := svc.Create(context.Background(), "tenant-001", "T-Shirt", "",
		[]string{"Size", "Color", "Material", "Fit"}, nil)

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
}

// ---------------------------------------------------------------------------
// Behavior 3: delegation passes errors through
// ---------------------------------------------------------------------------

func TestProductService_Get_RepoReturnsNotFound_PassesErrorThrough(t *testing.T) {
	svc := NewProductService(&fakeProductRepo{err: apperrors.ErrNotFound})

	_, err := svc.Get(context.Background(), "tenant-001", "missing")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestProductService_Activate_RepoRejects_PassesConflictThrough(t *testing.T) {
	svc := NewProductService(&fakeProductRepo{err: apperrors.ErrConflict})

	_, err := svc.Activate(context.Background(), "tenant-001", "product-001")

	require.ErrorIs(t, err, apperrors.ErrConflict)
}
