//go:build integration

// Test Budget: 4 distinct behaviors × 2 = 8 max integration tests
// Actual: 6
//
// Behavior 1: AttachImageToProduct — persists the image and a media event;
//
//	images round-trip on GetByID in position order
//
// Behavior 2: ReorderProductImages — reorders positions atomically
// Behavior 3: RemoveProductImage — deletes one image and re-densifies; an
//
//	unknown image id is ErrNotFound
//
// Behavior 4: tenant scoping (ADR-001) — attaching under another tenant is
//
//	ErrNotFound (the product is indistinguishable from missing)
package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/outbox"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

func pngDraft(key string) domain.ImageDraft {
	return domain.ImageDraft{StorageKey: key, ContentType: "image/png", ByteSize: 2048, Width: 800, Height: 600, AltText: "front"}
}

// ---------------------------------------------------------------------------
// Behavior 1: AttachImageToProduct persists atomically with the media event
// ---------------------------------------------------------------------------

func TestPostgresProductRepository_AttachImageToProduct_PersistsImageAndEvent(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresProductRepository(db, WithEventRecorder(outbox.NewRecorder()))
	ctx := context.Background()
	saved, err := repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)

	updated, err := repo.AttachImageToProduct(ctx, tenantA, saved.ID, pngDraft("t/"+tenantA+"/p/"+saved.ID+"/one.png"))
	require.NoError(t, err)
	require.Len(t, updated.Images, 1, "the image must be attached")
	assert.NotEmpty(t, updated.Images[0].ID, "the database must assign the image ID")
	assert.Equal(t, 0, updated.Images[0].Position, "the first image is primary")

	got, err := repo.GetByID(ctx, tenantA, saved.ID)
	require.NoError(t, err)
	require.Len(t, got.Images, 1, "images must round-trip on read")
	assert.Equal(t, "front", got.Images[0].AltText)

	var eventCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE tenant_id = $1 AND event_type = $2`,
		tenantA, domain.ProductMediaUpdatedEventType,
	).Scan(&eventCount))
	assert.Equal(t, 1, eventCount, "the media event must be recorded with the change")
}

func TestPostgresProductRepository_AttachImageToProduct_OrdersByPosition(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	ctx := context.Background()
	saved, err := repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)

	_, err = repo.AttachImageToProduct(ctx, tenantA, saved.ID, pngDraft("t/"+tenantA+"/p/"+saved.ID+"/a.png"))
	require.NoError(t, err)
	updated, err := repo.AttachImageToProduct(ctx, tenantA, saved.ID, pngDraft("t/"+tenantA+"/p/"+saved.ID+"/b.png"))
	require.NoError(t, err)

	require.Len(t, updated.Images, 2, "both images must persist")
	assert.Equal(t, 0, updated.Images[0].Position)
	assert.Equal(t, 1, updated.Images[1].Position)
}

// ---------------------------------------------------------------------------
// Behavior 2: ReorderProductImages
// ---------------------------------------------------------------------------

func TestPostgresProductRepository_ReorderProductImages_SwapsPrimary(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	ctx := context.Background()
	saved, err := repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)
	_, err = repo.AttachImageToProduct(ctx, tenantA, saved.ID, pngDraft("t/"+tenantA+"/p/"+saved.ID+"/a.png"))
	require.NoError(t, err)
	two, err := repo.AttachImageToProduct(ctx, tenantA, saved.ID, pngDraft("t/"+tenantA+"/p/"+saved.ID+"/b.png"))
	require.NoError(t, err)

	reordered, err := repo.ReorderProductImages(ctx, tenantA, saved.ID, []string{two.Images[1].ID, two.Images[0].ID})
	require.NoError(t, err)

	assert.Equal(t, two.Images[1].ID, reordered.Images[0].ID, "the reorder must promote the second image to primary")
	got, err := repo.GetByID(ctx, tenantA, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, two.Images[1].ID, got.Images[0].ID, "the new order must persist")
}

// ---------------------------------------------------------------------------
// Behavior 3: RemoveProductImage
// ---------------------------------------------------------------------------

func TestPostgresProductRepository_RemoveProductImage_DropsAndReDensifies(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	ctx := context.Background()
	saved, err := repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)
	one, err := repo.AttachImageToProduct(ctx, tenantA, saved.ID, pngDraft("t/"+tenantA+"/p/"+saved.ID+"/a.png"))
	require.NoError(t, err)
	two, err := repo.AttachImageToProduct(ctx, tenantA, saved.ID, pngDraft("t/"+tenantA+"/p/"+saved.ID+"/b.png"))
	require.NoError(t, err)

	updated, err := repo.RemoveProductImage(ctx, tenantA, saved.ID, one.Images[0].ID)
	require.NoError(t, err)

	require.Len(t, updated.Images, 1, "one image must remain")
	assert.Equal(t, two.Images[1].ID, updated.Images[0].ID)
	assert.Equal(t, 0, updated.Images[0].Position, "the survivor must become primary")
}

func TestPostgresProductRepository_RemoveProductImage_UnknownID_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	ctx := context.Background()
	saved, err := repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)

	_, err = repo.RemoveProductImage(ctx, tenantA, saved.ID, "00000000-0000-0000-0000-000000000000")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Behavior 4: tenant scoping — the cross-tenant negative case (ADR-001)
// ---------------------------------------------------------------------------

func TestPostgresProductRepository_AttachImageToProduct_OtherTenant_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	ctx := context.Background()
	saved, err := repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)

	_, err = repo.AttachImageToProduct(ctx, tenantB, saved.ID, pngDraft("t/"+tenantB+"/p/"+saved.ID+"/a.png"))

	require.ErrorIs(t, err, apperrors.ErrNotFound, "another tenant's product must be indistinguishable from a missing one")
}
