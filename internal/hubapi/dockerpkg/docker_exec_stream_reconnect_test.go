package dockerpkg

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

func TestDockerExecReconnectRejectsLateFramesAndOldFinalizer(t *testing.T) {
	d, conn, peer, url, finished := newDockerExecHarness(t)
	first := dialDockerExecBrowser(t, url+"?request=old")
	oldStart := readDockerExecStart(t, peer)
	if oldStart.Type != agentmgr.MsgDockerExecStart || oldStart.ID == "public-session" {
		t.Fatalf("expected a private stream start, got %+v", oldStart)
	}
	d.ProcessAgentDockerExecStarted(conn, dockerExecMessage(t, agentmgr.DockerExecStartedData{SessionID: oldStart.ID}))
	second := dialDockerExecBrowser(t, url+"?request=new")
	var newStart agentmgr.Message
	for i := 0; i < 2; i++ {
		message := readDockerExecWire(t, peer)
		switch message.Type {
		case agentmgr.MsgDockerExecStart:
			assertDockerExecStart(t, message)
			newStart = message
		case agentmgr.MsgDockerExecClose:
			if message.ID != oldStart.ID {
				t.Fatalf("old finalizer closed a different exec: %+v", message)
			}
		default:
			t.Fatalf("unexpected reconnect message: %+v", message)
		}
	}
	if newStart.ID == "" || newStart.ID == oldStart.ID || newStart.ID == "public-session" {
		t.Fatalf("reconnect did not get its own stream ID: %q", newStart.ID)
	}
	waitDockerExecFinished(t, finished, "old")
	if _, exists := d.DockerExecBridges.Load(oldStart.ID); exists {
		t.Fatal("old wire dispatch survived finalization")
	}
	current, exists := d.DockerExecBridges.Load(newStart.ID)
	if !exists {
		t.Fatal("old finalizer deleted the new stream")
	}
	if owner, _ := d.dockerExecOwners.Load("public-session"); owner != current {
		t.Fatal("old finalizer removed public ownership")
	}
	bridge := current.(*DockerExecBridge)
	for _, staleID := range []string{oldStart.ID, "public-session"} {
		d.ProcessAgentDockerExecStarted(conn, dockerExecMessage(t, agentmgr.DockerExecStartedData{SessionID: staleID}))
		d.ProcessAgentDockerExecData(conn, dockerExecMessage(t, agentmgr.DockerExecDataPayload{SessionID: staleID, Data: "b2xk"}))
		d.ProcessAgentDockerExecClosed(conn, dockerExecMessage(t, agentmgr.DockerExecCloseData{SessionID: staleID, Reason: "late close"}))
	}
	assertDockerExecOpenAndQuiet(t, bridge)
	d.ProcessAgentDockerExecStarted(conn, dockerExecMessage(t, agentmgr.DockerExecStartedData{SessionID: newStart.ID}))
	d.ProcessAgentDockerExecData(conn, dockerExecMessage(t, agentmgr.DockerExecDataPayload{
		SessionID: newStart.ID, Data: base64.StdEncoding.EncodeToString([]byte("new shell")),
	}))
	readDockerExecOutput(t, second, "new shell")
	if err := second.WriteMessage(websocket.BinaryMessage, []byte("still current")); err != nil {
		t.Fatal(err)
	}
	input := readDockerExecWire(t, peer)
	if input.Type != agentmgr.MsgDockerExecInput || input.ID != newStart.ID {
		t.Fatalf("new browser lost its exec after old cleanup: %+v", input)
	}
	var inputData agentmgr.DockerExecInputData
	if err := json.Unmarshal(input.Data, &inputData); err != nil || inputData.SessionID != newStart.ID {
		t.Fatalf("input payload used the wrong stream: %+v err=%v", inputData, err)
	}
	if err := second.WriteJSON(ControlMessage{Type: "resize", Cols: 100, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	resize := readDockerExecWire(t, peer)
	var resizeData agentmgr.DockerExecResizeData
	if err := json.Unmarshal(resize.Data, &resizeData); err != nil || resize.Type != agentmgr.MsgDockerExecResize || resize.ID != newStart.ID || resizeData.SessionID != newStart.ID {
		t.Fatalf("resize did not target the current wire stream: %+v err=%v", resize, err)
	}
	_ = first.Close()
	_ = second.Close()
	waitDockerExecFinished(t, finished, "new")
}

func TestDockerExecDisconnectAndFramesRequireExactConnection(t *testing.T) {
	var bridges sync.Map
	d := &Deps{DockerExecBridges: &bridges}
	oldConn := &agentmgr.AgentConn{AssetID: "node-1"}
	newConn := &agentmgr.AgentConn{AssetID: "node-1"}
	old := dockerExecTestBridge("old", oldConn)
	current := dockerExecTestBridge("new", newConn)
	d.registerDockerExecStream(old)
	d.registerDockerExecStream(current)
	d.AgentMgr = agentmgr.NewManager()
	d.AgentMgr.Register(newConn)
	stale := dockerExecTestBridge("new", oldConn)
	stale.WireSessionID = "wire-stale-registration"
	d.registerDockerExecStream(stale)
	if owner, _ := d.dockerExecOwners.Load("new"); owner != current {
		t.Fatal("late registration from an old agent evicted the current owner")
	}
	if _, exists := d.DockerExecBridges.Load(stale.WireSessionID); exists {
		t.Fatal("late registration from an old agent gained wire dispatch")
	}
	d.CloseDockerExecBridgesForConnection(oldConn)
	select {
	case <-old.ClosedCh:
	default:
		t.Fatal("old connection retained its exec")
	}
	d.ProcessAgentDockerExecStarted(oldConn, dockerExecMessage(t, agentmgr.DockerExecStartedData{SessionID: current.WireSessionID}))
	d.ProcessAgentDockerExecData(oldConn, dockerExecMessage(t, agentmgr.DockerExecDataPayload{SessionID: current.WireSessionID, Data: "b2xk"}))
	d.ProcessAgentDockerExecClosed(oldConn, dockerExecMessage(t, agentmgr.DockerExecCloseData{SessionID: current.WireSessionID}))
	assertDockerExecOpenAndQuiet(t, current)
	d.ProcessAgentDockerExecData(newConn, dockerExecMessage(t, agentmgr.DockerExecDataPayload{SessionID: current.WireSessionID, Data: "bmV3"}))
	select {
	case data := <-current.OutputCh:
		if string(data) != "new" {
			t.Fatalf("wrong connection output: %q", data)
		}
	default:
		t.Fatal("current connection output was rejected")
	}
}

func TestDockerExecReplacementBeforeStartDoesNotLaunchOldExec(t *testing.T) {
	d, conn, peer, url, finished := newDockerExecHarness(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	d.ParseTerminalSize = func(cols, rows string) (int, int) {
		if cols == "hold" {
			close(entered)
			<-release
		}
		return 80, 24
	}
	_ = dialDockerExecBrowser(t, url+"?request=old&cols=hold")
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("old stream did not reach the startup barrier")
	}
	second := dialDockerExecBrowser(t, url+"?request=new")
	start := readDockerExecStart(t, peer)
	if start.Type != agentmgr.MsgDockerExecStart {
		t.Fatalf("expected replacement start: %+v", start)
	}
	d.ProcessAgentDockerExecStarted(conn, dockerExecMessage(t, agentmgr.DockerExecStartedData{SessionID: start.ID}))
	releaseOnce.Do(func() { close(release) })
	waitDockerExecFinished(t, finished, "old")
	if err := second.WriteMessage(websocket.BinaryMessage, []byte("alive")); err != nil {
		t.Fatal(err)
	}
	input := readDockerExecWire(t, peer)
	if input.Type != agentmgr.MsgDockerExecInput || input.ID != start.ID {
		t.Fatalf("superseded startup sent a late start or close: %+v", input)
	}
	_ = second.Close()
	waitDockerExecFinished(t, finished, "new")
}

func dockerExecTestBridge(id string, conn *agentmgr.AgentConn) *DockerExecBridge {
	return &DockerExecBridge{
		SessionID: id, WireSessionID: "wire-" + id, AgentConn: conn, ExpectedAgentID: conn.AssetID,
		OutputCh: make(chan []byte, 4), ClosedCh: make(chan struct{}), ReadyCh: make(chan struct{}),
	}
}

func assertDockerExecOpenAndQuiet(t *testing.T, bridge *DockerExecBridge) {
	t.Helper()
	select {
	case <-bridge.ClosedCh:
		t.Fatal("stale connection or stream closed the replacement")
	default:
	}
	select {
	case <-bridge.ReadyCh:
		t.Fatal("stale started or output frame reached the replacement")
	default:
	}
	select {
	case <-bridge.OutputCh:
		t.Fatal("stale output reached the replacement")
	default:
	}
}

func newDockerExecHarness(t *testing.T) (*Deps, *agentmgr.AgentConn, *websocket.Conn, string, <-chan string) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	accepted := make(chan *websocket.Conn, 1)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ws, err := upgrader.Upgrade(w, r, nil); err == nil {
			accepted <- ws
		}
	}))
	t.Cleanup(agentServer.Close)
	peer := dialDockerExecBrowser(t, agentServer.URL)
	var socket *websocket.Conn
	select {
	case socket = <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("agent websocket was not accepted")
	}
	conn := agentmgr.NewAgentConn(socket, "node-1", "linux")
	t.Cleanup(conn.Close)
	manager := agentmgr.NewManager()
	manager.Register(conn)
	var bridges sync.Map
	d := &Deps{
		AgentMgr: manager, DockerExecBridges: &bridges, TerminalWebSocketUpgrader: upgrader,
		ParseTerminalSize:          func(string, string) (int, int) { return 80, 24 },
		IsControlMessage:           func(data []byte) bool { return len(data) > 0 && data[0] == '{' },
		TouchBrowserWSReadDeadline: func(*websocket.Conn) error { return nil },
		StartBrowserWSKeepalive:    func(*websocket.Conn, *sync.Mutex, string) func() { return func() {} },
	}
	finished := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { finished <- r.URL.Query().Get("request") }()
		d.HandleDockerExecTerminalStream(w, r, terminal.Session{ID: "public-session"}, "node-1", "container-1")
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { d.CloseDockerExecBridgesForConnection(conn) })
	return d, conn, peer, server.URL, finished
}

