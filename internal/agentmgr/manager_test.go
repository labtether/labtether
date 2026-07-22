package agentmgr

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newTestConn creates an AgentConn backed by a real WebSocket for testing.
func newTestConn(t *testing.T, assetID, platform string) (*AgentConn, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Keep connection alive until test closes it.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial test ws: %v", err)
	}

	ac := NewAgentConn(conn, assetID, platform)
	cleanup := func() {
		ac.Close()
		srv.Close()
	}
	return ac, cleanup
}

func TestRegisterAndGet(t *testing.T) {
	m := NewManager()

	conn, cleanup := newTestConn(t, "node-1", "linux")
	defer cleanup()

	m.Register(conn)

	got, ok := m.Get("node-1")
	if !ok || got.AssetID != "node-1" {
		t.Fatalf("expected to find node-1, got ok=%v", ok)
	}

	if m.Count() != 1 {
		t.Fatalf("expected count 1, got %d", m.Count())
	}
}

func TestUnregister(t *testing.T) {
	m := NewManager()

	conn, cleanup := newTestConn(t, "node-2", "darwin")
	defer cleanup()

	m.Register(conn)
	m.Unregister("node-2")

	if m.IsConnected("node-2") {
		t.Fatal("expected node-2 to be disconnected")
	}
	if m.Count() != 0 {
		t.Fatalf("expected count 0, got %d", m.Count())
	}
}

func TestRegisterReplacesExisting(t *testing.T) {
	m := NewManager()

	conn1, cleanup1 := newTestConn(t, "node-3", "linux")
	defer cleanup1()
	conn2, cleanup2 := newTestConn(t, "node-3", "linux")
	defer cleanup2()

	m.Register(conn1)
	m.Register(conn2)

	if m.Count() != 1 {
		t.Fatalf("expected count 1 after replace, got %d", m.Count())
	}

	got, ok := m.Get("node-3")
	if !ok {
		t.Fatal("expected node-3 to exist")
	}
	if got != conn2 {
		t.Fatal("expected the newer connection to be stored")
	}
}

func TestUnregisterIfMatchSkipsNewerConnection(t *testing.T) {
	m := NewManager()

	conn1, cleanup1 := newTestConn(t, "node-4", "linux")
	defer cleanup1()
	conn2, cleanup2 := newTestConn(t, "node-4", "linux")
	defer cleanup2()

	m.Register(conn1)
	m.Register(conn2)

	if removed := m.UnregisterIfMatch("node-4", conn1); removed {
		t.Fatal("expected unregister with stale connection to be ignored")
	}
	if !m.IsConnected("node-4") {
		t.Fatal("expected node-4 to remain connected after stale unregister")
	}

	if removed := m.UnregisterIfMatch("node-4", conn2); !removed {
		t.Fatal("expected unregister with active connection to succeed")
	}
	if m.IsConnected("node-4") {
		t.Fatal("expected node-4 to be disconnected after active unregister")
	}
}

func TestConnectedAssets(t *testing.T) {
	m := NewManager()

	conn1, cleanup1 := newTestConn(t, "a", "linux")
	defer cleanup1()
	conn2, cleanup2 := newTestConn(t, "b", "windows")
	defer cleanup2()

	m.Register(conn1)
	m.Register(conn2)

	assets := m.ConnectedAssets()
	if len(assets) != 2 {
		t.Fatalf("expected 2 connected assets, got %d", len(assets))
	}

	found := map[string]bool{}
	for _, id := range assets {
		found[id] = true
	}
	if !found["a"] || !found["b"] {
		t.Fatalf("expected assets a and b, got %v", assets)
	}
}

func TestIsConnectedEmpty(t *testing.T) {
	m := NewManager()
	if m.IsConnected("nonexistent") {
		t.Fatal("expected false for nonexistent asset")
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewManager()
	const n = 50

	conns := make([]*AgentConn, n)
	cleanups := make([]func(), n)
	for i := 0; i < n; i++ {
		conns[i], cleanups[i] = newTestConn(t, strings.Repeat("x", 1)+string(rune('A'+i%26)), "linux")
		defer cleanups[i]()
	}

	var wg sync.WaitGroup
	wg.Add(n * 2)

	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			m.Register(conns[idx])
		}(i)
	}

	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			m.IsConnected(conns[idx].AssetID)
			m.Count()
			m.ConnectedAssets()
		}(i)
	}

	wg.Wait()
}

