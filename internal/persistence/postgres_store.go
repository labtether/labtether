package persistence

import (
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements persistence stores backed by PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool

	logEventsWatermarkMu              sync.RWMutex
	logEventsWatermark                time.Time
	logEventsWatermarkFetchedAt       time.Time
	logEventsWatermarkRefreshInterval time.Duration
}

// Pool returns the underlying connection pool for use by subsystems
// that need direct pool access (e.g. job queue).
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}
