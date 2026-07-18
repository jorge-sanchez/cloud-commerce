// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 6
//
// Behavior 1: AttachImage — appends within limits; rejects unsupported type,
//
//	oversize bytes, and the per-product cap
//
// Behavior 2: ReorderImages — reindexes positions; a set mismatch is rejected
// Behavior 3: RemoveImage — drops one image and re-densifies positions; an
//
//	unknown id is ErrImageNotFound
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

func pngDraft(key string) domain.ImageDraft {
	return domain.ImageDraft{StorageKey: key, ContentType: "image/png", ByteSize: 1000, Width: 800, Height: 600}
}

// attachN attaches n images to a fresh product and assigns them synthetic IDs
// (the repository normally assigns these on insert).
func attachN(t *testing.T, n int) *domain.Product {
	t.Helper()
	p, err := domain.NewProduct("tenant-1", "Tee", "", nil)
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		img, err := p.AttachImage(pngDraft("t/tenant-1/p//k" + string(rune('a'+i)) + ".png"))
		require.NoError(t, err)
		img.ID = "img-" + string(rune('a'+i))
	}
	return p
}

// ---------------------------------------------------------------------------
// Behavior 1: AttachImage
// ---------------------------------------------------------------------------

func TestProduct_AttachImage_WithinLimits_AppendsAtNextPosition(t *testing.T) {
	p := attachN(t, 1)

	img, err := p.AttachImage(pngDraft("t/tenant-1/p//k2.png"))

	require.NoError(t, err)
	require.Len(t, p.Images, 2, "the image must be appended")
	assert.Equal(t, 1, img.Position, "a second image takes position 1")
	assert.Equal(t, "image/png", img.ContentType)
}

func TestProduct_AttachImage_UnsupportedTypeOversizeAndOverCap_AreRejected(t *testing.T) {
	p := attachN(t, 0)

	_, err := p.AttachImage(domain.ImageDraft{StorageKey: "k", ContentType: "application/pdf", ByteSize: 10})
	require.ErrorIs(t, err, domain.ErrUnsupportedImage)

	_, err = p.AttachImage(domain.ImageDraft{StorageKey: "k", ContentType: "image/png", ByteSize: domain.MaxImageBytes + 1})
	require.ErrorIs(t, err, domain.ErrImageTooLarge)

	full := attachN(t, domain.MaxImagesPerProduct)
	_, err = full.AttachImage(pngDraft("t/tenant-1/p//over.png"))
	require.ErrorIs(t, err, domain.ErrTooManyImages)
}

// ---------------------------------------------------------------------------
// Behavior 2: ReorderImages
// ---------------------------------------------------------------------------

func TestProduct_ReorderImages_ValidPermutation_ReindexesPositions(t *testing.T) {
	p := attachN(t, 3) // img-a, img-b, img-c at 0,1,2

	require.NoError(t, p.ReorderImages([]string{"img-c", "img-a", "img-b"}))

	assert.Equal(t, "img-c", p.Images[0].ID)
	assert.Equal(t, 0, p.Images[0].Position)
	assert.Equal(t, "img-b", p.Images[2].ID)
	assert.Equal(t, 2, p.Images[2].Position)
}

func TestProduct_ReorderImages_SetMismatch_ReturnsReorderMismatch(t *testing.T) {
	p := attachN(t, 2)

	err := p.ReorderImages([]string{"img-a", "img-a"}) // duplicate, missing img-b

	require.ErrorIs(t, err, domain.ErrReorderMismatch)
}

// ---------------------------------------------------------------------------
// Behavior 3: RemoveImage
// ---------------------------------------------------------------------------

func TestProduct_RemoveImage_Middle_ReDensifiesPositions(t *testing.T) {
	p := attachN(t, 3) // a,b,c

	require.NoError(t, p.RemoveImage("img-b"))

	require.Len(t, p.Images, 2, "one image must be removed")
	assert.Equal(t, "img-a", p.Images[0].ID)
	assert.Equal(t, 0, p.Images[0].Position)
	assert.Equal(t, "img-c", p.Images[1].ID)
	assert.Equal(t, 1, p.Images[1].Position, "positions must stay contiguous")
}

func TestProduct_RemoveImage_UnknownID_ReturnsImageNotFound(t *testing.T) {
	p := attachN(t, 1)

	err := p.RemoveImage("img-missing")

	require.ErrorIs(t, err, domain.ErrImageNotFound)
}
