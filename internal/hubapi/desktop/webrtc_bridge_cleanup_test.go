package desktop

import (
	"testing"
	"time"
)

func TestWebRTCBridgeCloseDoesNotWaitForBlockedWriter(t *testing.T) {
	bridge := &WebRTCSignalingBridge{ClosedCh: make(chan struct{})}
	bridge.mu.Lock() // The writer holds this until its socket write completes.
	finished := make(chan struct{})
	go func() { bridge.Close(); close(finished) }()
	select {
	case <-finished:
		bridge.mu.Unlock()
	case <-time.After(time.Second):
		bridge.mu.Unlock()
		<-finished
		t.Fatal("Close waited for the writer it must unblock")
	}
	select {
	case <-bridge.ClosedCh:
	default:
		t.Fatal("bridge close signal was not delivered")
	}
}
