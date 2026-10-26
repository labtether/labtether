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
	"github.com/labtether/labtether/internal/terminal"
)

// A database that accepts connections but never completes startup must not
// retain logical-session gates. With one pool slot this also covers queue waits.
func TestTerminalStoreUnavailableDatabaseIsBounded(t *testing.T) {
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
		"lookup":            func() error { _, _, err := store.GetSession("session"); return err },
		"create":            func() error { _, err := store.CreateSession(terminal.CreateSessionRequest{Target: "test"}); return err },
		"list":              func() error { _, err := store.ListSessions(); return err },
		"delete":            func() error { return store.DeleteTerminalSession("session") },
		"saved lookup":      func() error { _, _, err := store.GetPersistentSession("saved"); return err },
		"attach":            func() error { _, err := store.MarkPersistentSessionAttached("saved", time.Now()); return err },
		"detach":            func() error { _, err := store.MarkPersistentSessionDetached("saved", time.Now()); return err },
		"saved delete":      func() error { return store.DeletePersistentSession("saved") },
		"scrollback write":  func() error { return store.UpsertScrollback("saved", nil, 0, 0) },
		"scrollback read":   func() error { _, err := store.GetScrollback("saved"); return err },
		"scrollback delete": func() error { return store.DeleteScrollback("saved") },
	}
	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(calls))
	for name, call := range calls {
		go func() { results <- result{name, call()} }()
	}
	deadline := time.NewTimer(terminalStoreQueryTimeout + 3*time.Second)
	defer deadline.Stop()
	for range calls {
		select {
		case got := <-results:
			if !errors.Is(got.err, context.DeadlineExceeded) {
				t.Errorf("%s: expected deadline, got %v", got.name, got.err)
			}
		case <-deadline.C:
			t.Fatal("terminal store retained a session operation past its database deadline")
		}
	}
}
