package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// TestBroadcasterConcurrentWrites verifies that concurrent broadcasts
// do not panic or corrupt data (regression for F1: concurrent WS writes).
func TestBroadcasterConcurrentWrites(t *testing.T) {
	eb := newEventBroadcaster()

	serverConn, cleanup := createTestWSPair(t)
	defer cleanup()

	client := eb.Register(serverConn)
	defer eb.Unregister(client)

	// Fire many concurrent broadcasts — should not panic.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			eb.Broadcast("test.event", map[string]any{"n": n})
		}(i)
	}
	wg.Wait()

	// If we got here without panic, the per-connection mutex is working.
}

// TestBroadcasterRegisterUnregister verifies the Register/Unregister API.
func TestBroadcasterRegisterUnregister(t *testing.T) {
	eb := newEventBroadcaster()

	serverConn, cleanup := createTestWSPair(t)
	defer cleanup()

	client := eb.Register(serverConn)
	if client == nil {
		t.Fatal("expected non-nil client from Register")
	}
	if eb.Count() != 1 {
		t.Fatalf("expected 1 client, got %d", eb.Count())
	}

	eb.Unregister(client)
	if eb.Count() != 0 {
		t.Fatalf("expected 0 clients after unregister, got %d", eb.Count())
	}
}

func TestBroadcasterDropsBackpressuredClientWithoutBlocking(t *testing.T) {
	eb := newEventBroadcaster()

	serverConn, cleanup := createTestWSPair(t)
	defer cleanup()

	outgoing := make(chan []byte, 1)
	client := shared.NewBrowserClientForTesting(serverConn, outgoing)
	outgoing <- []byte("already-full")
	eb.InjectClientForTesting(client)

	start := time.Now()
	eb.Broadcast("test.event", map[string]any{"slow": true})
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("expected broadcast to avoid head-of-line blocking on a full client queue")
	}
	if eb.Count() != 0 {
		t.Fatalf("expected slow client to be evicted after queue backpressure, got %d clients", eb.Count())
	}
}

func TestDeliverFileResponseWaitsForBridgeCapacity(t *testing.T) {
	sut := newTestAPIServer(t)
	bridge := newFileBridge(1, "node-audit")
	defer bridge.Close()
	sut.fileBridges.Store("req-1", bridge)
	defer sut.fileBridges.Delete("req-1")

	first := agentmgr.Message{ID: "first"}
	second := agentmgr.Message{ID: "second"}
	bridge.Ch <- first

	delivered := make(chan struct{})
	go func() {
		sut.deliverFileResponse("req-1", second)
		close(delivered)
	}()

	select {
	case <-delivered:
		t.Fatal("expected second delivery to block while the bridge buffer is full")
	case <-time.After(50 * time.Millisecond):
	}

	gotFirst := <-bridge.Ch
	if gotFirst.ID != first.ID {
		t.Fatalf("expected first message to drain first, got %+v", gotFirst)
	}

	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("expected second delivery to complete once capacity is available")
	}

	gotSecond := <-bridge.Ch
	if gotSecond.ID != second.ID {
		t.Fatalf("expected second message after capacity returned, got %+v", gotSecond)
	}
}

// createTestWSPair creates a server-side WebSocket connection for testing.
// Returns the server-side *websocket.Conn and a cleanup function.
func createTestWSPair(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()

	var serverConn *websocket.Conn
	ready := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConn = conn
		close(ready)
		// Keep handler alive until connection closes.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))

	wsURL := "ws" + ts.URL[4:] + "/ws"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		ts.Close()
		t.Fatalf("dial failed: %v", err)
	}

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		clientConn.Close()
		ts.Close()
		t.Fatal("timeout waiting for server WebSocket")
	}

	return serverConn, func() {
		clientConn.Close()
		ts.Close()
	}
}
