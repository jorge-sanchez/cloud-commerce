//go:build integration

// Test Budget: 6 distinct behaviors × 2 = 12 max integration tests
// Actual: 10
//
// Behavior 1: SaveNewWithVariants — persists the aggregate atomically with
//
//	the created event; a tenant-wide duplicate SKU returns ErrConflict and
//	rolls back the whole aggregate
//
// Behavior 2: GetByID/ListByTenant — round-trip products with variants,
//
//	newest first
//
// Behavior 3: tenant scoping (ADR-001) — another tenant's product is
//
//	ErrNotFound
//
// Behavior 4: ActivateIfActivatable — entity-approved transition persists
//
//	with the activated event; entity-rejected transition changes nothing
//
// Behavior 5: ListActiveByTenant — the storefront read excludes drafts
// Behavior 6: GetActiveVariant — resolves purchasable variants only
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
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/domain"
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

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

func shirtFixture(t *testing.T, sku string) *domain.Product {
	t.Helper()
	p, err := domain.NewProduct(tenantA, "T-Shirt", "soft cotton", []string{"Size"})
	require.NoError(t, err)
	require.NoError(t, p.AddVariant(sku, []string{"S"}, 1990))
	return p
}

// ---------------------------------------------------------------------------
// Behavior 1: SaveNewWithVariants is atomic and records the event
// ---------------------------------------------------------------------------

func TestPostgresProductRepository_SaveNewWithVariants_Valid_PersistsAggregateAndEvent(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresProductRepository(db, WithEventRecorder(outbox.NewRecorder()))

	saved, err := repo.SaveNewWithVariants(context.Background(), tenantA, shirtFixture(t, "TS-S"))

	require.NoError(t, err)
	require.NotEmpty(t, saved.ID, "the database must assign the product ID")
	require.Len(t, saved.Variants, 1, "the variant must be persisted with the aggregate")
	assert.NotEmpty(t, saved.Variants[0].ID, "the database must assign variant IDs")

	var eventCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE tenant_id = $1 AND event_type = $2`,
		tenantA, domain.ProductCreatedEventType,
	).Scan(&eventCount))
	assert.Equal(t, 1, eventCount, "the created event must be recorded with the aggregate")
}

func TestPostgresProductRepository_SaveNewWithVariants_DuplicateTenantSKU_ConflictsAndRollsBack(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresProductRepository(db, WithEventRecorder(outbox.NewRecorder()))
	_, err := repo.SaveNewWithVariants(context.Background(), tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)

	// A second product reusing the SKU — unique per tenant, not per product.
	_, err = repo.SaveNewWithVariants(context.Background(), tenantA, shirtFixture(t, "ts-s"))
	require.ErrorIs(t, err, apperrors.ErrConflict, "tenant-wide SKU uniqueness must be case-insensitive")

	var products, outboxRows int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&products))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM outbox`).Scan(&outboxRows))
	assert.Equal(t, 1, products, "the rejected aggregate must roll back entirely")
	assert.Equal(t, 1, outboxRows, "the rejected aggregate must not record an event")
}

// ---------------------------------------------------------------------------
// Behavior 2: round-trip with variants
// ---------------------------------------------------------------------------

func TestPostgresProductRepository_GetByID_RoundTripsVariants(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	saved, err := repo.SaveNewWithVariants(context.Background(), tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)

	got, err := repo.GetByID(context.Background(), tenantA, saved.ID)

	require.NoError(t, err)
	assert.Equal(t, "T-Shirt", got.Title)
	assert.Equal(t, []string{"Size"}, got.Options)
	require.Len(t, got.Variants, 1, "variants must round-trip")
	assert.Equal(t, "TS-S", got.Variants[0].SKU)
	assert.Equal(t, int64(1990), got.Variants[0].PriceCents)
}

