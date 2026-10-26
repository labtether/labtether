package agentmgr

import (
	"errors"
	"testing"
	"time"
)

// within fails the test when fn does not return promptly.
func within(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { fn(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("%s blocked behind a stalled agent connection", what)
	}
}

// One agent whose socket write or credential check stalls must not stall
// routing for every agent: manager reads, registration and the stalled
// agent's own read loop (TouchLastMessage) must stay fast.
func TestStalledConnectionDoesNotBlockManagerRouting(t *testing.T) {
	m := NewManager()
	stalled, cleanupStalled := newTestConn(t, "stalled", "linux")
	defer cleanupStalled()
	m.Register(stalled)

	// Stall 1: a write in progress holds only the write mutex.
	stalled.writeMu.Lock()
	writeReleased := false
	defer func() {
		if !writeReleased {
			stalled.writeMu.Unlock()
		}
	}()

	// Stall 2: a credential check blocked on a slow database.
	validatorEntered := make(chan struct{})
	releaseValidator := make(chan struct{})
	stalled.SetCredentialValidator(func() error {
		close(validatorEntered)
		<-releaseValidator
		return errors.New("credential revoked")
	})
	sendDone := make(chan error, 1)
	go func() { sendDone <- stalled.Send(Message{Type: MsgConfigUpdate}) }()
	select {
	case <-validatorEntered:
	case <-time.After(time.Second):
		t.Fatal("send never reached the credential check")
	}

	within(t, "TouchLastMessage", stalled.TouchLastMessage)
	within(t, "Meta", func() { _ = stalled.Meta("terminal.tmux.has") })
	within(t, "ConnectedAssetsInfo", func() { _ = m.ConnectedAssetsInfo() })

	other, cleanupOther := newTestConn(t, "healthy", "linux")
	defer cleanupOther()
	within(t, "Register of another agent", func() { m.Register(other) })
	within(t, "Get", func() {
		if conn, ok := m.Get("healthy"); !ok || conn != other {
			t.Error("healthy agent was not routable")
		}
		if _, ok := m.Get("stalled"); !ok {
			t.Error("stalled agent disappeared from routing")
		}
	})
	// Replacing the stalled agent must not wait for its write or credential check.
	replacement, cleanupReplacement := newTestConn(t, "stalled", "linux")
	defer cleanupReplacement()
	within(t, "Register replacing the stalled agent", func() { m.Register(replacement) })

	close(releaseValidator)
	stalled.writeMu.Unlock()
	writeReleased = true
	select {
	case err := <-sendDone:
		if !errors.Is(err, ErrAgentCredentialRejected) {
			t.Fatalf("stalled send error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stalled send never finished")
	}
}
