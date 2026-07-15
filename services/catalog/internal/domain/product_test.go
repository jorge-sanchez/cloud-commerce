// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 6
//
// Behavior 1: NewProduct — valid options accepted; duplicate option names rejected
// Behavior 2: AddVariant — aggregate enforces SKU uniqueness and option arity
// Behavior 3: Activate — draft with variants activates; empty draft is rejected
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
)

func draftShirt(t *testing.T) *domain.Product {
	t.Helper()
	p, err := domain.NewProduct("tenant-001", "T-Shirt", "soft cotton", []string{"Size", "Color"})
	require.NoError(t, err)
	return p
}

// ---------------------------------------------------------------------------
// Behavior 1: NewProduct option validation
// ---------------------------------------------------------------------------

func TestNewProduct_ValidOptions_StartsDraft(t *testing.T) {
	p := draftShirt(t)

	assert.Equal(t, domain.ProductStatusDraft, p.Status)
	assert.Equal(t, []string{"Size", "Color"}, p.Options)
}

func TestNewProduct_DuplicateOptionNames_ReturnsErrBadOptionName(t *testing.T) {
	_, err := domain.NewProduct("tenant-001", "T-Shirt", "", []string{"Size", "size"})

	require.ErrorIs(t, err, domain.ErrBadOptionName)
}

// ---------------------------------------------------------------------------
// Behavior 2: AddVariant aggregate rules
// ---------------------------------------------------------------------------

func TestProduct_AddVariant_DuplicateSKU_ReturnsErrDuplicateSKU(t *testing.T) {
	p := draftShirt(t)
	require.NoError(t, p.AddVariant("TS-S-RED", []string{"S", "Red"}, 1990))

	err := p.AddVariant("ts-s-red", []string{"M", "Red"}, 1990)

	require.ErrorIs(t, err, domain.ErrDuplicateSKU, "SKU uniqueness must be case-insensitive")
	require.Len(t, p.Variants, 1, "the rejected variant must not be added")
}

func TestProduct_AddVariant_WrongOptionArity_ReturnsErrOptionArity(t *testing.T) {
	p := draftShirt(t)

	err := p.AddVariant("TS-S", []string{"S"}, 1990)

	require.ErrorIs(t, err, domain.ErrOptionArity)
}

// ---------------------------------------------------------------------------
// Behavior 3: Activate requires a draft with variants
// ---------------------------------------------------------------------------

func TestProduct_Activate_DraftWithVariant_BecomesActive(t *testing.T) {
	p := draftShirt(t)
	require.NoError(t, p.AddVariant("TS-S-RED", []string{"S", "Red"}, 1990))

	require.NoError(t, p.Activate())

	assert.Equal(t, domain.ProductStatusActive, p.Status)
}

func TestProduct_Activate_NoVariants_ReturnsErrNoVariants(t *testing.T) {
	p := draftShirt(t)

	err := p.Activate()

	require.ErrorIs(t, err, domain.ErrNoVariants)
	assert.Equal(t, domain.ProductStatusDraft, p.Status, "a rejected activation must not change status")
}
