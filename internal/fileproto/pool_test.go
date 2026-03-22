package fileproto

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// mockRemoteFS implements RemoteFS for pool tests.
type mockRemoteFS struct {
	mu           sync.Mutex
	connected    bool
	closed       bool
	listErr      error // when non-nil, List returns this error (simulates dead connection)
	connectErr   error // when non-nil, Connect returns this error
	connectCalls int
	closeCalls   int
}

func (m *mockRemoteFS) Connect(_ context.Context, _ ConnectionConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectCalls++
	if m.connectErr != nil {
		return m.connectErr
	}
	m.connected = true
	m.closed = false
	return nil
}

func (m *mockRemoteFS) List(_ context.Context, _ string, _ bool) ([]FileEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	return []FileEntry{{Name: "test.txt", IsDir: false}}, nil
}

func (m *mockRemoteFS) Read(_ context.Context, _ string) (io.ReadCloser, int64, error) {
	return nil, 0, ErrNotSupported
}

func (m *mockRemoteFS) Write(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return ErrNotSupported
}

func (m *mockRemoteFS) Mkdir(_ context.Context, _ string) error {
	return ErrNotSupported
}

func (m *mockRemoteFS) Delete(_ context.Context, _ string) error {
	return ErrNotSupported
}

func (m *mockRemoteFS) Rename(_ context.Context, _, _ string) error {
	return ErrNotSupported
}

func (m *mockRemoteFS) Copy(_ context.Context, _, _ string) error {
	return ErrNotSupported
}

func (m *mockRemoteFS) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalls++
	m.closed = true
	m.connected = false
	return nil
}

// testPool creates a pool with a custom factory so we can inject mocks.
// It replaces newRemoteFS for the duration of the test.
type testPool struct {
	*Pool
	mocks []*mockRemoteFS
	mu    sync.Mutex
	idx   int
}

func newTestPool(mocks ...*mockRemoteFS) *testPool {
	tp := &testPool{
		Pool:  NewPool(),
		mocks: mocks,
	}
	return tp
}

// injectMock manually places a mock into the pool's session map for a given ID.
func (tp *testPool) injectMock(connectionID string, mock *mockRemoteFS, config ConnectionConfig) {
	tp.Pool.mu.Lock()
	defer tp.Pool.mu.Unlock()
	tp.Pool.sessions[connectionID] = &poolEntry{
		fs:       mock,
		config:   config,
		lastUsed: time.Now(),
	}
}

// injectMockWithTime places a mock with a specific lastUsed time.
func (tp *testPool) injectMockWithTime(connectionID string, mock *mockRemoteFS, config ConnectionConfig, lastUsed time.Time) {
	tp.Pool.mu.Lock()
	defer tp.Pool.mu.Unlock()
	tp.Pool.sessions[connectionID] = &poolEntry{
		fs:       mock,
		config:   config,
		lastUsed: lastUsed,
	}
}

func (tp *testPool) sessionCount() int {
	tp.Pool.mu.Lock()
	defer tp.Pool.mu.Unlock()
	return len(tp.Pool.sessions)
}

func TestPool_Get_CreatesNewSession(t *testing.T) {
	// The real newRemoteFS will create actual adapter structs, which will fail
	// to connect — so we test the pool logic by pre-injecting mocks instead.
	// For the "creates new" path, we exercise the real Get path with a protocol
	// that won't connect, then verify the error is surfaced properly.
	pool := NewPool()
	defer pool.Close()

	ctx := context.Background()
	config := ConnectionConfig{
		Protocol:    "sftp",
		Host:        "192.0.2.1", // TEST-NET, won't actually connect
		Port:        22,
		Username:    "test",
		Secret:      "pass",
		InitialPath: "/",
	}

	// Attempting to connect to a non-routable address should fail with a
	// connect error (not a panic or nil pointer). We use a very short timeout.
	shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err := pool.Get(shortCtx, "conn-1", config)
	if err == nil {
		t.Fatal("expected connection error for non-routable host, got nil")
	}
	// Pool should not cache failed connections.
	pool.mu.Lock()
	count := len(pool.sessions)
	pool.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 cached sessions after failed connect, got %d", count)
	}
}