func TestCloseRejectsQueuedCredentialValidatedWrites(t *testing.T) {
	conn, cleanup := newTestConn(t, "queued-revoke", "linux")
	defer cleanup()
	entered := make(chan struct{})
	release := make(chan struct{})
	var validations atomic.Int32
	conn.SetCredentialValidator(func() error {
		if validations.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil
	})

	const writers = 16
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		go func() {
			errs <- conn.Send(Message{Type: MsgConfigUpdate})
		}()
	}
	<-entered
	closed := make(chan struct{})
	go func() {
		conn.Close()
		close(closed)
	}()
	deadline := time.Now().Add(time.Second)
	for !conn.rejected.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !conn.rejected.Load() {
		t.Fatal("close did not mark the connection rejected before waiting on the active writer")
	}
	close(release)
	<-closed
	for i := 0; i < writers; i++ {
		if err := <-errs; !errors.Is(err, ErrAgentCredentialRejected) {
			t.Fatalf("queued writer %d error=%v", i, err)
		}
	}
	if got := validations.Load(); got != 1 {
		t.Fatalf("credential validations=%d, want only in-progress writer", got)
	}
}

func TestAgentAdmissionEnforcesGlobalAndSourceLimitsAndReleasesOnce(t *testing.T) {
	m := NewManager()
	releaseA, rejected := m.TryReserveAdmission("192.0.2.1", 2, 1)
	if rejected != AdmissionAllowed {
		t.Fatalf("first admission rejected=%v", rejected)
	}
	if _, rejected := m.TryReserveAdmission("192.0.2.1", 2, 1); rejected != AdmissionSourceLimit {
		t.Fatalf("same-source rejection=%v, want source limit", rejected)
	}
	releaseB, rejected := m.TryReserveAdmission("192.0.2.2", 2, 1)
	if rejected != AdmissionAllowed {
		t.Fatalf("second source rejected=%v", rejected)
	}
	if _, rejected := m.TryReserveAdmission("192.0.2.3", 2, 1); rejected != AdmissionGlobalLimit {
		t.Fatalf("global rejection=%v, want global limit", rejected)
	}
	if got := m.AdmissionCount(); got != 2 {
		t.Fatalf("admissions=%d, want 2", got)
	}
	releaseA()
	releaseA()
	if got := m.AdmissionCount(); got != 1 {
		t.Fatalf("idempotent release admissions=%d, want 1", got)
	}
	if releaseC, rejected := m.TryReserveAdmission("192.0.2.1", 2, 1); rejected != AdmissionAllowed {
		t.Fatalf("released source was not admitted: %v", rejected)
	} else {
		releaseC()
	}
	releaseB()
	if got := m.AdmissionCount(); got != 0 {
		t.Fatalf("final admissions=%d, want 0", got)
	}
}

func TestCredentialValidationLeaseBoundsAuthoritativeChecks(t *testing.T) {
	conn := NewAgentConn(nil, "leased", "linux")
	var validations atomic.Int32
	conn.SetCredentialValidatorWithLease(func() error {
		validations.Add(1)
		return nil
	}, 20*time.Millisecond)

	if err := conn.ValidateCredential(); err != nil {
		t.Fatal(err)
	}
	if err := conn.ValidateCredential(); err != nil {
		t.Fatal(err)
	}
	if got := validations.Load(); got != 1 {
		t.Fatalf("validations within lease=%d, want 1", got)
	}
	time.Sleep(25 * time.Millisecond)
	if err := conn.ValidateCredential(); err != nil {
		t.Fatal(err)
	}
	if got := validations.Load(); got != 2 {
		t.Fatalf("validations after lease=%d, want 2", got)
	}
	conn.rejected.Store(true)
	if err := conn.ValidateCredential(); !errors.Is(err, ErrAgentCredentialRejected) {
		t.Fatalf("local invalidation error=%v", err)
	}
}