func TestPostgresProductRepository_ListByTenant_ReturnsNewestFirstWithVariants(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	ctx := context.Background()
	_, err := repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-1"))
	require.NoError(t, err)
	second, err := repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-2"))
	require.NoError(t, err)

	products, total, err := repo.ListByTenant(ctx, tenantA, 1, 20)

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, products, 2, "both products must be listed")
	assert.Equal(t, second.ID, products[0].ID, "newest product must come first")
	require.Len(t, products[0].Variants, 1, "listed products must carry their variants")
}

// ---------------------------------------------------------------------------
// Behavior 3: tenant scoping — the cross-tenant negative case (ADR-001)
// ---------------------------------------------------------------------------

func TestPostgresProductRepository_GetByID_OtherTenantsProduct_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	saved, err := repo.SaveNewWithVariants(context.Background(), tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)

	_, err = repo.GetByID(context.Background(), tenantB, saved.ID)

	require.ErrorIs(t, err, apperrors.ErrNotFound, "another tenant's product must be indistinguishable from a missing one")
}

// ---------------------------------------------------------------------------
// Behavior 4: ActivateIfActivatable — the entity decides
// ---------------------------------------------------------------------------

func TestPostgresProductRepository_ActivateIfActivatable_DraftThenRepeat_ActivatesOnceThenConflicts(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresProductRepository(db, WithEventRecorder(outbox.NewRecorder()))
	saved, err := repo.SaveNewWithVariants(context.Background(), tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)

	activated, err := repo.ActivateIfActivatable(context.Background(), tenantA, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ProductStatusActive, activated.Status)

	var eventCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE event_type = $1`, domain.ProductActivatedEventType,
	).Scan(&eventCount))
	assert.Equal(t, 1, eventCount, "the activated event must be recorded with the transition")

	_, err = repo.ActivateIfActivatable(context.Background(), tenantA, saved.ID)
	require.ErrorIs(t, err, apperrors.ErrConflict, "a second activation must be rejected by the entity")
}

// ---------------------------------------------------------------------------
// Behavior 5: the storefront read excludes drafts
// ---------------------------------------------------------------------------

func TestPostgresProductRepository_ListActiveByTenant_MixedStatuses_ReturnsOnlyActive(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	ctx := context.Background()

	activated, err := repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-LIVE"))
	require.NoError(t, err)
	_, err = repo.ActivateIfActivatable(ctx, tenantA, activated.ID)
	require.NoError(t, err)
	_, err = repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-DRAFT"))
	require.NoError(t, err)

	products, total, err := repo.ListActiveByTenant(ctx, tenantA, 1, 20)

	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, products, 1, "drafts must not appear on the storefront")
	assert.Equal(t, "TS-LIVE", products[0].Variants[0].SKU)
}

// ---------------------------------------------------------------------------
// Behavior 6: GetActiveVariant resolves purchasable variants only
// ---------------------------------------------------------------------------

func TestPostgresProductRepository_GetActiveVariant_ActiveProduct_ReturnsSnapshot(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	ctx := context.Background()
	saved, err := repo.SaveNewWithVariants(ctx, tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)
	_, err = repo.ActivateIfActivatable(ctx, tenantA, saved.ID)
	require.NoError(t, err)

	got, err := repo.GetActiveVariant(ctx, tenantA, saved.Variants[0].ID)

	require.NoError(t, err)
	assert.Equal(t, "T-Shirt", got.ProductTitle)
	assert.Equal(t, int64(1990), got.PriceCents)
}

func TestPostgresProductRepository_GetActiveVariant_DraftProduct_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresProductRepository(openMigratedDB(t))
	saved, err := repo.SaveNewWithVariants(context.Background(), tenantA, shirtFixture(t, "TS-S"))
	require.NoError(t, err)

	_, err = repo.GetActiveVariant(context.Background(), tenantA, saved.Variants[0].ID)

	require.ErrorIs(t, err, apperrors.ErrNotFound, "draft products must not be purchasable")
}
