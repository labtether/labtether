package persistence

import (
	"context"
	"errors"
	"fmt"
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
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = 5 * time.Minute
	config.MaxConnIdleTime = 1 * time.Minute
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

	migrations, err := normalizedSchemaMigrations(postgresSchemaMigrations())
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if err := applySchemaMigration(ctx, conn, migration); err != nil {
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

func applySchemaMigration(ctx context.Context, conn *pgxpool.Conn, migration schemaMigration) error {
	var alreadyApplied bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, migration.Version).Scan(&alreadyApplied); err != nil {
		return err
	}
	if alreadyApplied {
		return nil
	}

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
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES ($1, $2, $3)`,
		migration.Version,
		migration.Name,
		time.Now().UTC(),
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
