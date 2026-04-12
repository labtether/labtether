package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type schemaMigration struct {
	Version    int
	Name       string
	Statements []string
}

const defaultDBStatementTimeout = 30 * time.Second
const defaultLogEventsWatermarkRefreshInterval = 3 * time.Second
const schemaMigrationAdvisoryLockKey int64 = 0x4c545348454d41 // "LTSHEMA"

func NewPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	if databaseURL == "" {
		return nil, errors.New("database url is required")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	maxConns := envIntOrDefault("LABTETHER_DB_MAX_CONNS", 10)
	minConns := envIntOrDefault("LABTETHER_DB_MIN_CONNS", 2)
	lifetimeMin := envIntOrDefault("LABTETHER_DB_MAX_CONN_LIFETIME_MIN", 5)
	idleMin := envIntOrDefault("LABTETHER_DB_MAX_CONN_IDLE_TIME_MIN", 1)

	config.MaxConns = int32(maxConns)
	config.MinConns = int32(minConns)
	config.MaxConnLifetime = time.Duration(lifetimeMin) * time.Minute
	config.MaxConnIdleTime = time.Duration(idleMin) * time.Minute
	config.HealthCheckPeriod = 30 * time.Second

	log.Printf("labtether db: pool max=%d min=%d lifetime=%dm idle=%dm",
		maxConns, minConns, lifetimeMin, idleMin)
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string, 2)
	}
	config.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(dbStatementTimeout().Milliseconds(), 10)

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	store := &PostgresStore{
		pool:                              pool,
		logEventsWatermark:                time.Unix(0, 0).UTC(),
		logEventsWatermarkFetchedAt:       time.Unix(0, 0).UTC(),
		logEventsWatermarkRefreshInterval: defaultLogEventsWatermarkRefreshInterval,
	}
	if err := store.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func dbStatementTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("LABTETHER_DB_STATEMENT_TIMEOUT"))
	if raw == "" {
		return defaultDBStatementTimeout
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return defaultDBStatementTimeout
	}
	return parsed
}

func envIntOrDefault(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (s *PostgresStore) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

func (s *PostgresStore) ensureSchema(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, schemaMigrationAdvisoryLockKey); err != nil {
		return err
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, schemaMigrationAdvisoryLockKey)
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INT PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL
	)`); err != nil {
		return err
	}

	// Add checksum column to existing deployments that pre-date this column.
	// NULL checksum means the migration was applied before checksum tracking was
	// introduced; those rows are skipped during verification (no false positives).
	if _, err := conn.Exec(ctx, `ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT`); err != nil {
		return err
	}

	migrations, err := normalizedSchemaMigrations(postgresSchemaMigrations())
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if err := applyOrVerifySchemaMigration(ctx, conn, migration); err != nil {
			return err
		}
	}
	return nil
}

func normalizedSchemaMigrations(migrations []schemaMigration) ([]schemaMigration, error) {
	if len(migrations) == 0 {
		return nil, nil
	}

	sorted := append([]schemaMigration(nil), migrations...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Version < sorted[j].Version
	})

	for i := 1; i < len(sorted); i++ {
		if sorted[i-1].Version == sorted[i].Version {
			return nil, fmt.Errorf(
				"duplicate schema migration version %d (%s and %s)",
				sorted[i].Version,
				sorted[i-1].Name,
				sorted[i].Name,
			)
		}
	}

	return sorted, nil
}

// schemaMigrationChecksum computes a stable SHA-256 hex digest over all SQL
// statements in a migration.  The digest is stored when a migration is first
// applied and re-verified on every subsequent startup.  A mismatch indicates
// that the migration source code was modified after being applied, which is a
// configuration error — migrations must be append-only.
func schemaMigrationChecksum(migration schemaMigration) string {
	h := sha256.New()
	for _, stmt := range migration.Statements {
		h.Write([]byte(stmt))
		h.Write([]byte{0}) // null-byte separator so adjacent statements don't merge
	}
	return hex.EncodeToString(h.Sum(nil))
}

func applyOrVerifySchemaMigration(ctx context.Context, conn *pgxpool.Conn, migration schemaMigration) error {
	// Query both existence and stored checksum in one round-trip.
	var (
		exists         bool
		storedChecksum *string // NULL when row pre-dates checksum column
	)
	if err := conn.QueryRow(ctx,
		`SELECT TRUE, checksum FROM schema_migrations WHERE version = $1`,
		migration.Version,
	).Scan(&exists, &storedChecksum); err != nil {
		// pgx returns an error on no rows; treat that as not-yet-applied.
		exists = false
	}

	if exists {
		// Verify checksum for already-applied migrations that have one recorded.
		// Rows with a NULL checksum were applied before checksum tracking existed;
		// backfill the checksum now so future startups can verify them.
		want := schemaMigrationChecksum(migration)
		if storedChecksum == nil {
			if _, err := conn.Exec(ctx,
				`UPDATE schema_migrations SET checksum = $1 WHERE version = $2`,
				want, migration.Version,
			); err != nil {
				return fmt.Errorf("schema migration v%d (%s): backfill checksum: %w",
					migration.Version, migration.Name, err)
			}
			log.Printf("labtether: schema migration v%d (%s): checksum backfilled",
				migration.Version, migration.Name)
		} else if *storedChecksum != want {
			return fmt.Errorf(
				"schema migration v%d (%s) has been modified after being applied "+
					"(stored checksum %s, computed %s) — "+
					"migrations are append-only; restore the original SQL statements "+
					"or consult docs/internal/UPGRADING.md for rollback instructions",
				migration.Version, migration.Name, *storedChecksum, want,
			)
		}
		return nil
	}

	log.Printf("labtether: applying schema migration v%d: %s", migration.Version, migration.Name)

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, stmt := range migration.Statements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at, checksum) VALUES ($1, $2, $3, $4)`,
		migration.Version,
		migration.Name,
		time.Now().UTC(),
		schemaMigrationChecksum(migration),
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
