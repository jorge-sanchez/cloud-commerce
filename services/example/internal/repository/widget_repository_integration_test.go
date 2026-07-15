//go:build integration

// Test Budget: 3 distinct behaviors × 2 = 6 max integration tests
// Actual: 4
//
// Behavior 1: SaveNew + GetByID — round-trips a widget through Postgres
// Behavior 2: PublishIfPublishable — entity-approved transition is persisted;
//
//	entity-rejected transition returns ErrConflict and writes nothing
//
// Behavior 3: ListByTenant — rows are scoped to the requesting tenant
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
	"github.com/jorge-sanchez/cloud-commerce/pkg/testdb"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/domain"
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

// ---------------------------------------------------------------------------
// Behavior 1: SaveNew + GetByID round-trip
// ---------------------------------------------------------------------------

func TestPostgresWidgetRepository_SaveNewAndGetByID_RoundTrips(t *testing.T) {
	repo := NewPostgresWidgetRepository(openMigratedDB(t))
	ctx := context.Background()

	saved, err := repo.SaveNew(ctx, tenantA, domain.NewWidget(tenantA, "hero banner"))
	require.NoError(t, err)
	require.NotEmpty(t, saved.ID, "the database must assign an ID")

	got, err := repo.GetByID(ctx, tenantA, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "hero banner", got.Name)
	assert.Equal(t, domain.WidgetStatusDraft, got.Status)
}

func TestPostgresWidgetRepository_GetByID_UnknownID_ReturnsNotFound(t *testing.T) {
	repo := NewPostgresWidgetRepository(openMigratedDB(t))

	_, err := repo.GetByID(context.Background(), tenantA, "33333333-3333-3333-3333-333333333333")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Behavior 2: PublishIfPublishable — the entity decides
// ---------------------------------------------------------------------------

func TestPostgresWidgetRepository_PublishIfPublishable_DraftThenRepeat_PersistsOnceThenConflicts(t *testing.T) {
	repo := NewPostgresWidgetRepository(openMigratedDB(t))
	ctx := context.Background()

	saved, err := repo.SaveNew(ctx, tenantA, domain.NewWidget(tenantA, "hero banner"))
	require.NoError(t, err)

	published, err := repo.PublishIfPublishable(ctx, tenantA, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.WidgetStatusPublished, published.Status)

	_, err = repo.PublishIfPublishable(ctx, tenantA, saved.ID)
	require.ErrorIs(t, err, apperrors.ErrConflict, "a second publish must be rejected by the entity")

	got, err := repo.GetByID(ctx, tenantA, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.WidgetStatusPublished, got.Status, "the rejected transition must not change the row")
}

// ---------------------------------------------------------------------------
// Behavior 3: ListByTenant is tenant-scoped
// ---------------------------------------------------------------------------

func TestPostgresWidgetRepository_ListByTenant_OtherTenantRows_AreExcluded(t *testing.T) {
	repo := NewPostgresWidgetRepository(openMigratedDB(t))
	ctx := context.Background()

	_, err := repo.SaveNew(ctx, tenantA, domain.NewWidget(tenantA, "widget A"))
	require.NoError(t, err)
	_, err = repo.SaveNew(ctx, tenantB, domain.NewWidget(tenantB, "widget B"))
	require.NoError(t, err)

	widgets, total, err := repo.ListByTenant(ctx, tenantA, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, widgets, 1, "only tenant A's widget may be returned")
	assert.Equal(t, "widget A", widgets[0].Name)
}
