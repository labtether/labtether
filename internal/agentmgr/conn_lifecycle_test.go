package agentmgr

import (
	"testing"
	"time"
)

func TestAgentConnCloseInterruptsBlockedWriter(t *testing.T) {
	conn, cleanup := newTestConn(t, "blocked-writer", "linux")
	defer cleanup()
	done := conn.Done()
	conn.writeMu.Lock()
	released := false
	defer func() {
		if !released {
			conn.writeMu.Unlock()
		}
	}()
	closed := make(chan struct{})
	go func() { conn.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close waits for a blocked writer")
	}
	select {
	case <-done:
	default:
		t.Fatal("disconnect waiters were not released")
	}
	conn.writeMu.Unlock()
	released = true
	conn.Close()
	if err := conn.Send(Message{Type: MsgConfigUpdate}); err != ErrAgentCredentialRejected {
		t.Fatalf("retired connection accepted send: %v", err)
	}
}
