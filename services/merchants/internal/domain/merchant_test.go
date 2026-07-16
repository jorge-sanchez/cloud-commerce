// Test Budget: 2 distinct behaviors × 2 = 4 max unit tests
// Actual: 4
//
// Behavior 1: UpdateProfile — valid profile is applied with email normalized;
//
//	invalid currency is rejected without mutating the entity
//
// Behavior 2: UpdateProfile — invalid timezone and invalid support email are
//
//	rejected with their domain sentinels
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jorge-sanchez/cloud-commerce/services/merchants/internal/domain"
)

func activeMerchant(t *testing.T) *domain.Merchant {
	t.Helper()
	m, err := domain.NewMerchant("Jorge's Store", "PE", "")
	require.NoError(t, err)
	return m
}

// ---------------------------------------------------------------------------
// Behavior 1: valid profile applies; invalid currency rejects without mutation
// ---------------------------------------------------------------------------

func TestMerchant_UpdateProfile_Valid_AppliesAndNormalizesEmail(t *testing.T) {
	m := activeMerchant(t)

	err := m.UpdateProfile("Tienda Jorge", domain.StoreSettings{
		Currency: "PEN", Timezone: "America/Lima", SupportEmail: "Help@Store.Test",
	})

	require.NoError(t, err)
	assert.Equal(t, "Tienda Jorge", m.Name)
	assert.Equal(t, "PEN", m.Settings.Currency)
	assert.Equal(t, "help@store.test", m.Settings.SupportEmail, "support email must be normalized")
}

func TestMerchant_UpdateProfile_LowercaseCurrency_ReturnsErrInvalidCurrencyAndKeepsEntity(t *testing.T) {
	m := activeMerchant(t)

	err := m.UpdateProfile("Tienda Jorge", domain.StoreSettings{Currency: "pen", Timezone: "UTC"})

	require.ErrorIs(t, err, domain.ErrInvalidCurrency)
	assert.Equal(t, "Jorge's Store", m.Name, "a rejected profile must not mutate the entity")
	assert.Equal(t, "USD", m.Settings.Currency)
}

// ---------------------------------------------------------------------------
// Behavior 2: timezone and support email validation
// ---------------------------------------------------------------------------

func TestMerchant_UpdateProfile_UnknownTimezone_ReturnsErrInvalidTimezone(t *testing.T) {
	m := activeMerchant(t)

	err := m.UpdateProfile("Jorge's Store", domain.StoreSettings{Currency: "USD", Timezone: "Mars/Olympus"})

	require.ErrorIs(t, err, domain.ErrInvalidTimezone)
}

func TestMerchant_UpdateProfile_BadSupportEmail_ReturnsErrInvalidEmail(t *testing.T) {
	m := activeMerchant(t)

	err := m.UpdateProfile("Jorge's Store", domain.StoreSettings{
		Currency: "USD", Timezone: "UTC", SupportEmail: "not-an-email",
	})

	require.ErrorIs(t, err, domain.ErrInvalidEmail)
}
