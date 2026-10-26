package desktop

import (
	"testing"
	"time"
)

// Agent desktop frames are delivered from the agent's read loop. A browser
// that stops reading must cost that loop at most the bounded queue wait, after
// which the stream is closed and later frames are dropped immediately.
func TestSlowBrowserIsClosedInsteadOfBlockingAgentDispatch(t *testing.T) {
	bridge := &DesktopBridge{OutputCh: make(chan []byte, 1), ClosedCh: make(chan struct{})}
	bridge.TrySendOutput([]byte("fills the queue"))

	started := time.Now()
	bridge.TrySendOutput([]byte("browser is not reading"))
	if waited := time.Since(started); waited > desktopOutputQueueWait+500*time.Millisecond {
		t.Fatalf("agent dispatch waited %s for a stalled browser", waited)
	}
	select {
	case <-bridge.ClosedCh:
	default:
		t.Fatal("stalled browser stream was not closed")
	}
	if got := bridge.CloseReason(); got != "browser backpressure" {
		t.Fatalf("close reason = %q", got)
	}

	started = time.Now()
	bridge.TrySendOutput([]byte("late frame"))
	if waited := time.Since(started); waited > 50*time.Millisecond {
		t.Fatalf("frame for a closed stream waited %s", waited)
	}
}
