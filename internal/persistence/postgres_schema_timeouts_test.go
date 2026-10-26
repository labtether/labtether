package persistence

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type recordingMigrationExecer struct{ statements []string }

func (r *recordingMigrationExecer) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	r.statements = append(r.statements, sql)
	return pgconn.CommandTag{}, nil
}

// Upgrades failed when a migration (v88 builds an index on metric_samples)
// outlasted the pool's 30 s statement_timeout. The runner must lift it for the
// migration transaction before any migration statement runs.
func TestSchemaMigrationLiftsStatementTimeoutBeforeStatements(t *testing.T) {
	t.Setenv("LABTETHER_DB_MIGRATION_LOCK_TIMEOUT", "")
	exec := &recordingMigrationExecer{}
	migration := schemaMigration{
		Version:    9999,
		Name:       "large index",
		Statements: []string{"CREATE INDEX IF NOT EXISTS idx_test ON metric_samples (asset_id)"},
	}
	before := schemaMigrationChecksum(migration)
	if err := applySchemaMigrationInTx(context.Background(), exec, migration, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if len(exec.statements) != 4 {
		t.Fatalf("statements = %q", exec.statements)
	}
	if exec.statements[0] != "SET LOCAL statement_timeout = 0" {
		t.Fatalf("first statement = %q, want local statement_timeout lift", exec.statements[0])
	}
	if want := "SET LOCAL lock_timeout = 300000"; exec.statements[1] != want {
		t.Fatalf("second statement = %q, want %q", exec.statements[1], want)
	}
	if exec.statements[2] != migration.Statements[0] || !strings.HasPrefix(exec.statements[3], "INSERT INTO schema_migrations") {
		t.Fatalf("migration body/record order = %q", exec.statements[2:])
	}
	if schemaMigrationChecksum(migration) != before {
		t.Fatal("timeout settings changed the migration checksum")
	}
}

func TestSchemaMigrationTimeoutIsGenerousAndConfigurable(t *testing.T) {
	t.Setenv("LABTETHER_DB_MIGRATION_TIMEOUT", "")
	if got := SchemaMigrationTimeout(); got != 30*time.Minute {
		t.Fatalf("default migration timeout = %s", got)
	}
	t.Setenv("LABTETHER_DB_MIGRATION_TIMEOUT", "2h")
	if got := SchemaMigrationTimeout(); got != 2*time.Hour {
		t.Fatalf("configured migration timeout = %s", got)
	}
	t.Setenv("LABTETHER_DB_MIGRATION_TIMEOUT", "-5m")
	if got := SchemaMigrationTimeout(); got != 30*time.Minute {
		t.Fatalf("invalid migration timeout fell through as %s", got)
	}
}
