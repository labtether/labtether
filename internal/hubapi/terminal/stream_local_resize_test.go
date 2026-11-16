package terminal

import (
	"testing"

	"github.com/creack/pty"
)

// Fault this guards: a resize message with cols or rows above 65535 was
// converted straight to the PTY's 16-bit fields, so 65536 columns became a
// 0-column terminal. Out-of-range sizes must fall back to the defaults.
func TestLocalTerminalResizeRejectsSizeThatWouldWrap(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no PTY available: %v", err)
	}
	defer func() { _ = ptmx.Close(); _ = tty.Close() }()

	handled, err := HandleLocalTerminalControlMessage(ptmx, []byte(`{"type":"resize","cols":65536,"rows":65577}`))
	if !handled || err != nil {
		t.Fatalf("resize not applied: handled=%v err=%v", handled, err)
	}
	rows, cols, err := pty.Getsize(ptmx)
	if err != nil {
		t.Fatalf("read PTY size: %v", err)
	}
	if cols != hubLocalTerminalDefaultCols || rows != hubLocalTerminalDefaultRows {
		t.Fatalf("PTY size = %dx%d, want default %dx%d", cols, rows, hubLocalTerminalDefaultCols, hubLocalTerminalDefaultRows)
	}
}
