package terminal

import (
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

func TestAgentTerminalExitDrainsOutputBeforeClosingIdleBrowser(t *testing.T) {
	for _, phase := range []string{"before_started", "while_reading_input"} {
		t.Run(phase, func(t *testing.T) {
			session := terminalmodel.Session{ID: "public-exit", Target: "node-1"}
			d, conn, peer, url, finished := newAgentTerminalHarness(t, session)
			browser := dialAgentTerminalTestBrowser(t, url+"?request=exit")
			defer browser.Close()
			start := readAgentTerminalStart(t, readAgentTerminalWire(t, peer))
			if phase == "while_reading_input" {
				d.ProcessAgentTerminalStarted(conn, agentTerminalMessage(t, agentmgr.TerminalStartedData{SessionID: start.SessionID}))
				readAgentTerminalReady(t, browser)
				if err := browser.WriteMessage(websocket.BinaryMessage, []byte("echo final")); err != nil {
					t.Fatal(err)
				}
				if input := readAgentTerminalWire(t, peer); input.Type != agentmgr.MsgTerminalData {
					t.Fatalf("browser did not reach input loop: %+v", input)
				}
			}
			want := "final line\r\n\u65e5\u672c\u8a9e \U0001f600\n"
			for _, data := range []string{"final line\r\n", "\u65e5\u672c\u8a9e \U0001f600\n"} {
				d.ProcessAgentTerminalData(conn, agentTerminalMessage(t, agentmgr.TerminalDataPayload{
					SessionID: start.SessionID, Data: base64.StdEncoding.EncodeToString([]byte(data)),
				}))
			}
			d.ProcessAgentTerminalClosed(conn, agentTerminalMessage(t, agentmgr.TerminalCloseData{SessionID: start.SessionID, Reason: "shell exited"}))
			_ = browser.SetReadDeadline(time.Now().Add(3 * time.Second))
			var output string
			for {
				kind, data, err := browser.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
						t.Fatalf("clean shell exit did not send a normal close: %v", err)
					}
					break
				}
				if kind == websocket.BinaryMessage {
					output += string(data)
				}
			}
			if output != want {
				t.Fatalf("final output truncated or duplicated: %q; want %q", output, want)
			}
			select {
			case request := <-finished:
				if request != "exit" {
					t.Fatalf("wrong handler ended: %q", request)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("terminal handler retained a blocked input reader")
			}
			stop := readAgentTerminalWire(t, peer)
			if stop.Type != agentmgr.MsgTerminalClose || stop.ID != start.SessionID {
				t.Fatalf("cleanup stopped the wrong PTY: %+v", stop)
			}
		})
	}
}

func TestAgentTerminalOutputFailureReleasesBlockedInput(t *testing.T) {
	var failWrites atomic.Bool
	ready := make(chan *TerminalBridge, 1)
	finished := make(chan struct{})
	d := &Deps{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer close(finished)
		bridge := &TerminalBridge{OutputCh: make(chan []byte, 1), ClosedCh: make(chan struct{}), WireSessionID: "wire-output-failure"}
		ready <- bridge
		var mu sync.Mutex
		_, _ = d.bridgeAgentTerminalIO(ws, nil, bridge, nil, &mu, "test-output-failure")
	}))
	server.Listener = &agentTerminalFailWriteListener{Listener: server.Listener, fail: &failWrites}
	server.Start()
	defer server.Close()
	browser := dialAgentTerminalTestBrowser(t, server.URL)
	defer browser.Close()
	var bridge *TerminalBridge
	select {
	case bridge = <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("output bridge not ready")
	}
	failWrites.Store(true)
	bridge.TrySendOutput([]byte("transport write fails"))
	_ = browser.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := browser.ReadMessage(); err == nil {
		t.Fatal("failed output left browser open")
	} else if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Fatal("failed output was reported as a clean shell exit")
	} else if timeout, ok := err.(interface{ Timeout() bool }); ok && timeout.Timeout() {
		t.Fatal("failed output required a browser read timeout")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("output write failure left input blocked")
	}
}

type agentTerminalFailWriteListener struct {
	net.Listener
	fail *atomic.Bool
}

func (l *agentTerminalFailWriteListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &agentTerminalFailWriteConn{Conn: conn, fail: l.fail}, nil
}

type agentTerminalFailWriteConn struct {
	net.Conn
	fail *atomic.Bool
}

func (c *agentTerminalFailWriteConn) Write(data []byte) (int, error) {
	if c.fail.Load() {
		return 0, errors.New("injected terminal output failure")
	}
	return c.Conn.Write(data)
}

func TestAgentTerminalStartRejectsClosedOrReplacedOwner(t *testing.T) {
	for _, closeBeforeStart := range []bool{false, true} {
		var bridges sync.Map
		d := &Deps{TerminalBridges: &bridges}
		bridge := &TerminalBridge{SessionID: "public", WireSessionID: "wire-old", OutputCh: make(chan []byte, 1), ClosedCh: make(chan struct{})}
		d.registerAgentTerminalStream(bridge, "", nil)
		if closeBeforeStart {
			bridge.Close()
		} else {
			replacement := &TerminalBridge{SessionID: "public", WireSessionID: "wire-new", OutputCh: make(chan []byte, 1), ClosedCh: make(chan struct{})}
			d.registerAgentTerminalStream(replacement, "", nil)
		}
		// No AgentConn is attached: reaching transport.Send would panic, proving
		// a closed or displaced startup cannot get as far as sending a start.
		if err := d.startAgentTerminalStream(bridge, "", nil, agentmgr.Message{Type: agentmgr.MsgTerminalStart}); err == nil {
			t.Fatal("closed or replaced stream was allowed to start")
		}
	}
}

func TestAgentTerminalOutputOverflowClosesInsteadOfSkippingBytes(t *testing.T) {
	ring := terminalmodel.NewRingBuffer(10)
	bridge := &TerminalBridge{OutputCh: make(chan []byte, 1), ClosedCh: make(chan struct{}), Scrollback: ring}
	bridge.TrySendOutput([]byte("prefix\n"))
	bridge.TrySendOutput([]byte("overflow\n"))
	bridge.TrySendOutput([]byte("late bytes\n"))
	select {
	case <-bridge.ClosedCh:
	default:
		t.Fatal("full output queue silently discarded bytes and kept the stream open")
	}
	if bridge.CloseReasonOr("") != "output_overflow" {
		t.Fatalf("wrong overflow reason: %q", bridge.CloseReasonOr(""))
	}
	if output := <-bridge.OutputCh; string(output) != "prefix\n" {
		t.Fatalf("queued prefix changed: %q", output)
	}
	if stored := string(ring.Snapshot()); stored != "prefix\noverflow\n" {
		t.Fatalf("reconnect output lost the overflow or accepted late bytes: %q", stored)
	}
}
