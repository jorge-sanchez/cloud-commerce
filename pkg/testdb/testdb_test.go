package testdb

// Test Budget: 1 distinct behavior × 2 = 2 max unit tests
// Actual: 2
//
// Behavior 1: swapDatabase — replaces the database path in a URL-form DSN,
// preserving credentials, host, and query parameters; rejects a non-URL DSN.
//
// The provisioning paths (shared server / testcontainer) are exercised by
// every integration-test package in the repo and are deliberately not
// unit-tested here — they require Docker or a live server.

import "testing"

func TestSwapDatabase_URLDSN_ReplacesOnlyThePath(t *testing.T) {
	got, err := swapDatabase("postgres://app:app@localhost:5432/app_test?sslmode=disable", "app_it_0a1b")
	if err != nil {
		t.Fatalf("swapDatabase: %v", err)
	}
	want := "postgres://app:app@localhost:5432/app_it_0a1b?sslmode=disable"
	if got != want {
		t.Errorf("swapDatabase = %q, want %q", got, want)
	}
}

func TestSwapDatabase_MalformedDSN_ReturnsError(t *testing.T) {
	if _, err := swapDatabase("postgres://app:app@localhost:5432/db?sslmode=disable\x00", "x"); err == nil {
		t.Error("expected error for malformed DSN")
	}
}