func TestPool_Get_ReusesExistingSession(t *testing.T) {
	tp := newTestPool()
	defer tp.Close()

	mock := &mockRemoteFS{}
	config := ConnectionConfig{Protocol: "sftp", InitialPath: "/"}
	tp.injectMock("conn-1", mock, config)

	ctx := context.Background()
	fs, err := tp.Pool.Get(ctx, "conn-1", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fs != mock {
		t.Fatal("expected pool to return the cached mock, got a different instance")
	}
	// Should still have exactly 1 session.
	if n := tp.sessionCount(); n != 1 {
		t.Fatalf("expected 1 session, got %d", n)
	}
}

func TestPool_Get_TransparentReconnectOnDeadSession(t *testing.T) {
	tp := newTestPool()
	defer tp.Close()

	deadMock := &mockRemoteFS{listErr: errors.New("connection reset")}
	config := ConnectionConfig{Protocol: "sftp", Host: "192.0.2.1", InitialPath: "/"}
	tp.injectMock("conn-1", deadMock, config)

	ctx := context.Background()
	// Get should detect the dead connection via the health check, close it,
	// and attempt to create a new one. The new real SFTP client will fail to
	// connect (non-routable), which is fine — we verify the dead one was closed.
	shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, _ = tp.Pool.Get(shortCtx, "conn-1", config)

	// The dead mock should have been closed.
	deadMock.mu.Lock()
	closeCalls := deadMock.closeCalls
	deadMock.mu.Unlock()
	if closeCalls == 0 {
		t.Fatal("expected dead session to be closed during reconnect attempt")
	}
}

func TestPool_Remove(t *testing.T) {
	tp := newTestPool()
	defer tp.Close()

	mock := &mockRemoteFS{}
	config := ConnectionConfig{Protocol: "sftp", InitialPath: "/"}
	tp.injectMock("conn-1", mock, config)

	if n := tp.sessionCount(); n != 1 {
		t.Fatalf("expected 1 session before remove, got %d", n)
	}

	tp.Pool.Remove("conn-1")

	if n := tp.sessionCount(); n != 0 {
		t.Fatalf("expected 0 sessions after remove, got %d", n)
	}

	mock.mu.Lock()
	closed := mock.closed
	mock.mu.Unlock()
	if !closed {
		t.Fatal("expected mock to be closed after Remove")
	}
}

func TestPool_Remove_NonExistent(t *testing.T) {
	pool := NewPool()
	defer pool.Close()
	// Should not panic.
	pool.Remove("does-not-exist")
}

func TestPool_Close_ClosesAllSessions(t *testing.T) {
	tp := newTestPool()

	mock1 := &mockRemoteFS{}
	mock2 := &mockRemoteFS{}
	config := ConnectionConfig{Protocol: "sftp", InitialPath: "/"}
	tp.injectMock("conn-1", mock1, config)
	tp.injectMock("conn-2", mock2, config)

	tp.Close()

	mock1.mu.Lock()
	closed1 := mock1.closed
	mock1.mu.Unlock()
	mock2.mu.Lock()
	closed2 := mock2.closed
	mock2.mu.Unlock()

	if !closed1 {
		t.Fatal("expected mock1 to be closed")
	}
	if !closed2 {
		t.Fatal("expected mock2 to be closed")
	}

	tp.Pool.mu.Lock()
	count := len(tp.Pool.sessions)
	tp.Pool.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 sessions after Close, got %d", count)
	}
}

func TestPool_ReapIdle(t *testing.T) {
	tp := newTestPool()
	defer tp.Close()

	fresh := &mockRemoteFS{}
	stale := &mockRemoteFS{}
	config := ConnectionConfig{Protocol: "sftp", InitialPath: "/"}

	tp.injectMock("fresh", fresh, config)
	tp.injectMockWithTime("stale", stale, config, time.Now().Add(-10*time.Minute))

	// Manually trigger reaping.
	tp.Pool.reapIdle()

	if n := tp.sessionCount(); n != 1 {
		t.Fatalf("expected 1 session after reaping, got %d", n)
	}

	// The stale session should be closed.
	stale.mu.Lock()
	staleClosed := stale.closed
	stale.mu.Unlock()
	if !staleClosed {
		t.Fatal("expected stale session to be closed by reaper")
	}

	// The fresh session should still be alive.
	fresh.mu.Lock()
	freshClosed := fresh.closed
	fresh.mu.Unlock()
	if freshClosed {
		t.Fatal("expected fresh session to remain open")
	}
}

func TestNewRemoteFS_AllProtocols(t *testing.T) {
	protocols := []struct {
		name     string
		wantType string
	}{
		{"sftp", "*fileproto.SFTPClient"},
		{"smb", "*fileproto.SMBClient"},
		{"ftp", "*fileproto.FTPClient"},
		{"webdav", "*fileproto.WebDAVClient"},
	}
	for _, tc := range protocols {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := newRemoteFS(tc.name)
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", tc.name, err)
			}
			if fs == nil {
				t.Fatalf("expected non-nil RemoteFS for %s", tc.name)
			}
		})
	}
}

func TestNewRemoteFS_UnsupportedProtocol(t *testing.T) {
	_, err := newRemoteFS("nfs")
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
}
