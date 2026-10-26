package terminal

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

func agentTerminalMessage(t *testing.T, payload any) agentmgr.Message {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return agentmgr.Message{Data: raw}
}

func TestAgentTerminalLateFramesAndFinalizerCannotReachReplacement(t *testing.T) {
	var bridges sync.Map
	d := &Deps{TerminalBridges: &bridges}
	conn := &agentmgr.AgentConn{AssetID: "node-1"}
	conn.SetMeta("terminal.tmux.has", "true")
	ring := terminalmodel.NewRingBuffer(100)
	makeBridge := func(wire string) *TerminalBridge {
		return &TerminalBridge{SessionID: "public", WireSessionID: wire, ExpectedAgentID: "node-1", AgentConn: conn,
			OutputCh: make(chan []byte, 4), ClosedCh: make(chan struct{}), RequireTmux: true, Scrollback: ring}
	}
	old, current := makeBridge("wire-old"), makeBridge("wire-new")
	bridges.Store(old.WireSessionID, old)
	bridges.Store(current.WireSessionID, current)
	d.ActiveAgentStreams.Store("public", current)
	old.CloseWithReason("session_attached_elsewhere")
	d.ProcessAgentTerminalStarted(conn, agentTerminalMessage(t, agentmgr.TerminalStartedData{SessionID: old.WireSessionID}))
	d.ProcessAgentTerminalData(conn, agentTerminalMessage(t, agentmgr.TerminalDataPayload{
		SessionID: old.WireSessionID, Data: base64.StdEncoding.EncodeToString([]byte("late old output")),
	}))
	d.ProcessAgentTerminalClosed(conn, agentTerminalMessage(t, agentmgr.TerminalCloseData{SessionID: old.WireSessionID}))
	if conn.Meta("terminal.tmux.has") != "true" || len(ring.Snapshot()) != 0 {
		t.Fatal("closed stream changed replacement capability or shared scrollback")
	}
	var closed []string
	closeFn := func(_ *agentmgr.AgentConn, id string) { closed = append(closed, id) }
	d.FinalizeAgentTerminalSession("public", old, conn, true, closeFn)
	d.FinalizeAgentTerminalSession("public", old, conn, true, closeFn)
	if len(closed) != 1 || closed[0] != old.WireSessionID {
		t.Fatalf("finalizer closed the wrong wire stream: %v", closed)
	}
	if got, _ := bridges.Load(current.WireSessionID); got != current {
		t.Fatal("old finalizer removed replacement dispatch")
	}
	if got, _ := d.ActiveAgentStreams.Load("public"); got != current {
		t.Fatal("old finalizer removed replacement public owner")
	}
	select {
	case <-current.ClosedCh:
		t.Fatal("late old frames closed replacement")
	default:
	}
	other := &agentmgr.AgentConn{AssetID: "node-1"}
	d.AgentMgr = agentmgr.NewManager()
	d.AgentMgr.Register(conn)
	stale := makeBridge("wire-stale-start")
	stale.AgentConn = other
	d.registerAgentTerminalStream(stale, "", nil)
	if owner, _ := d.ActiveAgentStreams.Load("public"); owner != current {
		t.Fatal("late registration from replaced agent evicted current owner")
	}
	if _, exists := bridges.Load(stale.WireSessionID); exists {
		t.Fatal("late registration from replaced agent gained a wire route")
	}
	d.ProcessAgentTerminalStarted(other, agentTerminalMessage(t, agentmgr.TerminalStartedData{SessionID: current.WireSessionID}))
	d.ProcessAgentTerminalData(other, agentTerminalMessage(t, agentmgr.TerminalDataPayload{
		SessionID: current.WireSessionID, Data: base64.StdEncoding.EncodeToString([]byte("old connection output")),
	}))
	d.ProcessAgentTerminalClosed(other, agentTerminalMessage(t, agentmgr.TerminalCloseData{SessionID: current.WireSessionID}))
	d.CloseTerminalBridgesForConnection(other)
	select {
	case <-current.ClosedCh:
		t.Fatal("different connection for the same asset closed replacement")
	default:
	}
	if len(ring.Snapshot()) != 0 || other.Meta("terminal.tmux.has") != "" {
		t.Fatal("different connection changed current output or capability")
	}
	d.CloseTerminalBridgesForConnection(conn)
	select {
	case <-current.ClosedCh:
	default:
		t.Fatal("exact connection disconnect missed its wire-keyed bridge")
	}
}

