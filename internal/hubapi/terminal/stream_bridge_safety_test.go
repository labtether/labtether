package terminal_test

import (
	"testing"

	desktoppkg "github.com/labtether/labtether/internal/hubapi/desktop"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
)

func TestTerminalBridgeCloseIsIdempotent(t *testing.T) {
	b := &terminalpkg.TerminalBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	b.Close()
	b.Close()
}

func TestDesktopBridgeCloseIsIdempotent(t *testing.T) {
	b := &desktoppkg.DesktopBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	b.Close()
	b.Close()
}

func TestTerminalBridgeTrySendOutputClosedOutputChannelDoesNotPanic(t *testing.T) {
	b := &terminalpkg.TerminalBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	close(b.OutputCh)
	b.TrySendOutput([]byte("data"))
}

func TestDesktopBridgeTrySendOutputClosedOutputChannelDoesNotPanic(t *testing.T) {
	b := &desktoppkg.DesktopBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	close(b.OutputCh)
	b.TrySendOutput([]byte("data"))
}
