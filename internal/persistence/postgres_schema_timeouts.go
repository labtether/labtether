package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	defaultSchemaMigrationTimeout     = 30 * time.Minute
	defaultSchemaMigrationLockTimeout = 5 * time.Minute
)

// SchemaMigrationTimeout bounds one complete schema migration run. Override it
// with LABTETHER_DB_MIGRATION_TIMEOUT (Go duration, for example "2h").
func SchemaMigrationTimeout() time.Duration {
	return envDurationOrDefault("LABTETHER_DB_MIGRATION_TIMEOUT", defaultSchemaMigrationTimeout)
}

// schemaMigrationLockTimeout bounds how long one migration may queue behind
// another session's lock (LABTETHER_DB_MIGRATION_LOCK_TIMEOUT).
func schemaMigrationLockTimeout() time.Duration {
	return envDurationOrDefault("LABTETHER_DB_MIGRATION_LOCK_TIMEOUT", defaultSchemaMigrationLockTimeout)
}

// schemaMigrationSessionSettings lift the pool's per-statement timeout for
// one migration transaction. The pool default (30 s) protects request paths,
// but index builds and backfills on large tables legitimately run longer; an
// upgrade must not fail half-way because of it. SET LOCAL ends with the
// transaction, and these settings are not part of the migration checksum.
func schemaMigrationSessionSettings() []string {
	return []string{
		"SET LOCAL statement_timeout = 0",
		fmt.Sprintf("SET LOCAL lock_timeout = %d", schemaMigrationLockTimeout().Milliseconds()),
	}
}

// migrationExecer is the transaction surface the runner needs; pgx.Tx
// satisfies it.
type migrationExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// applySchemaMigrationInTx runs one migration and records it, inside the
// caller's transaction.
func applySchemaMigrationInTx(ctx context.Context, tx migrationExecer, migration schemaMigration, appliedAt time.Time) error {
	for _, setting := range schemaMigrationSessionSettings() {
		if _, err := tx.Exec(ctx, setting); err != nil {
			return fmt.Errorf("schema migration v%d (%s): configure timeouts: %w", migration.Version, migration.Name, err)
		}
	}
	for _, stmt := range migration.Statements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at, checksum) VALUES ($1, $2, $3, $4)`,
		migration.Version,
		migration.Name,
		appliedAt,
		schemaMigrationChecksum(migration),
	)
	return err
}
