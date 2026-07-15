//go:build integration

package testdb_test

// Test Budget: 1 distinct behavior × 2 = 2 max integration tests
// Actual: 2
//
// Behavior 1: Open provisions a usable, isolated, empty database (shared
// server when TEST_POSTGRES_DSN is set, testcontainer otherwise) and two
// Opens never hand out the same database.

import (
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/jorge-sanchez/go-service-template/pkg/testdb"
)

func TestOpen_ProvisionsEmptyUsableDatabase(t *testing.T) {
	dsn, cleanup := testdb.Open(t)
	defer cleanup()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open provisioned database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE smoke (id INT PRIMARY KEY)`); err != nil {
		t.Fatalf("DDL on provisioned database: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM smoke`).Scan(&n); err != nil {
		t.Fatalf("query provisioned database: %v", err)
	}
	if n != 0 {
		t.Errorf("expected empty table, got %d rows", n)
	}
}

func TestOpen_TwoOpens_AreIsolated(t *testing.T) {
	dsn1, cleanup1 := testdb.Open(t)
	defer cleanup1()
	dsn2, cleanup2 := testdb.Open(t)
	defer cleanup2()

	if dsn1 == dsn2 {
		t.Fatalf("two Opens returned the same DSN: %s", dsn1)
	}

	db1, err := sql.Open("pgx", dsn1)
	if err != nil {
		t.Fatalf("open first database: %v", err)
	}
	defer db1.Close()
	if _, err := db1.Exec(`CREATE TABLE only_in_one (id INT)`); err != nil {
		t.Fatalf("DDL on first database: %v", err)
	}

	db2, err := sql.Open("pgx", dsn2)
	if err != nil {
		t.Fatalf("open second database: %v", err)
	}
	defer db2.Close()
	var exists bool
	if err := db2.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'only_in_one')`).Scan(&exists); err != nil {
		t.Fatalf("query second database: %v", err)
	}
	if exists {
		t.Error("table created in first database is visible in second — databases are not isolated")
	}
}
