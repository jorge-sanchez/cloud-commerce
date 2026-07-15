// Test Budget: 2 distinct behaviors × 2 = 4 max unit tests
// Actual: 4
//
// Behavior 1: NewCollection — empty handle is derived from the title;
//
//	explicit malformed handles are rejected
//
// Behavior 2: Slugify — collapses punctuation and accents to single hyphens
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

// ---------------------------------------------------------------------------
// Behavior 1: NewCollection handle rules
// ---------------------------------------------------------------------------

func TestNewCollection_EmptyHandle_DerivesFromTitle(t *testing.T) {
	c, err := domain.NewCollection("tenant-001", "Summer Sale 2026", "")

	require.NoError(t, err)
	assert.Equal(t, "summer-sale-2026", c.Handle)
	assert.Equal(t, "Summer Sale 2026", c.Title)
}

func TestNewCollection_MalformedExplicitHandle_ReturnsErrBadHandle(t *testing.T) {
	_, err := domain.NewCollection("tenant-001", "Summer Sale", "Summer Sale!")

	require.ErrorIs(t, err, domain.ErrBadHandle)
}

// ---------------------------------------------------------------------------
// Behavior 2: Slugify
// ---------------------------------------------------------------------------

func TestSlugify_PunctuatedTitle_CollapsesToSingleHyphens(t *testing.T) {
	assert.Equal(t, "polos-camisetas", domain.Slugify("  Polos & Camisetas!  "))
}

func TestSlugify_OnlyPunctuation_ReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", domain.Slugify("!!! ???"))
}