func dialDockerExecBrowser(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(url, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws
}

func dockerExecMessage(t *testing.T, data any) agentmgr.Message {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return agentmgr.Message{Data: raw}
}

func readDockerExecWire(t *testing.T, ws *websocket.Conn) agentmgr.Message {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var message agentmgr.Message
	if err := ws.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	return message
}

func readDockerExecOutput(t *testing.T, ws *websocket.Conn, want string) {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	kind, data, err := ws.ReadMessage()
	if err != nil || kind != websocket.BinaryMessage || string(data) != want {
		t.Fatalf("output kind=%d data=%q err=%v; want %q", kind, data, err, want)
	}
}

func waitDockerExecFinished(t *testing.T, finished <-chan string, want string) {
	t.Helper()
	select {
	case got := <-finished:
		if got != want {
			t.Fatalf("handler %q ended instead of %q", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("handler %q did not finish", want)
	}
}

func readDockerExecStart(t *testing.T, ws *websocket.Conn) agentmgr.Message {
	t.Helper()
	message := readDockerExecWire(t, ws)
	assertDockerExecStart(t, message)
	return message
}

func assertDockerExecStart(t *testing.T, message agentmgr.Message) {
	t.Helper()
	var data agentmgr.DockerExecStartData
	if err := json.Unmarshal(message.Data, &data); err != nil || message.Type != agentmgr.MsgDockerExecStart || data.SessionID == "" || data.SessionID != message.ID || data.SessionID == "public-session" {
		t.Fatalf("start did not use a distinct wire session: %+v err=%v", message, err)
	}
}
