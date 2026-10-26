package agentmgr

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentMessageLifecycleFinishesBeforeReplacementCleanup(t *testing.T) {
	manager := NewManager()
	old, closeOld := newTestConn(t, "node", "linux")
	defer closeOld()
	fresh, closeFresh := newTestConn(t, "node", "linux")
	defer closeFresh()
	var state atomic.Int32
	manager.RegisterWithLifecycle(old, func(*AgentConn) { state.Store(2) }, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	dispatched := make(chan bool, 1)
	go func() {
		dispatched <- manager.DispatchIfCurrent(old, func() { close(entered); <-release; state.Store(1) })
	}()
	<-entered
	published := make(chan struct{})
	go func() { manager.RegisterWithLifecycle(fresh, nil, func() { state.Store(3) }); close(published) }()
	select {
	case <-published:
		t.Fatal("replacement published while old message handler was active")
	case <-time.After(30 * time.Millisecond):
	}
	// Reads and other assets remain usable while this handler is held.
	other, closeOther := newTestConn(t, "other", "linux")
	defer closeOther()
	otherReady := make(chan struct{})
	go func() { manager.Register(other); manager.Get(old.AssetID); close(otherReady) }()
	select {
	case <-otherReady:
	case <-time.After(time.Second):
		t.Fatal("one asset's handler blocked unrelated manager work")
	}
	close(release)
	released = true
	select {
	case ok := <-dispatched:
		if !ok {
			t.Fatal("current handler was rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not finish")
	}
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("replacement did not finish")
	}
	if state.Load() != 3 {
		t.Fatalf("old handler overwrote new state: %d", state.Load())
	}
	called := false
	if manager.DispatchIfCurrent(old, func() { called = true }) || called {
		t.Fatal("retired connection dispatched another message")
	}
	if !manager.DispatchIfCurrent(fresh, func() { state.Store(4) }) || state.Load() != 4 {
		t.Fatal("new connection cannot dispatch")
	}
}

func TestAgentMessageLifecycleClosesRejectedOwnerAfterHandler(t *testing.T) {
	manager := NewManager()
	conn, cleanup := newTestConn(t, "revoked", "linux")
	defer cleanup()
	cleaned := 0
	manager.RegisterWithLifecycle(conn, func(*AgentConn) { cleaned++ }, nil)
	finished := make(chan struct{})
	go func() { manager.DispatchIfCurrent(conn, func() { conn.Close() }); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("handler retirement deadlocked its lifecycle")
	}
	if manager.IsConnected(conn.AssetID) || cleaned != 1 {
		t.Fatalf("retired owner cleanup: connected=%v, count=%d", manager.IsConnected(conn.AssetID), cleaned)
	}
	manager.UnregisterIfMatch(conn.AssetID, conn)
	if cleaned != 1 {
		t.Fatal("read-loop cleanup ran twice")
	}
}
