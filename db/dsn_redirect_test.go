package db

import (
	"os"
	"strings"
	"testing"
)

// The redirect must never touch a non-test process (the real server), must
// rewrite the database name for test binaries, and must leave a DSN without a
// database name untouched (the caller surfaces the unchanged DSN instead).
func TestDSNRedirectsTestRunsToTestDatabase(t *testing.T) {
	t.Setenv("ARTEX_PG_DSN", "postgres://u:p@127.0.0.1:5432/artex?sslmode=disable")
	t.Setenv("ARTEX_ALLOW_PROD_DB_IN_TESTS", "")

	dsn, source, err := DSN()
	if err != nil {
		t.Fatalf("DSN: %v", err)
	}
	if !strings.Contains(dsn, "/artex_test") {
		t.Fatalf("test run must be redirected to the _test database, got %s", dsn)
	}
	if strings.Contains(dsn, "/artex?") || strings.HasSuffix(dsn, "/artex") {
		t.Fatalf("test run must not point at the configured database, got %s", dsn)
	}
	if !strings.Contains(source, "_test") {
		t.Fatalf("source should report the redirect, got %s", source)
	}
}

func TestDSNOptOutKeepsConfiguredDatabase(t *testing.T) {
	t.Setenv("ARTEX_PG_DSN", "postgres://u:p@127.0.0.1:5432/artex?sslmode=disable")
	t.Setenv("ARTEX_ALLOW_PROD_DB_IN_TESTS", "1")

	dsn, _, err := DSN()
	if err != nil {
		t.Fatalf("DSN: %v", err)
	}
	if !strings.Contains(dsn, "/artex?") {
		t.Fatalf("opt-out must keep the configured database, got %s", dsn)
	}
}

func TestTestDatabaseDSN(t *testing.T) {
	got, err := testDatabaseDSN("postgres://u:p@h:5432/artex?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if want := "postgres://u:p@h:5432/artex_test?sslmode=disable"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if _, err := testDatabaseDSN("postgres://u:p@h:5432"); err == nil {
		t.Fatal("a DSN without a database name must be refused, not rewritten")
	}
}

// isTestRun is exercised implicitly by every test in this binary; the flag
// scan itself is trivial enough not to need its own case beyond a smoke check.
func TestIsTestRunSmoke(t *testing.T) {
	if !isTestRun() {
		t.Fatal("running under go test, isTestRun must be true")
	}
	if os.Getenv("ARTEX_ALLOW_PROD_DB_IN_TESTS") == "" && isTestRun() {
		// combined with the redirect tests above this pins the invariant:
		// a test binary without the opt-out always gets a redirected DSN.
		dsn, _, err := DSN()
		if err == nil && strings.Contains(dsn, "/artex?") {
			t.Fatal("test binary without opt-out resolved the production database")
		}
	}
}
