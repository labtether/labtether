package terminal

import (
	"bytes"
	"encoding/base64"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

func TestAgentTerminalSavedReplayKeepsQueuedOutputExactlyOnce(t *testing.T) {
	for _, source := range []string{"memory_ring", "stored_history_only"} {
		t.Run(source, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			session := terminalmodel.Session{ID: "public-replay", Target: "node-1", PersistentSessionID: "saved", TmuxSessionName: "lt-saved"}
			d, conn, peer, url, finished := newAgentTerminalHarnessWithListener(t, session, func(listener net.Listener) net.Listener {
				return &agentTerminalStartupListener{Listener: listener, entered: entered, release: release}
			})
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			d.TerminalScrollbackStore = &agentScrollbackTestStore{buffer: []byte("old history\r\n")}
			if source == "memory_ring" {
				d.TerminalInMemStore = terminalmodel.NewStore()
			}
			browser := dialAgentTerminalTestBrowser(t, url+"?request=replay")
			defer browser.Close()
			start := readAgentTerminalStart(t, readAgentTerminalWire(t, peer))
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("handler did not reach the post-start write barrier")
			}
			// Both real wire frames arrive before startup can choose ready/output
			// or replay history. This deterministically exposed the duplicate.
			for _, data := range []string{"first new line\r\n", "last \U0001f600 line\n"} {
				d.ProcessAgentTerminalData(conn, agentTerminalMessage(t, agentmgr.TerminalDataPayload{
					SessionID: start.SessionID, Data: base64.StdEncoding.EncodeToString([]byte(data)),
				}))
			}
			d.ProcessAgentTerminalClosed(conn, agentTerminalMessage(t, agentmgr.TerminalCloseData{SessionID: start.SessionID, Reason: "shell exited"}))
			releaseOnce.Do(func() { close(release) })
			_ = browser.SetReadDeadline(time.Now().Add(3 * time.Second))
			var output string
			for {
				kind, data, err := browser.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
						t.Fatalf("saved session exit did not send a normal close: %v", err)
					}
					break
				}
				if kind == websocket.BinaryMessage {
					output += string(data)
				}
			}
			if want := "old history\r\nfirst new line\r\nlast \U0001f600 line\n"; output != want {
				t.Fatalf("replay duplicated or dropped queued output: %q; want %q", output, want)
			}
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				t.Fatal("saved session handler did not finish")
			}
		})
	}
}

type agentTerminalStartupListener struct {
	net.Listener
	entered chan struct{}
	release <-chan struct{}
}

func (l *agentTerminalStartupListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &agentTerminalStartupConn{Conn: conn, entered: l.entered, release: l.release}, nil
}

type agentTerminalStartupConn struct {
	net.Conn
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (c *agentTerminalStartupConn) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("agent_starting_shell")) {
		c.once.Do(func() {
			close(c.entered)
			<-c.release
		})
	}
	return c.Conn.Write(data)
}
