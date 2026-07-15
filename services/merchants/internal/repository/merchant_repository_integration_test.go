//go:build integration

// Test Budget: 5 distinct behaviors × 2 = 10 max integration tests
// Actual: 10
//
// Behavior 1: SaveNewWithOwner — persists merchant+owner atomically with the
//
//	signed-up event in the outbox; a taken email returns ErrConflict and
//	writes nothing new
//
// Behavior 2: GetUserByEmail — round-trips the stored owner for login
// Behavior 3: tenant scoping (ADR-001) — GetMerchantWithUser with another
//
//	tenant's user returns ErrNotFound
//
// Behavior 4: UpdateStoreProfile — entity-approved profile persists with the
//
//	settings event; entity-rejected profile returns ErrValidation and
//	writes neither
//
// Behavior 5: staff — SaveNewStaff + ListUsers round-trips (owner first);
//
//	DeleteUserIfRemovable removes staff but refuses the owner
package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/outbox"
	"github.com/jorge-sanchez/cloud-commerce/pkg/testdb"
	"github.com/jorge-sanchez/cloud-commerce/services/merchants/internal/domain"
)

// openMigratedDB provisions an isolated database (shared server in CI,
// testcontainer locally) and applies this service's up migrations.
func openMigratedDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn, cleanup := testdb.Open(t)
	t.Cleanup(cleanup)

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	migrations, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, migrations, "no up migrations found")
	for _, m := range migrations {
		ddl, err := os.ReadFile(m)
		require.NoError(t, err)
		_, err = db.Exec(string(ddl))
		require.NoError(t, err, "apply %s", m)
	}
	return db
}

func signUpFixture(t *testing.T, repo *PostgresMerchantRepository, store, email string) (*domain.Merchant, *domain.User) {
	t.Helper()
	m, err := domain.NewMerchant(store)
	require.NoError(t, err)
	owner, err := domain.NewOwner(email, "bcrypt-hash-placeholder")
	require.NoError(t, err)
	savedM, savedU, err := repo.SaveNewWithOwner(context.Background(), m, owner)
	require.NoError(t, err)
	return savedM, savedU
}

// ---------------------------------------------------------------------------
// Behavior 1: SaveNewWithOwner is atomic and records the event
// ---------------------------------------------------------------------------

func TestPostgresMerchantRepository_SaveNewWithOwner_Valid_PersistsAggregateAndEvent(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresMerchantRepository(db, WithEventRecorder(outbox.NewRecorder()))

	merchant, owner := signUpFixture(t, repo, "Jorge's Store", "owner@store.test")

	require.NotEmpty(t, merchant.ID, "the database must assign the merchant ID")
	assert.Equal(t, merchant.ID, owner.MerchantID, "the owner must belong to the new merchant")

	var eventCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE tenant_id = $1 AND event_type = $2`,
		merchant.ID, domain.MerchantSignedUpEventType,
	).Scan(&eventCount))
	assert.Equal(t, 1, eventCount, "the signed-up event must be recorded with the aggregate")
}

func TestPostgresMerchantRepository_SaveNewWithOwner_TakenEmail_ReturnsConflictAndWritesNothing(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresMerchantRepository(db, WithEventRecorder(outbox.NewRecorder()))
	signUpFixture(t, repo, "First Store", "owner@store.test")

	m, err := domain.NewMerchant("Second Store")
	require.NoError(t, err)
	owner, err := domain.NewOwner("owner@store.test", "bcrypt-hash-placeholder")
	require.NoError(t, err)

	_, _, err = repo.SaveNewWithOwner(context.Background(), m, owner)
	require.ErrorIs(t, err, apperrors.ErrConflict)

	var merchants, outboxRows int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM merchants`).Scan(&merchants))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM outbox`).Scan(&outboxRows))
	assert.Equal(t, 1, merchants, "the rejected sign-up must roll back the merchant insert")
	assert.Equal(t, 1, outboxRows, "the rejected sign-up must not record an event")
}

// ---------------------------------------------------------------------------
// Behavior 2: GetUserByEmail round-trips for login
// ---------------------------------------------------------------------------

func TestPostgresMerchantRepository_GetUserByEmail_Existing_ReturnsStoredHash(t *testing.T) {
	repo := NewPostgresMerchantRepository(openMigratedDB(t))
	_, saved := signUpFixture(t, repo, "Jorge's Store", "owner@store.test")

	user, err := repo.GetUserByEmail(context.Background(), "owner@store.test")

	require.NoError(t, err)
	assert.Equal(t, saved.ID, user.ID)
	assert.Equal(t, "bcrypt-hash-placeholder", user.PasswordHash)
	assert.Equal(t, domain.UserRoleOwner, user.Role)
}

func TestPostgresMerchantRepository_GetUserByEmail_Unknown_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresMerchantRepository(openMigratedDB(t))

	_, err := repo.GetUserByEmail(context.Background(), "nobody@store.test")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Behavior 3: tenant scoping — the cross-tenant negative case (ADR-001)
// ---------------------------------------------------------------------------

func TestPostgresMerchantRepository_GetMerchantWithUser_SameTenant_ReturnsBoth(t *testing.T) {
	repo := NewPostgresMerchantRepository(openMigratedDB(t))
	merchant, owner := signUpFixture(t, repo, "Jorge's Store", "owner@store.test")

	gotMerchant, gotUser, err := repo.GetMerchantWithUser(context.Background(), merchant.ID, owner.ID)

	require.NoError(t, err)
	assert.Equal(t, "Jorge's Store", gotMerchant.Name)
	assert.Equal(t, owner.ID, gotUser.ID)
}

func TestPostgresMerchantRepository_GetMerchantWithUser_OtherTenantsUser_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresMerchantRepository(openMigratedDB(t))
	_, ownerA := signUpFixture(t, repo, "Store A", "a@store.test")
	merchantB, _ := signUpFixture(t, repo, "Store B", "b@store.test")

	_, _, err := repo.GetMerchantWithUser(context.Background(), merchantB.ID, ownerA.ID)

	require.ErrorIs(t, err, apperrors.ErrNotFound, "another tenant's user must be indistinguishable from a missing one")
}

// ---------------------------------------------------------------------------
// Behavior 4: UpdateStoreProfile — the entity decides
// ---------------------------------------------------------------------------

func TestPostgresMerchantRepository_UpdateStoreProfile_Valid_PersistsSettingsAndEvent(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresMerchantRepository(db, WithEventRecorder(outbox.NewRecorder()))
	merchant, _ := signUpFixture(t, repo, "Jorge's Store", "owner@store.test")

	updated, err := repo.UpdateStoreProfile(context.Background(), merchant.ID, "Tienda Jorge", domain.StoreSettings{
		Currency: "PEN", Timezone: "America/Lima", SupportEmail: "help@store.test",
	})

	require.NoError(t, err)
	assert.Equal(t, "PEN", updated.Settings.Currency)

	got, err := repo.GetByID(context.Background(), merchant.ID)
	require.NoError(t, err)
	assert.Equal(t, "Tienda Jorge", got.Name, "the new profile must be persisted")
	assert.Equal(t, "America/Lima", got.Settings.Timezone)

	var eventCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE tenant_id = $1 AND event_type = $2`,
		merchant.ID, domain.MerchantSettingsUpdatedEventType,
	).Scan(&eventCount))
	assert.Equal(t, 1, eventCount, "the settings event must be recorded with the update")
}