func TestAgentTerminalReconnectKeepsWireIdentityThroughSavedSessionDelete(t *testing.T) {
	session := terminalmodel.Session{ID: "public", Target: "node-1", PersistentSessionID: "saved", TmuxSessionName: "lt-saved"}
	d, conn, agentPeer, browserURL, _ := newAgentTerminalHarness(t, session)
	first := dialAgentTerminalTestBrowser(t, browserURL)
	defer first.Close()
	start1 := readAgentTerminalStart(t, readAgentTerminalWire(t, agentPeer))
	if start1.SessionID == "" || start1.SessionID == session.ID || start1.TmuxSession != "lt-saved" {
		t.Fatalf("first stream did not separate wire and persistent identity: %+v", start1)
	}
	d.ProcessAgentTerminalStarted(conn, agentTerminalMessage(t, agentmgr.TerminalStartedData{SessionID: start1.SessionID, TmuxAttached: true}))
	readAgentTerminalReady(t, first)
	second := dialAgentTerminalTestBrowser(t, browserURL)
	defer second.Close()
	var start2 agentmgr.TerminalStartData
	oldClosed := false
	for i := 0; i < 2; i++ {
		message := readAgentTerminalWire(t, agentPeer)
		switch message.Type {
		case agentmgr.MsgTerminalStart:
			start2 = readAgentTerminalStart(t, message)
		case agentmgr.MsgTerminalClose:
			var closeData agentmgr.TerminalCloseData
			if err := json.Unmarshal(message.Data, &closeData); err != nil {
				t.Fatal(err)
			}
			oldClosed = closeData.SessionID == start1.SessionID && message.ID == start1.SessionID
		default:
			t.Fatalf("unexpected reconnect message: %s", message.Type)
		}
	}
	if !oldClosed || start2.SessionID == "" || start2.SessionID == start1.SessionID || start2.SessionID == session.ID || start2.TmuxSession != "lt-saved" {
		t.Fatalf("reconnect reused/closed wrong stream: oldClosed=%v start=%+v", oldClosed, start2)
	}
	for _, staleID := range []string{start1.SessionID, session.ID} {
		d.ProcessAgentTerminalStarted(conn, agentTerminalMessage(t, agentmgr.TerminalStartedData{SessionID: staleID}))
		d.ProcessAgentTerminalData(conn, agentTerminalMessage(t, agentmgr.TerminalDataPayload{
			SessionID: staleID, Data: base64.StdEncoding.EncodeToString([]byte("stale")),
		}))
		d.ProcessAgentTerminalClosed(conn, agentTerminalMessage(t, agentmgr.TerminalCloseData{SessionID: staleID}))
	}
	d.ProcessAgentTerminalStarted(conn, agentTerminalMessage(t, agentmgr.TerminalStartedData{SessionID: start2.SessionID, TmuxAttached: true}))
	readAgentTerminalReady(t, second)
	d.ProcessAgentTerminalData(conn, agentTerminalMessage(t, agentmgr.TerminalDataPayload{
		SessionID: start2.SessionID, Data: base64.StdEncoding.EncodeToString([]byte("current output")),
	}))
	_ = second.SetReadDeadline(time.Now().Add(2 * time.Second))
	kind, output, err := second.ReadMessage()
	if err != nil || kind != websocket.BinaryMessage || string(output) != "current output" {
		t.Fatalf("replacement received stale output or closure: kind=%d output=%q err=%v", kind, output, err)
	}
	for _, input := range []struct {
		kind int
		data []byte
		want string
	}{
		{websocket.BinaryMessage, []byte("paste 😀\r\n"), agentmgr.MsgTerminalData},
		{websocket.TextMessage, []byte(`{"type":"resize","cols":111,"rows":42}`), agentmgr.MsgTerminalResize},
	} {
		if err := second.WriteMessage(input.kind, input.data); err != nil {
			t.Fatal(err)
		}
		message := readAgentTerminalWire(t, agentPeer)
		var fields struct {
			SessionID string `json:"session_id"`
			Data      string `json:"data"`
			Cols      int    `json:"cols"`
			Rows      int    `json:"rows"`
		}
		if err := json.Unmarshal(message.Data, &fields); err != nil {
			t.Fatal(err)
		}
		if message.Type != input.want || message.ID != start2.SessionID || fields.SessionID != start2.SessionID {
			t.Fatalf("input lost wire identity: %+v", message)
		}
		if input.want == agentmgr.MsgTerminalData {
			data, err := base64.StdEncoding.DecodeString(fields.Data)
			if err != nil || string(data) != string(input.data) {
				t.Fatalf("input bytes changed: %q %v", data, err)
			}
		} else if fields.Cols != 111 || fields.Rows != 42 {
			t.Fatalf("resize changed: %+v", fields)
		}
	}
	if err := d.ClosePersistentTerminalAttachments(terminalmodel.PersistentSession{ID: "saved"}, []terminalmodel.Session{session}); err != nil {
		t.Fatal(err)
	}
	message := readAgentTerminalWire(t, agentPeer)
	var closed agentmgr.TerminalCloseData
	if err := json.Unmarshal(message.Data, &closed); err != nil {
		t.Fatal(err)
	}
	if message.Type != agentmgr.MsgTerminalClose || message.ID != start2.SessionID || closed.SessionID != start2.SessionID {
		t.Fatalf("saved-session deletion lost the current wire ID: %+v", message)
	}
}

type blockedAgentScrollbackStore struct {
	agentScrollbackTestStore
	loadMu  sync.Mutex
	loads   int
	entered chan struct{}
	release chan struct{}
}

