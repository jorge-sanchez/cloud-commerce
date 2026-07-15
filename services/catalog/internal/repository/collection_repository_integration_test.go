//go:build integration

// Test Budget: 3 distinct behaviors × 2 = 6 max integration tests
// Actual: 6
//
// Behavior 1: SaveNew + GetByID — round-trips a collection; a duplicate
//
//	handle in the tenant returns ErrConflict
//
// Behavior 2: AddProduct/RemoveProduct — membership round-trips and re-adds
//
//	are no-ops
//
// Behavior 3: tenant scoping (ADR-001) — another tenant's product cannot be
//
//	attached; another tenant's collection is ErrNotFound
package repository

import (
	"context"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

func collectionFixture(t *testing.T, repo *PostgresCollectionRepository, tenantID, title string) *domain.Collection {
	t.Helper()
	c, err := domain.NewCollection(tenantID, title, "")
	require.NoError(t, err)
	saved, err := repo.SaveNew(context.Background(), tenantID, c)
	require.NoError(t, err)
	return saved
}

// ---------------------------------------------------------------------------
// Behavior 1: round-trip and handle uniqueness
// ---------------------------------------------------------------------------

func TestPostgresCollectionRepository_SaveNewAndGetByID_RoundTrips(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresCollectionRepository(db)

	saved := collectionFixture(t, repo, tenantA, "Summer Sale")

	got, err := repo.GetByID(context.Background(), tenantA, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "Summer Sale", got.Title)
	assert.Equal(t, "summer-sale", got.Handle)
	assert.Empty(t, got.ProductIDs, "a fresh collection has no products")
}

func TestPostgresCollectionRepository_SaveNew_DuplicateHandle_ReturnsConflict(t *testing.T) {
	repo := NewPostgresCollectionRepository(openMigratedDB(t))
	collectionFixture(t, repo, tenantA, "Summer Sale")

	c, err := domain.NewCollection(tenantA, "Summer Sale", "")
	require.NoError(t, err)
	_, err = repo.SaveNew(context.Background(), tenantA, c)

	require.ErrorIs(t, err, apperrors.ErrConflict)
}

// ---------------------------------------------------------------------------
// Behavior 2: membership
// ---------------------------------------------------------------------------

func TestPostgresCollectionRepository_AddThenRemoveProduct_RoundTrips(t *testing.T) {
	db := openMigratedDB(t)
	products := NewPostgresProductRepository(db)
	repo := NewPostgresCollectionRepository(db)
	ctx := context.Background()

	product, err := products.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)
	collection := collectionFixture(t, repo, tenantA, "Summer Sale")

	require.NoError(t, repo.AddProduct(ctx, tenantA, collection.ID, product.ID))
	require.NoError(t, repo.AddProduct(ctx, tenantA, collection.ID, product.ID), "re-adding must be a no-op")

	got, err := repo.GetByID(ctx, tenantA, collection.ID)
	require.NoError(t, err)
	require.Len(t, got.ProductIDs, 1, "the product must be a member exactly once")
	assert.Equal(t, product.ID, got.ProductIDs[0])

	require.NoError(t, repo.RemoveProduct(ctx, tenantA, collection.ID, product.ID))
	got, err = repo.GetByID(ctx, tenantA, collection.ID)
	require.NoError(t, err)
	assert.Empty(t, got.ProductIDs, "the removed product must no longer be a member")
}

func TestPostgresCollectionRepository_AddProduct_UnknownProduct_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresCollectionRepository(openMigratedDB(t))
	collection := collectionFixture(t, repo, tenantA, "Summer Sale")

	err := repo.AddProduct(context.Background(), tenantA, collection.ID, "33333333-3333-3333-3333-333333333333")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Behavior 3: tenant scoping — the cross-tenant negative case (ADR-001)
// ---------------------------------------------------------------------------

func TestPostgresCollectionRepository_AddProduct_OtherTenantsProduct_ReturnsNotFound(t *testing.T) {
	db := openMigratedDB(t)
	products := NewPostgresProductRepository(db)
	repo := NewPostgresCollectionRepository(db)
	ctx := context.Background()

	foreign, err := domain.NewProduct(tenantB, "Foreign", "", nil)
	require.NoError(t, err)
	require.NoError(t, foreign.AddVariant("F-1", nil, 100))
	foreignSaved, err := products.SaveNewWithVariants(ctx, tenantB, foreign)
	require.NoError(t, err)

	collection := collectionFixture(t, repo, tenantA, "Summer Sale")

	err = repo.AddProduct(ctx, tenantA, collection.ID, foreignSaved.ID)
	require.ErrorIs(t, err, apperrors.ErrNotFound, "another tenant's product must not be attachable")
}

func TestPostgresCollectionRepository_GetByID_OtherTenantsCollection_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresCollectionRepository(openMigratedDB(t))
	saved := collectionFixture(t, repo, tenantA, "Summer Sale")

	_, err := repo.GetByID(context.Background(), tenantB, saved.ID)

	require.ErrorIs(t, err, apperrors.ErrNotFound, "another tenant's collection must be indistinguishable from missing")
}