func TestPostgresMerchantRepository_UpdateStoreProfile_EntityRejects_ReturnsValidationAndWritesNothing(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresMerchantRepository(db, WithEventRecorder(outbox.NewRecorder()))
	merchant, _ := signUpFixture(t, repo, "Jorge's Store", "owner@store.test")

	_, err := repo.UpdateStoreProfile(context.Background(), merchant.ID, "Jorge's Store", domain.StoreSettings{
		Currency: "pen", Timezone: "UTC",
	})

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)

	got, err := repo.GetByID(context.Background(), merchant.ID)
	require.NoError(t, err)
	assert.Equal(t, "USD", got.Settings.Currency, "the rejected profile must not change the row")

	var eventCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE event_type = $1`,
		domain.MerchantSettingsUpdatedEventType,
	).Scan(&eventCount))
	assert.Equal(t, 0, eventCount, "the rejected profile must not record an event")
}

// ---------------------------------------------------------------------------
// Behavior 5: staff round-trip and the owner-immutable rule
// ---------------------------------------------------------------------------

func TestPostgresMerchantRepository_SaveNewStaffAndListUsers_RoundTripsOwnerFirst(t *testing.T) {
	repo := NewPostgresMerchantRepository(openMigratedDB(t))
	merchant, _ := signUpFixture(t, repo, "Jorge's Store", "owner@store.test")
	ctx := context.Background()

	staff, err := domain.NewStaff("staff@store.test", "bcrypt-hash-placeholder")
	require.NoError(t, err)
	saved, err := repo.SaveNewStaff(ctx, merchant.ID, staff)
	require.NoError(t, err)
	require.NotEmpty(t, saved.ID, "the database must assign the staff ID")

	users, err := repo.ListUsers(ctx, merchant.ID)
	require.NoError(t, err)
	require.Len(t, users, 2, "owner and staff must both be listed")
	assert.Equal(t, domain.UserRoleOwner, users[0].Role, "the owner must come first")
	assert.Equal(t, "staff@store.test", users[1].Email)
}

func TestPostgresMerchantRepository_DeleteUserIfRemovable_StaffThenOwner_RemovesStaffRefusesOwner(t *testing.T) {
	repo := NewPostgresMerchantRepository(openMigratedDB(t))
	merchant, owner := signUpFixture(t, repo, "Jorge's Store", "owner@store.test")
	ctx := context.Background()

	staff, err := domain.NewStaff("staff@store.test", "bcrypt-hash-placeholder")
	require.NoError(t, err)
	saved, err := repo.SaveNewStaff(ctx, merchant.ID, staff)
	require.NoError(t, err)

	require.NoError(t, repo.DeleteUserIfRemovable(ctx, merchant.ID, saved.ID))

	err = repo.DeleteUserIfRemovable(ctx, merchant.ID, owner.ID)
	require.ErrorIs(t, err, apperrors.ErrConflict, "the owner must not be removable")

	users, err := repo.ListUsers(ctx, merchant.ID)
	require.NoError(t, err)
	require.Len(t, users, 1, "only the owner must remain")
	assert.Equal(t, domain.UserRoleOwner, users[0].Role)
}
