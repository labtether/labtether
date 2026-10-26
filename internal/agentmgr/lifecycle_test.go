package agentmgr

import (
	"testing"
	"time"
)

func TestAgentLifecycleCleanupWaitsBeforeSameAssetRegistration(t *testing.T) {
	m := NewManager()
	old, closeOld := newTestConn(t, "node", "linux")
	defer closeOld()
	newer, closeNew := newTestConn(t, "node", "linux")
	defer closeNew()
	other, closeOther := newTestConn(t, "other", "linux")
	defer closeOther()
	entered := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	finished := make(chan struct{})
	m.RegisterWithLifecycle(old, func(*AgentConn) {
		// Reads must stay usable during cleanup; callbacks use other owners.
		if m.IsConnected("node") {
			t.Error("disconnected node is still visible")
		}
		close(entered)
		<-release
	}, nil)
	go func() {
		defer close(finished)
		m.UnregisterIfMatch(old.AssetID, old)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start")
	}
	registerStarted := make(chan struct{})
	registered := make(chan struct{})
	go func() {
		close(registerStarted)
		m.Register(newer)
		close(registered)
	}()
	<-registerStarted
	otherRegistered := make(chan struct{})
	go func() { m.Register(other); close(otherRegistered) }()
	select {
	case <-otherRegistered:
	case <-time.After(time.Second):
		t.Fatal("unrelated asset registration blocked by node cleanup")
	}
	select {
	case <-registered:
		close(release)
		t.Fatal("replacement registered while old cleanup could still close it")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not finish")
	}
	select {
	case <-registered:
	case <-time.After(time.Second):
		t.Fatal("replacement remained blocked")
	}
	if current, ok := m.Get("node"); !ok || current != newer {
		t.Fatal("replacement was not preserved")
	}
}

func TestAgentLifecycleReplacementCleansBeforePublishingAndRejectsOldCleanup(t *testing.T) {
	m := NewManager()
	old, closeOld := newTestConn(t, "node", "linux")
	defer closeOld()
	newer, closeNew := newTestConn(t, "node", "linux")
	defer closeNew()
	cleaned := false
	published := false
	m.RegisterWithLifecycle(old, func(previous *AgentConn) {
		if previous != old || previous.ValidateCredential() == nil {
			t.Fatal("replacement did not retire the old socket")
		}
		if m.IsConnected("node") {
			t.Fatal("new socket exposed before cleanup")
		}
		cleaned = true
	}, nil)
	m.RegisterWithLifecycle(newer, nil, func() {
		current, _ := m.Get("node")
		if !cleaned || current != newer {
			t.Fatal("new state published out of order")
		}
		published = true
	})
	if !published {
		t.Fatal("replacement state was not published")
	}
	if m.UnregisterIfMatch(old.AssetID, old) {
		t.Fatal("stale cleanup accepted")
	}
	if current, _ := m.Get("node"); current != newer {
		t.Fatal("replacement removed")
	}
}

func TestAgentLifecycleUnregisterWaitsForPresencePublication(t *testing.T) {
	m := NewManager()
	conn, closeConn := newTestConn(t, "node", "linux")
	defer closeConn()
	publishing := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	registered := make(chan struct{})
	cleaned := make(chan struct{})
	go func() {
		m.RegisterWithLifecycle(conn, func(*AgentConn) { close(cleaned) }, func() {
			close(publishing)
			<-release
		})
		close(registered)
	}()
	select {
	case <-publishing:
	case <-time.After(time.Second):
		t.Fatal("presence publication did not start")
	}
	started := make(chan struct{})
	unregistered := make(chan struct{})
	go func() {
		close(started)
		m.UnregisterIfMatch(conn.AssetID, conn)
		close(unregistered)
	}()
	<-started
	select {
	case <-unregistered:
		t.Fatal("revocation completed before presence publication")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	for _, done := range []<-chan struct{}{registered, unregistered, cleaned} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("lifecycle did not finish after publication")
		}
	}
	if m.IsConnected(conn.AssetID) {
		t.Fatal("revoked connection remained published")
	}
}
