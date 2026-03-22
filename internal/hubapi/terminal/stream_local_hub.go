package terminal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/terminal"
)

const (
	HubLocalTerminalTarget        = "__hub__"
	EnvEnableHubLocalTerminal     = "LABTETHER_ENABLE_LOCAL_TERMINAL"
	EnvHubLocalTerminalShell      = "LABTETHER_LOCAL_TERMINAL_SHELL"
	hubLocalTerminalStartupWait   = 8 * time.Second
	hubLocalTerminalDefaultCols   = 160
	hubLocalTerminalDefaultRows   = 40
	hubLocalTerminalOutputBufSize = 64 * 1024
)

func IsHubLocalTerminalTarget(target string) bool {
	return strings.EqualFold(strings.TrimSpace(target), HubLocalTerminalTarget)
}

func HubLocalTerminalEnabled() bool {
	return shared.EnvOrDefaultBool(EnvEnableHubLocalTerminal, false)
}

func ResolveHubLocalTerminalCommand() (string, []string, error) {
	override := strings.TrimSpace(shared.EnvOrDefault(EnvHubLocalTerminalShell, ""))
	if override != "" {
		fields := strings.Fields(override)
		if len(fields) == 0 {
			return "", nil, fmt.Errorf("%s is empty", EnvHubLocalTerminalShell)
		}
		return fields[0], fields[1:], nil
	}

	for _, candidate := range []string{"/bin/bash", "/bin/ash", "/bin/sh"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, []string{"-l"}, nil
		}
	}
	return "", nil, errors.New("no supported local shell found (/bin/bash, /bin/ash, /bin/sh)")
}

func (d *Deps) HandleHubLocalTerminalStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	traceID := shared.BrowserStreamTraceID(r)
	traceLog := shared.StreamTraceLogValue(traceID)
	logContext := fmt.Sprintf("session=%s target=%s trace=%s", session.ID, session.Target, traceLog)

	wsConn, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("terminal-local: upgrade_failed %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local upgrade errors.
		return
	}
	defer wsConn.Close()
	wsConn.SetReadLimit(maxTerminalInputReadBytes)
	connectStart := time.Now()
	_ = WriteTerminalStatus(wsConn, "hub_local_shell_starting", "Starting hub local diagnostics shell...", 0, 0, 0)

	shellBinary, shellArgs, err := ResolveHubLocalTerminalCommand()
	if err != nil {
		log.Printf("terminal-local: stream_setup_failed reason=shell_unavailable %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local runtime errors.
		_ = WriteTerminalError(wsConn, "shell_unavailable", fmt.Sprintf("Local shell unavailable: %v", err))
		return
	}

	cmd, err := securityruntime.NewCommand(shellBinary, shellArgs...)
	if err != nil {
		log.Printf("terminal-local: stream_setup_failed reason=shell_policy_blocked %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local policy errors.
		_ = WriteTerminalError(wsConn, "shell_policy_blocked", fmt.Sprintf("Local shell blocked by runtime policy: %v", err))
		return
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cmd.Dir = "/"

	cols, rows := ParseTerminalSize(r.URL.Query().Get("cols"), r.URL.Query().Get("rows"))
	if cols <= 0 {
		cols = hubLocalTerminalDefaultCols
	}
	if rows <= 0 {
		rows = hubLocalTerminalDefaultRows
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		log.Printf("terminal-local: stream_setup_failed reason=pty_start_failed %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local PTY startup errors.
		_ = WriteTerminalError(wsConn, "pty_start_failed", fmt.Sprintf("Failed to start local PTY shell: %v", err))
		return
	}
	defer func() {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	log.Printf("terminal-local: stream_connected %s shell=%s cols=%d rows=%d", logContext, shellBinary, cols, rows) // #nosec G706 -- Log context and shell path are bounded local runtime values.
	_ = WriteTerminalReady(wsConn, "Local hub diagnostics shell connected", time.Since(connectStart).Milliseconds())

	var writeMu sync.Mutex
	stopKeepalive := shared.StartBrowserWebSocketKeepalive(wsConn, &writeMu, "terminal-local:"+session.ID)
	defer stopKeepalive()

	writeOutput := func(payload []byte) error {
		if len(payload) == 0 {
			return nil
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = wsConn.SetWriteDeadline(time.Now().Add(TerminalStreamWriteDeadline))
		return wsConn.WriteMessage(websocket.BinaryMessage, payload)
	}

	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		buf := make([]byte, hubLocalTerminalOutputBufSize)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				if err := writeOutput(chunk); err != nil {
					return
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					log.Printf("terminal-local: stream_runtime_event reason=pty_read_failed %s err=%v", logContext, readErr) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.
				}
				return
			}
		}
	}()

	stopCloser := make(chan struct{})
	defer close(stopCloser)
	go func() {
		select {
		case <-streamDone:
			_ = wsConn.SetReadDeadline(time.Now())
			_ = wsConn.Close()
		case <-stopCloser:
		}
	}()

	endReason := "unknown"
	var endErr error
	for {
		messageType, payload, readErr := wsConn.ReadMessage()
		if readErr != nil {
			select {
			case <-streamDone:
				endReason = "local_shell_closed"
				goto streamEnd
			default:
			}
			switch {
			case websocket.IsCloseError(readErr, websocket.CloseNormalClosure):
				endReason = "browser_ws_closed_normal"
			case websocket.IsCloseError(readErr, websocket.CloseGoingAway):
				endReason = "browser_ws_closed_going_away"
			default:
				var closeErr *websocket.CloseError
				switch {
				case errors.As(readErr, &closeErr):
					endReason = fmt.Sprintf("browser_ws_closed_code_%d", closeErr.Code)
				default:
					var netErr net.Error
					if errors.As(readErr, &netErr) && netErr.Timeout() {
						endReason = "browser_ws_read_timeout"
					} else {
						endReason = "browser_ws_read_error"
					}
				}
				endErr = readErr
			}
			break
		}
		_ = shared.TouchBrowserWebSocketReadDeadline(wsConn)
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		handled, controlErr := HandleLocalTerminalControlMessage(ptmx, payload)
		if controlErr != nil {
			endReason = "control_message_error"
			endErr = controlErr
			_ = WriteTerminalError(wsConn, "control_message_error", "Terminal control channel failed")
			break
		}
		if handled {
			continue
		}
		if _, writeErr := ptmx.Write(payload); writeErr != nil {
			endReason = "pty_write_failed"
			endErr = writeErr
			_ = WriteTerminalError(wsConn, "pty_write_failed", "Local PTY input channel closed")
			break
		}
	}

streamEnd:
	if endErr != nil {
		log.Printf("terminal-local: stream_ended reason=%s %s elapsed=%s err=%v", endReason, logContext, time.Since(connectStart).Round(time.Millisecond), endErr) // #nosec G706 -- Reason and context are bounded runtime values.
	} else {
		log.Printf("terminal-local: stream_ended reason=%s %s elapsed=%s", endReason, logContext, time.Since(connectStart).Round(time.Millisecond)) // #nosec G706 -- Reason and context are bounded runtime values.
	}
}

func HandleLocalTerminalControlMessage(ptmx *os.File, payload []byte) (bool, error) {
	if ptmx == nil || len(payload) == 0 {
		return false, nil
	}
	if trimmed := strings.TrimSpace(string(payload)); trimmed == "" || trimmed[0] != '{' {
		return false, nil
	}

	msg := TerminalControlMessage{}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return false, nil
	}

	switch strings.ToLower(strings.TrimSpace(msg.Type)) {
	case "input":
		if msg.Data == "" {
			return true, nil
		}
		_, err := io.WriteString(ptmx, msg.Data)
		return true, err
	case "resize":
		cols := msg.Cols
		rows := msg.Rows
		if cols <= 0 {
			cols = hubLocalTerminalDefaultCols
		}
		if rows <= 0 {
			rows = hubLocalTerminalDefaultRows
		}
		return true, pty.Setsize(ptmx, &pty.Winsize{
			Cols: uint16(cols),
			Rows: uint16(rows),
		})
	case "ping":
		return true, nil
	default:
		return false, nil
	}
}