func (s *blockedAgentScrollbackStore) GetScrollback(id string) ([]byte, error) {
	s.loadMu.Lock()
	s.loads++
	first := s.loads == 1
	s.loadMu.Unlock()
	if first {
		close(s.entered)
		<-s.release
	}
	return s.agentScrollbackTestStore.GetScrollback(id)
}

func TestAgentTerminalReconnectWhileLoadingScrollbackDoesNotStartOldPTY(t *testing.T) {
	session := terminalmodel.Session{ID: "public", Target: "node-1", PersistentSessionID: "saved", TmuxSessionName: "lt-saved"}
	d, conn, peer, browserURL, finished := newAgentTerminalHarness(t, session)
	store := &blockedAgentScrollbackStore{entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(store.release) })
	d.TerminalInMemStore = terminalmodel.NewStore()
	d.TerminalScrollbackStore = store
	first := dialAgentTerminalTestBrowser(t, browserURL+"?request=old")
	defer first.Close()
	select {
	case <-store.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first stream did not reach saved output load")
	}
	second := dialAgentTerminalTestBrowser(t, browserURL+"?request=new&session=public-new")
	defer second.Close()
	start := readAgentTerminalStart(t, readAgentTerminalWire(t, peer))
	d.ProcessAgentTerminalStarted(conn, agentTerminalMessage(t, agentmgr.TerminalStartedData{SessionID: start.SessionID, TmuxAttached: true}))
	readAgentTerminalReady(t, second)
	release.Do(func() { close(store.release) })
	select {
	case request := <-finished:
		if request != "old" {
			t.Fatalf("replacement ended first: %s", request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("old startup did not stop after replacement")
	}
	// After the old handler has returned, the next wire message must still be
	// input to the current stream, with no abandoned terminal.start or close.
	if err := second.WriteMessage(websocket.BinaryMessage, []byte("still current")); err != nil {
		t.Fatal(err)
	}
	message := readAgentTerminalWire(t, peer)
	if message.Type != agentmgr.MsgTerminalData || message.ID != start.SessionID {
		t.Fatalf("old startup sent a late message: %+v", message)
	}
}

func newAgentTerminalHarness(t *testing.T, session terminalmodel.Session) (*Deps, *agentmgr.AgentConn, *websocket.Conn, string, <-chan string) {
	t.Helper()
	return newAgentTerminalHarnessWithListener(t, session, nil)
}

func newAgentTerminalHarnessWithListener(t *testing.T, session terminalmodel.Session, wrapListener func(net.Listener) net.Listener) (*Deps, *agentmgr.AgentConn, *websocket.Conn, string, <-chan string) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	accepted := make(chan *websocket.Conn, 1)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			accepted <- conn
		}
	}))
	t.Cleanup(agentServer.Close)
	agentPeer := dialAgentTerminalTestBrowser(t, agentServer.URL)
	t.Cleanup(func() { _ = agentPeer.Close() })
	var agentSocket *websocket.Conn
	select {
	case agentSocket = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("agent websocket was not accepted")
	}
	conn := agentmgr.NewAgentConn(agentSocket, "node-1", "linux")
	t.Cleanup(conn.Close)
	conn.SetMeta("terminal.tmux.has", "true")
	manager := agentmgr.NewManager()
	manager.Register(conn)
	var bridges sync.Map
	d := &Deps{AgentMgr: manager, TerminalBridges: &bridges, TerminalWebSocketUpgrader: upgrader}
	finished := make(chan string, 8)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { finished <- r.URL.Query().Get("request") }()
		requestSession := session
		if id := r.URL.Query().Get("session"); id != "" {
			requestSession.ID = id
		}
		d.HandleAgentTerminalStream(w, r, requestSession)
	}))
	if wrapListener != nil {
		server.Listener = wrapListener(server.Listener)
	}
	server.Start()
	t.Cleanup(server.Close)
	t.Cleanup(func() { d.CloseTerminalBridgesForAsset("node-1") })
	return d, conn, agentPeer, server.URL, finished
}

func dialAgentTerminalTestBrowser(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(url, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func readAgentTerminalWire(t *testing.T, conn *websocket.Conn) agentmgr.Message {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var message agentmgr.Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	return message
}

func readAgentTerminalStart(t *testing.T, message agentmgr.Message) agentmgr.TerminalStartData {
	t.Helper()
	var start agentmgr.TerminalStartData
	if message.Type != agentmgr.MsgTerminalStart {
		t.Fatalf("expected start: %+v", message)
	}
	if err := json.Unmarshal(message.Data, &start); err != nil {
		t.Fatal(err)
	}
	if message.ID != start.SessionID {
		t.Fatal("start envelope differs from payload identity")
	}
	return start
}

func readAgentTerminalReady(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	for i := 0; i < 8; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		kind, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var event struct {
			Type string `json:"type"`
		}
		if kind == websocket.TextMessage && json.Unmarshal(data, &event) == nil && event.Type == "ready" {
			return
		}
		if kind == websocket.BinaryMessage {
			t.Fatalf("unexpected pre-ready output: %q", data)
		}
	}
	t.Fatal("terminal was not ready")
}
