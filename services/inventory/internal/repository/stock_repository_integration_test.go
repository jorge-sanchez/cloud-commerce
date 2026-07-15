//go:build integration

// Test Budget: 5 distinct behaviors × 2 = 10 max integration tests
// Actual: 9
//
// Behavior 1: EnsureDefaultLocation + InitializeStock — idempotent under
//
//	replay (the consumer receives events at-least-once)
//
// Behavior 2: AdjustIfSufficient — entity-approved delta persists with the
//
//	adjusted event; entity-rejected delta returns ErrConflict unchanged
//
// Behavior 3: tenant scoping (ADR-001) — another tenant's stock is
//
//	ErrNotFound
//
// Behavior 4: ListStockByTenant — pages stock with totals
// Behavior 5: ApplyStockDeduction — clamps at zero and replays are no-ops
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
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/domain"
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
	tenantA  = "11111111-1111-1111-1111-111111111111"
	tenantB  = "22222222-2222-2222-2222-222222222222"
	variant1 = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	variant2 = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

func initializedStock(t *testing.T, repo *PostgresStockRepository) *domain.Location {
	t.Helper()
	location, err := repo.EnsureDefaultLocation(context.Background(), tenantA)
	require.NoError(t, err)
	require.NoError(t, repo.InitializeStock(context.Background(), tenantA, location.ID, []domain.StockInit{
		{VariantID: variant1, SKU: "TS-S"},
		{VariantID: variant2, SKU: "TS-M"},
	}))
	return location
}

// ---------------------------------------------------------------------------
// Behavior 1: initialization is idempotent under replay
// ---------------------------------------------------------------------------

func TestPostgresStockRepository_InitializeStock_Replayed_IsIdempotent(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresStockRepository(db)
	location := initializedStock(t, repo)

	// Replay the same event: same tenant, same variants (at-least-once).
	require.NoError(t, repo.InitializeStock(context.Background(), tenantA, location.ID, []domain.StockInit{
		{VariantID: variant1, SKU: "TS-S"},
	}))

	var rows int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM stock_levels`).Scan(&rows))
	assert.Equal(t, 2, rows, "replays must not create duplicate stock rows")
}

func TestPostgresStockRepository_EnsureDefaultLocation_CalledTwice_ReturnsSameLocation(t *testing.T) {
	repo := NewPostgresStockRepository(openMigratedDB(t))

	first, err := repo.EnsureDefaultLocation(context.Background(), tenantA)
	require.NoError(t, err)
	second, err := repo.EnsureDefaultLocation(context.Background(), tenantA)
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "the default location must be created exactly once")
	assert.True(t, first.IsDefault)
}

// ---------------------------------------------------------------------------
// Behavior 2: AdjustIfSufficient — the entity decides
// ---------------------------------------------------------------------------

func TestPostgresStockRepository_AdjustIfSufficient_Receive_PersistsAndRecordsEvent(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresStockRepository(db, WithEventRecorder(outbox.NewRecorder()))
	location := initializedStock(t, repo)

	level, err := repo.AdjustIfSufficient(context.Background(), tenantA, location.ID, variant1, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(10), level.OnHand)

	var eventCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM outbox WHERE event_type = $1`, domain.StockAdjustedEventType,
	).Scan(&eventCount))
	assert.Equal(t, 1, eventCount, "the adjusted event must be recorded with the change")
}

func TestPostgresStockRepository_AdjustIfSufficient_BelowZero_ConflictsAndKeepsRow(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPostgresStockRepository(db, WithEventRecorder(outbox.NewRecorder()))
	location := initializedStock(t, repo)

	_, err := repo.AdjustIfSufficient(context.Background(), tenantA, location.ID, variant1, -1)

	require.ErrorIs(t, err, apperrors.ErrConflict, "stock must never go below zero")

	var onHand int64
	require.NoError(t, db.QueryRow(
		`SELECT on_hand FROM stock_levels WHERE tenant_id = $1 AND variant_id = $2`,
		tenantA, variant1,
	).Scan(&onHand))
	assert.Equal(t, int64(0), onHand, "the rejected adjustment must not change the row")
}

// ---------------------------------------------------------------------------
// Behavior 3: tenant scoping — the cross-tenant negative case (ADR-001)
// ---------------------------------------------------------------------------

func TestPostgresStockRepository_AdjustIfSufficient_OtherTenantsStock_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresStockRepository(openMigratedDB(t))
	location := initializedStock(t, repo)

	_, err := repo.AdjustIfSufficient(context.Background(), tenantB, location.ID, variant1, 5)

	require.ErrorIs(t, err, apperrors.ErrNotFound, "another tenant's stock must be indistinguishable from missing")
}

// ---------------------------------------------------------------------------
// Behavior 4: listing pages with totals
// ---------------------------------------------------------------------------

func TestPostgresStockRepository_ListStockByTenant_ReturnsPageAndTotal(t *testing.T) {
	repo := NewPostgresStockRepository(openMigratedDB(t))
	initializedStock(t, repo)

	levels, total, err := repo.ListStockByTenant(context.Background(), tenantA, 1, 1)

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, levels, 1, "page size must be honored")
	assert.Equal(t, "TS-M", levels[0].SKU, "stock is ordered by SKU")
}

// ---------------------------------------------------------------------------
// Behavior 5: ApplyStockDeduction clamps and dedupes
// ---------------------------------------------------------------------------

func TestPostgresStockRepository_ApplyStockDeduction_OverOnHand_ClampsAtZero(t *testing.T) {
	repo := NewPostgresStockRepository(openMigratedDB(t))
	location := initializedStock(t, repo)
	_, err := repo.AdjustIfSufficient(context.Background(), tenantA, location.ID, variant1, 3)
	require.NoError(t, err)

	err = repo.ApplyStockDeduction(context.Background(), tenantA, "eeeeeeee-0000-0000-0000-000000000001",
		[]domain.StockDeduction{{VariantID: variant1, Qty: 5}})

	require.NoError(t, err, "a paid order must apply even when stock drifted")
	levels, _, err := repo.ListStockByTenant(context.Background(), tenantA, 1, 20)
	require.NoError(t, err)
	for _, l := range levels {
		if l.VariantID == variant1 {
			assert.Equal(t, int64(0), l.OnHand, "the deduction must clamp at zero, never negative")
		}
	}
}

func TestPostgresStockRepository_ApplyStockDeduction_ReplayedEvent_IsNoOp(t *testing.T) {
	repo := NewPostgresStockRepository(openMigratedDB(t))
	location := initializedStock(t, repo)
	_, err := repo.AdjustIfSufficient(context.Background(), tenantA, location.ID, variant1, 10)
	require.NoError(t, err)

	const eventID = "eeeeeeee-0000-0000-0000-000000000002"
	deduct := []domain.StockDeduction{{VariantID: variant1, Qty: 4}}
	require.NoError(t, repo.ApplyStockDeduction(context.Background(), tenantA, eventID, deduct))
	require.NoError(t, repo.ApplyStockDeduction(context.Background(), tenantA, eventID, deduct))

	levels, _, err := repo.ListStockByTenant(context.Background(), tenantA, 1, 20)
	require.NoError(t, err)
	for _, l := range levels {
		if l.VariantID == variant1 {
			assert.Equal(t, int64(6), l.OnHand, "a replayed event must deduct exactly once")
		}
	}
}
