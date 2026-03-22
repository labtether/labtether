package persistence

import (
	"testing"
	"time"
)

func TestNormalizedSchemaMigrationsSortsByVersion(t *testing.T) {
	input := []schemaMigration{
		{Version: 3, Name: "third"},
		{Version: 1, Name: "first"},
		{Version: 2, Name: "second"},
	}

	got, err := normalizedSchemaMigrations(input)
	if err != nil {
		t.Fatalf("normalizedSchemaMigrations returned error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(got))
	}
	if got[0].Version != 1 || got[1].Version != 2 || got[2].Version != 3 {
		t.Fatalf("expected sorted versions [1 2 3], got [%d %d %d]", got[0].Version, got[1].Version, got[2].Version)
	}
}

func TestNormalizedSchemaMigrationsRejectsDuplicates(t *testing.T) {
	input := []schemaMigration{
		{Version: 7, Name: "one"},
		{Version: 7, Name: "two"},
	}

	if _, err := normalizedSchemaMigrations(input); err == nil {
		t.Fatalf("expected duplicate version error, got nil")
	}
}

func TestPostgresSchemaMigrationsHaveUniqueVersions(t *testing.T) {
	migrations, err := normalizedSchemaMigrations(postgresSchemaMigrations())
	if err != nil {
		t.Fatalf("postgresSchemaMigrations returned invalid migration set: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatalf("expected at least one migration")
	}
}

func TestDBStatementTimeoutDefaultWhenUnset(t *testing.T) {
	t.Setenv("LABTETHER_DB_STATEMENT_TIMEOUT", "")

	if got := dbStatementTimeout(); got != defaultDBStatementTimeout {
		t.Fatalf("expected default timeout %s, got %s", defaultDBStatementTimeout, got)
	}
}

func TestDBStatementTimeoutUsesConfiguredValue(t *testing.T) {
	t.Setenv("LABTETHER_DB_STATEMENT_TIMEOUT", "42s")

	if got := dbStatementTimeout(); got != 42*time.Second {
		t.Fatalf("expected timeout 42s, got %s", got)
	}
}

func TestDBStatementTimeoutInvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("LABTETHER_DB_STATEMENT_TIMEOUT", "definitely-not-a-duration")

	if got := dbStatementTimeout(); got != defaultDBStatementTimeout {
		t.Fatalf("expected default timeout %s on invalid input, got %s", defaultDBStatementTimeout, got)
	}
}
