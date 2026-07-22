package persistence

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// hubRuntimeAdvisoryLockKey is independent from the schema-migration lock.
// Holding it fences the live, connection-owning hub runtime while still
// allowing standalone migrators and read-only database tooling to operate.
const hubRuntimeAdvisoryLockKey int64 = 0x4c544855425254 // "LTHUBRT"

var (
	ErrHubRuntimeLeaseHeld = errors.New("another hub already owns the live runtime lease")
	ErrHubRuntimeLeaseLost = errors.New("hub live runtime lease was lost")
)

// HubRuntimeLease owns a PostgreSQL session-level advisory lock. The acquired
// pool connection must remain dedicated to this lease for its entire lifetime;
// returning it to the pool would allow unrelated work to inherit the lock.
type HubRuntimeLease struct {
	mu          sync.Mutex
	conn        *pgxpool.Conn
	releaseOnce sync.Once
	releaseErr  error
}

// AcquireHubRuntimeLease prevents two hub API/worker runtimes from serving the
// same database with replica-local agent WebSocket state. It is deliberately a
// non-blocking claim so an accidental second active hub fails closed instead of
// appearing healthy while only some agent operations work.
func (s *PostgresStore) AcquireHubRuntimeLease(ctx context.Context) (*HubRuntimeLease, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("postgres store is unavailable")
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire hub runtime lease connection: %w", err)
	}

	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, hubRuntimeAdvisoryLockKey).Scan(&acquired); err != nil {
		conn.Release()
		return nil, fmt.Errorf("claim hub runtime lease: %w", err)
	}
	if !acquired {
		conn.Release()
		return nil, ErrHubRuntimeLeaseHeld
	}
	return &HubRuntimeLease{conn: conn}, nil
}

// Ping verifies that the exact PostgreSQL session holding the advisory lock is
// still alive. Callers must stop serving immediately on failure: PostgreSQL
// releases session locks when a connection is lost.
func (l *HubRuntimeLease) Ping(ctx context.Context) error {
	if l == nil {
		return ErrHubRuntimeLeaseLost
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return ErrHubRuntimeLeaseLost
	}
	if err := l.conn.Conn().Ping(ctx); err != nil {
		return fmt.Errorf("%w: lease database session is unavailable", ErrHubRuntimeLeaseLost)
	}
	return nil
}

// Release unlocks and returns the dedicated connection exactly once.
func (l *HubRuntimeLease) Release(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.releaseOnce.Do(func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.conn == nil {
			return
		}
		var unlocked bool
		if err := l.conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, hubRuntimeAdvisoryLockKey).Scan(&unlocked); err != nil {
			l.releaseErr = fmt.Errorf("release hub runtime lease: %w", err)
		} else if !unlocked {
			l.releaseErr = ErrHubRuntimeLeaseLost
		}
		l.conn.Release()
		l.conn = nil
	})
	return l.releaseErr
}
