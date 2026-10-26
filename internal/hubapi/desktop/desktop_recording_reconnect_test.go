package desktop

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

func TestSlowRecordingSetupCannotReplaceNewDesktop(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	accepted := make(chan *websocket.Conn, 1)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			accepted <- ws
		}
	}))
	defer agentServer.Close()
	peer := desktopTestSocket(t, agentServer.URL)
	var socket *websocket.Conn
	select {
	case socket = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("agent did not connect")
	}
	conn := agentmgr.NewAgentConn(socket, "node", "linux")
	defer conn.Close()
	manager := agentmgr.NewManager()
	manager.Register(conn)
	var wires sync.Map
	var optionsMu sync.RWMutex
	options := map[string]DesktopSessionOptions{"public": {Record: true}}
	blocked, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var recordings, requests atomic.Int32
	stopped := make(chan string, 2)
	d := &Deps{
		DesktopBridges: &wires, AgentMgr: manager, TerminalWebSocketUpgrader: &upgrader, MaxDesktopInputReadBytes: 1024,
		DesktopSessionMu: &optionsMu, DesktopSessionOpts: &options,
		BrowserStreamTraceID: func(*http.Request) string { return "trace" },
		StreamTraceLogValue:  func(v string) string { return v }, SanitizeAgentStreamReason: func(v string) string { return v },
		StartBrowserWSKeepalive:    func(*websocket.Conn, *sync.Mutex, string) func() { return func() {} },
		TouchBrowserWSReadDeadline: func(*websocket.Conn) error { return nil },
		StartRecording: func(string, string, string, string) (*ActiveRecording, error) {
			if recordings.Add(1) == 1 {
				close(blocked)
				<-release
				return &ActiveRecording{ID: "old"}, nil
			}
			return &ActiveRecording{ID: "new"}, nil
		},
		StopRecording: func(rec *ActiveRecording) { stopped <- rec.ID },
	}
	defer d.CloseDesktopStreamsForConnection(conn)
	session := terminal.Session{ID: "public", Target: "node"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			defer close(finished)
		}
		d.HandleAgentDesktopStream(w, r, session)
	}))
	defer server.Close()
	desktopTestSocket(t, server.URL)
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("recording setup did not block")
	}
	current := desktopTestSocket(t, server.URL)
	start := readDesktopWire(t, peer)
	if start.Type != agentmgr.MsgDesktopStart {
		t.Fatalf("unexpected start: %+v", start)
	}
	unblock()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("old recording setup did not unwind")
	}
	select {
	case id := <-stopped:
		if id != "old" {
			t.Fatalf("stopped recording %q", id)
		}
	default:
		t.Fatal("old recording was not stopped")
	}
	d.ProcessAgentDesktopStarted(conn, desktopWireMessage(t, agentmgr.DesktopStartedData{SessionID: start.ID}))
	if err := current.WriteMessage(websocket.BinaryMessage, []byte("input")); err != nil {
		t.Fatal(err)
	}
	message := readDesktopWire(t, peer)
	if message.Type != agentmgr.MsgDesktopData || message.ID != start.ID {
		t.Fatalf("old setup stole new stream: %+v", message)
	}
	_ = current.Close()
	message = readDesktopWire(t, peer)
	if message.Type != agentmgr.MsgDesktopClose || message.ID != start.ID {
		t.Fatalf("wrong final stop: %+v", message)
	}
}
