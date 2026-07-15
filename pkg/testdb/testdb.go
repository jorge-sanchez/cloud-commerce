// Package testdb provisions an isolated PostgreSQL database for one
// integration-test scope (a package, or a single test) and hands back its DSN.
// Callers own the schema: apply migrations or DDL to the returned database
// exactly as before.
//
// Two provisioning modes:
//
//   - Shared server (CI): when TEST_POSTGRES_DSN is set, a uniquely-named
//     database is created on that server and dropped on cleanup. Creating a
//     database on a warm server takes ~100ms, versus multiple seconds for a
//     container start, and it removes the Docker-socket lookups that made
//     integration tests permanently miss Go's test cache.
//   - Testcontainer (local dev): when TEST_POSTGRES_DSN is unset, a dedicated
//     postgres:18-alpine container is started per call, exactly as the
//     per-service helpers did before this package existed.
//
// TEST_POSTGRES_DSN must be a URL-form DSN whose role can CREATE DATABASE,
// e.g. postgres://app:app@localhost:5432/app_test?sslmode=disable.
package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // "pgx" database/sql driver for the admin connection
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// EnvSharedDSN is the environment variable holding the shared-server admin DSN.
const EnvSharedDSN = "TEST_POSTGRES_DSN"

// createRetries bounds retry attempts for CREATE DATABASE, which can fail
// transiently when two databases are created concurrently from the same
// template (SQLSTATE 55006).
const createRetries = 3

// Open provisions an empty PostgreSQL database and returns its DSN plus a
// cleanup function. It fails the test on any provisioning error. Callers open
// their own connections (lib/pq, pgx, sqlx — the DSN is URL-form) and apply
// their own schema.
func Open(t testing.TB) (dsn string, cleanup func()) {
	t.Helper()
	dsn, cleanup, err := Provision(context.Background())
	if err != nil {
		t.Fatalf("testdb: %v", err)
	}
	return dsn, cleanup
}

// Provision is the error-returning form of Open for callers without a
// testing.TB, e.g. TestMain.
func Provision(ctx context.Context) (dsn string, cleanup func(), err error) {
	if adminDSN := os.Getenv(EnvSharedDSN); adminDSN != "" {
		return provisionShared(ctx, adminDSN)
	}
	return startContainer(ctx)
}

// provisionShared creates a uniquely-named database on the shared server
// behind adminDSN. Cleanup force-drops it, disconnecting any lingering
// sessions.
func provisionShared(ctx context.Context, adminDSN string) (string, func(), error) {
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return "", nil, fmt.Errorf("open admin connection to %s: %w", EnvSharedDSN, err)
	}

	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		_ = admin.Close()
		return "", nil, fmt.Errorf("random database suffix: %w", err)
	}
	name := fmt.Sprintf("app_it_%x", suffix)

	for attempt := 1; ; attempt++ {
		_, err = admin.ExecContext(ctx, "CREATE DATABASE "+name)
		if err == nil {
			break
		}
		if attempt == createRetries {
			_ = admin.Close()
			return "", nil, fmt.Errorf("create database %s: %w", name, err)
		}
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}

	dsn, err := swapDatabase(adminDSN, name)
	if err != nil {
		_ = admin.Close()
		return "", nil, fmt.Errorf("derive DSN for %s: %w", name, err)
	}

	cleanup := func() {
		_, _ = admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = admin.Close()
	}
	return dsn, cleanup, nil
}

// startContainer starts a dedicated postgres:18-alpine testcontainer,
// preserving the exact behavior of the per-service helpers this package
// replaces (tmpfs data dir and raised shm for speed).
func startContainer(ctx context.Context) (string, func(), error) {
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("app_test"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("app"),
		testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				ShmSize: 256 * 1024 * 1024,
				// postgres:18 moved the data mount point from
				// /var/lib/postgresql/data to /var/lib/postgresql — the old
				// path fails the image's entrypoint check.
				Tmpfs: map[string]string{"/var/lib/postgresql": "rw"},
			},
		}),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		return "", nil, fmt.Errorf("start postgres testcontainer: %w", err)
	}

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return "", nil, fmt.Errorf("get postgres connection string: %w", err)
	}

	return dsn, func() { _ = pgContainer.Terminate(ctx) }, nil
}

// swapDatabase returns adminDSN with its database path replaced by name.
func swapDatabase(adminDSN, name string) (string, error) {
	u, err := url.Parse(adminDSN)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}
