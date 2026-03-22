package main

import "testing"

func TestTerminalBridgeCloseIsIdempotent(t *testing.T) {
	b := &terminalBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	b.Close()
	b.Close()
}

func TestDesktopBridgeCloseIsIdempotent(t *testing.T) {
	b := &desktopBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	b.Close()
	b.Close()
}

func TestTerminalBridgeTrySendOutputClosedOutputChannelDoesNotPanic(t *testing.T) {
	b := &terminalBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	close(b.OutputCh)
	b.TrySendOutput([]byte("data"))
}

func TestDesktopBridgeTrySendOutputClosedOutputChannelDoesNotPanic(t *testing.T) {
	b := &desktopBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	close(b.OutputCh)
	b.TrySendOutput([]byte("data"))
}
