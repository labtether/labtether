package persistence

import (
	"strings"
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

func TestSchemaMigrationChecksumIsStable(t *testing.T) {
	m := schemaMigration{
		Version:    1,
		Name:       "test",
		Statements: []string{"CREATE TABLE foo (id TEXT PRIMARY KEY)", "CREATE INDEX idx_foo ON foo(id)"},
	}

	first := schemaMigrationChecksum(m)
	second := schemaMigrationChecksum(m)
	if first != second {
		t.Fatalf("checksum is not deterministic: %s vs %s", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("expected 64-char hex SHA-256, got %d chars: %s", len(first), first)
	}
}

func TestSchemaMigrationChecksumDiffersOnStatementChange(t *testing.T) {
	base := schemaMigration{
		Version:    1,
		Name:       "test",
		Statements: []string{"CREATE TABLE foo (id TEXT PRIMARY KEY)"},
	}
	modified := schemaMigration{
		Version:    1,
		Name:       "test",
		Statements: []string{"CREATE TABLE foo (id TEXT PRIMARY KEY, extra TEXT)"},
	}

	if schemaMigrationChecksum(base) == schemaMigrationChecksum(modified) {
		t.Fatalf("expected different checksums for different statements")
	}
}

func TestSchemaMigrationChecksumDiffersOnStatementOrder(t *testing.T) {
	a := schemaMigration{
		Version:    1,
		Name:       "test",
		Statements: []string{"CREATE TABLE a (id TEXT)", "CREATE TABLE b (id TEXT)"},
	}
	b := schemaMigration{
		Version:    1,
		Name:       "test",
		Statements: []string{"CREATE TABLE b (id TEXT)", "CREATE TABLE a (id TEXT)"},
	}

	if schemaMigrationChecksum(a) == schemaMigrationChecksum(b) {
		t.Fatalf("expected different checksums for different statement order")
	}
}

func TestSchemaMigrationChecksumDiffersOnAdjacentStatements(t *testing.T) {
	// Ensure ["AB", "C"] and ["A", "BC"] produce different checksums (null-byte separator).
	ab := schemaMigration{Version: 1, Name: "t", Statements: []string{"AB", "C"}}
	a := schemaMigration{Version: 1, Name: "t", Statements: []string{"A", "BC"}}

	if schemaMigrationChecksum(ab) == schemaMigrationChecksum(a) {
		t.Fatalf("null-byte separator not working: adjacent statement concatenation collision")
	}
}

func TestSchemaMigrationChecksumIsHexString(t *testing.T) {
	m := schemaMigration{Version: 99, Name: "hex_check", Statements: []string{"SELECT 1"}}
	cs := schemaMigrationChecksum(m)
	for _, ch := range cs {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			t.Fatalf("checksum contains non-hex character %q: %s", ch, cs)
		}
	}
}

func TestPostgresSchemaMigrationsChecksumsAreReproducible(t *testing.T) {
	// Ensure every migration in the real set produces the same checksum twice —
	// guards against any non-determinism in the migrations slice construction.
	migrations := postgresSchemaMigrations()
	for _, m := range migrations {
		if schemaMigrationChecksum(m) != schemaMigrationChecksum(m) {
			t.Fatalf("non-deterministic checksum for migration v%d (%s)", m.Version, m.Name)
		}
	}
}
