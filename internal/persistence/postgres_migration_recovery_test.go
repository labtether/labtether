package persistence

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresMigrationsRecoverAfterMarkerDeletion(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping migration recovery integration test")
	}

	migrations, err := normalizedSchemaMigrations(postgresSchemaMigrations())
	if err != nil {
		t.Fatalf("invalid schema migrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatalf("expected at least one schema migration")
	}
	latest := migrations[len(migrations)-1]

	store := mustOpenPostgresStoreForMigrationRecovery(t, dbURL)
	if count := migrationVersionCount(t, store, latest.Version); count != 1 {
		store.Close()
		t.Fatalf("expected latest migration version %d to be applied once, got %d", latest.Version, count)
	}

	if _, err := store.pool.Exec(context.Background(), `DELETE FROM schema_migrations WHERE version = $1`, latest.Version); err != nil {
		store.Close()
		t.Fatalf("delete latest migration marker: %v", err)
	}
	if count := migrationVersionCount(t, store, latest.Version); count != 0 {
		store.Close()
		t.Fatalf("expected migration marker deletion for version %d, got count=%d", latest.Version, count)
	}
	store.Close()

	recovered := mustOpenPostgresStoreForMigrationRecovery(t, dbURL)
	if count := migrationVersionCount(t, recovered, latest.Version); count != 1 {
		recovered.Close()
		t.Fatalf("expected migration version %d to be restored after reopen, got %d", latest.Version, count)
	}
	recovered.Close()

	again := mustOpenPostgresStoreForMigrationRecovery(t, dbURL)
	defer again.Close()
	if count := migrationVersionCount(t, again, latest.Version); count != 1 {
		t.Fatalf("expected migration version %d to remain idempotent, got %d", latest.Version, count)
	}
}

func TestPostgresMigrationsCreateCanonicalTables(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping canonical migration integration test")
	}

	store := mustOpenPostgresStoreForMigrationRecovery(t, dbURL)
	defer store.Close()

	tables := []string{
		"provider_instances",
		"resource_external_refs",
		"canonical_resource_relationships",
		"canonical_capability_sets",
		"canonical_template_bindings",
		"canonical_ingest_checkpoints",
		"canonical_reconciliation_results",
		"group_maintenance_windows",
		"maintenance_overrides",
		"group_reliability_history",
		"group_profile_assignments",
		"group_profile_drift_checks",
		"group_failover_pairs",
	}
	for _, table := range tables {
		assertTableExists(t, store, table)
	}

	assertColumnExists(t, store, "groups", "geo_label")
	assertColumnExists(t, store, "groups", "status")
}

func mustOpenPostgresStoreForMigrationRecovery(t *testing.T, dbURL string) *PostgresStore {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := NewPostgresStore(ctx, dbURL)
	if err != nil {
		t.Fatalf("NewPostgresStore failed: %v", err)
	}
	return store
}

func migrationVersionCount(t *testing.T, store *PostgresStore, version int) int {
	t.Helper()

	var count int
	if err := store.pool.QueryRow(
		context.Background(),
		`SELECT COUNT(1) FROM schema_migrations WHERE version = $1`,
		version,
	).Scan(&count); err != nil {
		t.Fatalf("query migration version count: %v", err)
	}
	return count
}

func assertTableExists(t *testing.T, store *PostgresStore, table string) {
	t.Helper()

	var regclass string
	if err := store.pool.QueryRow(
		context.Background(),
		`SELECT COALESCE(to_regclass($1)::text, '')`,
		"public."+table,
	).Scan(&regclass); err != nil {
		t.Fatalf("query table %s existence: %v", table, err)
	}
	if regclass == "" {
		t.Fatalf("expected table %s to exist after migrations", table)
	}
}

func assertColumnExists(t *testing.T, store *PostgresStore, table string, column string) {
	t.Helper()

	var exists bool
	if err := store.pool.QueryRow(
		context.Background(),
		`SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
		)`,
		table,
		column,
	).Scan(&exists); err != nil {
		t.Fatalf("query column %s.%s existence: %v", table, column, err)
	}
	if !exists {
		t.Fatalf("expected column %s.%s to exist after migrations", table, column)
	}
}
