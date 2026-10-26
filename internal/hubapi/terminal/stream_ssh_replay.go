package terminal

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
	"golang.org/x/crypto/ssh"
)

const sshReplayMaxBytes = 1024 * 1024
const sshReplayTimeout = 5 * time.Second

// Stored history is best-effort. A timed-out live capture closes the owned
// connection, so its error must reach the caller before it announces ready.
func (d *Deps) replayScrollbackOnReconnect(wsConn *websocket.Conn, sshClient *ssh.Client, session terminal.Session, logContext string, closeChain func() error) error {
	persistentSessionID := strings.TrimSpace(session.PersistentSessionID)
	if persistentSessionID == "" {
		return nil
	}
	var stored []byte
	if d.TerminalScrollbackStore != nil {
		var err error
		stored, err = d.TerminalScrollbackStore.GetScrollback(persistentSessionID)
		if err != nil {
			log.Printf("terminal-ssh: scrollback_replay_skip reason=db_read_failed %s err=%v", logContext, err) // #nosec G706 -- Bounded session identifiers and database errors.
		}
		stored = sshReplayTail(stored)
	}
	var tmuxCapture []byte
	if name := strings.TrimSpace(session.TmuxSessionName); name != "" && sshClient != nil {
		command := "tmux capture-pane -t " + shared.ShellSingleQuote(name) + " -p -e -S -20000 2>/dev/null || true"
		var err error
		tmuxCapture, err = captureSSHScrollback(sshClient, command, closeChain, sshReplayTimeout)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return fmt.Errorf("capture terminal scrollback: %w", err)
			}
			tmuxCapture = nil
			log.Printf("terminal-ssh: scrollback_replay_skip reason=capture_failed %s err=%v", logContext, err) // #nosec G706 -- Bounded session identifiers and connection errors.
		}
	}
	merged := sshReplayTail(terminal.MergeScrollback(stored, tmuxCapture))
	if len(merged) == 0 {
		return nil
	}
	if err := wsConn.SetWriteDeadline(time.Now().Add(TerminalStreamWriteDeadline)); err != nil {
		return err
	}
	return wsConn.WriteMessage(websocket.BinaryMessage, merged)
}

// The deadline covers opening, running and closing the exec channel. Merely
// closing that channel cannot interrupt an unresponsive jump host.
func captureSSHScrollback(client *ssh.Client, command string, closeChain func() error, timeout time.Duration) ([]byte, error) {
	timedOut := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		close(timedOut)
		_ = closeChain()
	})
	output := &sshReplayOutput{limit: sshReplayMaxBytes}
	session, err := client.NewSession()
	if err == nil {
		session.Stdout, session.Stderr = output, output
		err = session.Run(command)
		_ = session.Close()
	}
	if !timer.Stop() {
		<-timedOut
		return nil, os.ErrDeadlineExceeded
	}
	return output.bytes(), err
}

func sshReplayTail(data []byte) []byte {
	if len(data) > sshReplayMaxBytes {
		return data[len(data)-sshReplayMaxBytes:]
	}
	return data
}

// Drain all output while retaining only recent history. stdout and stderr can
// write concurrently, so both share this bounded writer.
type sshReplayOutput struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func (output *sshReplayOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	n := len(data)
	if n >= output.limit {
		output.data = append(output.data[:0], data[n-output.limit:]...)
		return n, nil
	}
	if overflow := len(output.data) + n - output.limit; overflow > 0 {
		copy(output.data, output.data[overflow:])
		output.data = output.data[:len(output.data)-overflow]
	}
	output.data = append(output.data, data...)
	return n, nil
}

func (output *sshReplayOutput) bytes() []byte {
	output.mu.Lock()
	defer output.mu.Unlock()
	return append([]byte(nil), output.data...)
}
