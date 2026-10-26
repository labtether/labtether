package persistence

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labtether/labtether/internal/logs"
)

// These real store owners run synchronously under agent socket ownership.
// A database that stalls during startup, including the pool wait behind it,
// must not hold that ownership forever. Cover the distinct SQL call forms:
// single write, batch write, rows query, and transaction acquisition.
func TestAgentLifecycleStoreUnavailableDatabaseIsBounded(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://test@localhost/test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = 30 * time.Second
	var peersMu sync.Mutex
	var peers []net.Conn
	closing := false
	cfg.ConnConfig.DialFunc = func(context.Context, string, string) (net.Conn, error) {
		conn, peer := net.Pipe()
		peersMu.Lock()
		if closing {
			_ = conn.Close()
			_ = peer.Close()
		} else {
			peers = append(peers, peer)
			go func() { _, _ = io.Copy(io.Discard, peer) }()
		}
		peersMu.Unlock()
		return conn, nil
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		peersMu.Lock()
		closing = true
		for _, peer := range peers {
			_ = peer.Close()
		}
		peersMu.Unlock()
		pool.Close()
	}()
	store := &PostgresStore{pool: pool}
	calls := map[string]func() error{
		"presence": func() error {
			_, err := store.UpdateHeartbeatForSession("asset", "socket", time.Now())
			return err
		},
		"log batch": func() error {
			return store.AppendEvents([]logs.Event{{AssetID: "asset", Message: "message"}})
		},
		"collector discovery": func() error {
			_, err := store.ListHubCollectors(500, true)
			return err
		},
		"canonical heartbeat transaction": func() error {
			return store.ReplaceResourceExternalRefs("asset", nil)
		},
	}
	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(calls))
	for name, call := range calls {
		go func() { results <- result{name, call()} }()
	}
	deadline := time.NewTimer(agentLifecycleStoreQueryTimeout + 3*time.Second)
	defer deadline.Stop()
	for range calls {
		select {
		case got := <-results:
			if !errors.Is(got.err, context.DeadlineExceeded) {
				t.Errorf("%s: expected deadline, got %v", got.name, got.err)
			}
		case <-deadline.C:
			t.Fatal("database retained an agent lifecycle operation past its deadline")
		}
	}
}
