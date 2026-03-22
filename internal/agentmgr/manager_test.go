package agentmgr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
