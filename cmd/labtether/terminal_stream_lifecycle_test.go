package main

import (
	"encoding/base64"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCloseTerminalBridgesForAssetClosesMatchingSessionsOnly(t *testing.T) {
	var srv apiServer
	matching := &terminalBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
	}
	nonMatching := &terminalBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-2",
	}
	probeCh := make(chan agentmgr.TerminalProbeResponse, 1)

	srv.terminalBridges.Store("sess-node-1", matching)
	srv.terminalBridges.Store("sess-node-2", nonMatching)
	srv.terminalBridges.Store("probe:node-1", probeCh)

	srv.closeTerminalBridgesForAsset("node-1")

	select {
	case <-matching.ClosedCh:
	default:
		t.Fatal("expected matching bridge to be closed")
	}

	select {
	case <-nonMatching.ClosedCh:
		t.Fatal("expected non-matching bridge to remain open")
	default:
	}

	// Probe channels share the same map and should not be closed by this sweep.
	select {
	case probeCh <- agentmgr.TerminalProbeResponse{}:
	default:
		t.Fatal("expected probe channel to remain open")
	}
}

func TestFinalizeAgentTerminalSessionSendsCloseAfterStart(t *testing.T) {
	var srv apiServer
	bridge := &terminalBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	srv.terminalBridges.Store("sess-finalize", bridge)

	closeCalls := 0
	closedSessionID := ""
	srv.finalizeAgentTerminalSession(
		"sess-finalize",
		bridge,
		nil,
		true,
		func(_ *agentmgr.AgentConn, sessionID string) {
			closeCalls++
			closedSessionID = sessionID
		},
	)

	if closeCalls != 1 {
		t.Fatalf("expected one terminal.close send, got %d", closeCalls)
	}
	if closedSessionID != "sess-finalize" {
		t.Fatalf("unexpected session id for close send: %q", closedSessionID)
	}
	if _, ok := srv.terminalBridges.Load("sess-finalize"); ok {
		t.Fatal("expected terminal bridge to be removed")
	}
	select {
	case <-bridge.ClosedCh:
	default:
		t.Fatal("expected bridge to be closed during finalize")
	}
}

func TestFinalizeAgentTerminalSessionSkipsCloseBeforeStart(t *testing.T) {
	var srv apiServer
	bridge := &terminalBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}

	closeCalls := 0
	srv.finalizeAgentTerminalSession(
		"sess-no-start",
		bridge,
		nil,
		false,
		func(_ *agentmgr.AgentConn, _ string) {
			closeCalls++
		},
	)

	if closeCalls != 0 {
		t.Fatalf("expected no terminal.close send before start, got %d", closeCalls)
	}
}

func TestProcessAgentTerminalHandlersIgnoreNonBridgeEntries(t *testing.T) {
	var srv apiServer
	probeCh := make(chan agentmgr.TerminalProbeResponse, 1)
	srv.terminalBridges.Store("sess-probe", probeCh)

	startedData, err := json.Marshal(agentmgr.TerminalStartedData{SessionID: "sess-probe"})
	if err != nil {
		t.Fatalf("marshal terminal started payload: %v", err)
	}
	dataPayload, err := json.Marshal(agentmgr.TerminalDataPayload{
		SessionID: "sess-probe",
		Data:      base64.StdEncoding.EncodeToString([]byte("hello")),
	})
	if err != nil {
		t.Fatalf("marshal terminal data payload: %v", err)
	}
	closedData, err := json.Marshal(agentmgr.TerminalCloseData{SessionID: "sess-probe"})
	if err != nil {
		t.Fatalf("marshal terminal close payload: %v", err)
	}

	assertDoesNotPanic(t, func() {
		srv.processAgentTerminalStarted(nil, agentmgr.Message{Data: startedData})
	})
	assertDoesNotPanic(t, func() {
		srv.processAgentTerminalData(nil, agentmgr.Message{Data: dataPayload})
	})
	assertDoesNotPanic(t, func() {
		srv.processAgentTerminalClosed(nil, agentmgr.Message{Data: closedData})
	})

	// Ensure the probe channel entry remains valid after all handlers run.
	select {
	case probeCh <- agentmgr.TerminalProbeResponse{}:
	default:
		t.Fatal("expected probe channel entry to remain open")
	}
}

func TestProcessAgentTerminalHandlersIgnoreMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &terminalBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
	}
	srv.terminalBridges.Store("sess-mismatch", bridge)

	startedData, err := json.Marshal(agentmgr.TerminalStartedData{SessionID: "sess-mismatch"})
	if err != nil {
		t.Fatalf("marshal terminal started payload: %v", err)
	}
	dataPayload, err := json.Marshal(agentmgr.TerminalDataPayload{
		SessionID: "sess-mismatch",
		Data:      base64.StdEncoding.EncodeToString([]byte("hello")),
	})
	if err != nil {
		t.Fatalf("marshal terminal data payload: %v", err)
	}
	closedData, err := json.Marshal(agentmgr.TerminalCloseData{SessionID: "sess-mismatch"})
	if err != nil {
		t.Fatalf("marshal terminal close payload: %v", err)
	}

	conn := &agentmgr.AgentConn{AssetID: "node-2"}
	srv.processAgentTerminalStarted(conn, agentmgr.Message{Data: startedData})
	srv.processAgentTerminalData(conn, agentmgr.Message{Data: dataPayload})
	srv.processAgentTerminalClosed(conn, agentmgr.Message{Data: closedData})

	select {
	case <-bridge.OutputCh:
		t.Fatal("expected no terminal output from mismatched sender")
	default:
	}
	select {
	case <-bridge.ClosedCh:
		t.Fatal("expected bridge to remain open for mismatched sender")
	default:
	}
}

func TestProcessAgentTerminalStartedRejectsMissingRequiredTmux(t *testing.T) {
	var srv apiServer
	bridge := &terminalBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
		SessionID:       "sess-persistent",
		Target:          "node-1",
		RequireTmux:     true,
	}
	srv.terminalBridges.Store("sess-persistent", bridge)

	conn := &agentmgr.AgentConn{AssetID: "node-1"}
	conn.SetMeta("terminal.tmux.has", "true")
	startedData, err := json.Marshal(agentmgr.TerminalStartedData{
		SessionID:    "sess-persistent",
		TmuxAttached: false,
	})
	if err != nil {
		t.Fatalf("marshal terminal started payload: %v", err)
	}

	srv.processAgentTerminalStarted(conn, agentmgr.Message{Data: startedData})

	select {
	case <-bridge.ClosedCh:
	default:
		t.Fatal("expected persistent bridge to close when the agent did not attach tmux")
	}
	if got := bridge.CloseReasonOr(""); got != "tmux_unavailable" {
		t.Fatalf("close reason=%q, want tmux_unavailable", got)
	}
	if got := conn.Meta("terminal.tmux.has"); got != "false" {
		t.Fatalf("cached tmux capability=%q, want false", got)
	}
	select {
	case <-bridge.OutputCh:
		t.Fatal("expected no ready marker after non-tmux shell startup")
	default:
	}
}

func TestHandleAgentTerminalStreamRejectsPersistentSessionWhenTmuxUnavailable(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	agentServerConn, agentClientConn, agentCleanup := newWebSocketPair(t)
	defer agentCleanup()
	agentConn := agentmgr.NewAgentConn(agentServerConn, "node-1", "linux")
	agentConn.SetMeta("terminal.tmux.has", "false")
	sut.agentMgr.Register(agentConn)
	defer sut.agentMgr.Unregister("node-1")
	sut.resetTerminalDepsForTest()

	session := terminal.Session{
		ID:                  "sess-persistent",
		Target:              "node-1",
		PersistentSessionID: "persistent-1",
	}
	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess-persistent/stream", nil)
	rec := httptest.NewRecorder()
	sut.handleAgentTerminalStream(rec, req, session)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 streaming a persistent shell without tmux, got %d: %s", rec.Code, rec.Body.String())
	}
	_ = waitForAgentTerminalMessage(t, agentClientConn, agentmgr.MsgTerminalProbe)
}

func TestSanitizeAgentStreamReason(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "   ", want: "unknown"},
		{name: "keeps safe", in: "agent_disconnected", want: "agent_disconnected"},
		{name: "normalizes punctuation and case", in: " Agent Closed / Timeout ", want: "agent_closed_timeout"},
		{name: "trims trailing separators", in: "***", want: "unknown"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeAgentStreamReason(tc.in); got != tc.want {
				t.Fatalf("sanitizeAgentStreamReason(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func newTerminalBridgeWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	serverConnCh := make(chan *websocket.Conn, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConnCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial websocket failed: %v", err)
	}

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(2 * time.Second):
		_ = clientConn.Close()
		server.Close()
		t.Fatal("timed out waiting for server websocket connection")
	}

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		server.Close()
	}

	return serverConn, clientConn, cleanup
}

func waitForAgentTerminalMessage(t *testing.T, conn *websocket.Conn, wantType string) agentmgr.Message {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read agent websocket message: %v", err)
	}
	if msg.Type != wantType {
		t.Fatalf("agent message type=%q, want %q", msg.Type, wantType)
	}
	return msg
}

func decodeTerminalStartData(t *testing.T, msg agentmgr.Message) agentmgr.TerminalStartData {
	t.Helper()

	var start agentmgr.TerminalStartData
	if err := json.Unmarshal(msg.Data, &start); err != nil {
		t.Fatalf("decode terminal start payload: %v", err)
	}
	return start
}

func decodeTerminalCloseData(t *testing.T, msg agentmgr.Message) agentmgr.TerminalCloseData {
	t.Helper()

	var closeData agentmgr.TerminalCloseData
	if err := json.Unmarshal(msg.Data, &closeData); err != nil {
		t.Fatalf("decode terminal close payload: %v", err)
	}
	return closeData
}

func waitForTerminalReadyEvent(t *testing.T, conn *websocket.Conn) terminalStreamEvent {
	t.Helper()

	for i := 0; i < 6; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read browser websocket message: %v", err)
		}
		if messageType != websocket.TextMessage {
			continue
		}
		var event terminalStreamEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatalf("decode browser event payload: %v", err)
		}
		if event.Type == "ready" {
			return event
		}
	}

	t.Fatal("timed out waiting for terminal ready event")
	return terminalStreamEvent{}
}

func waitForTerminalBinaryPayload(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()

	for i := 0; i < 6; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read browser websocket payload: %v", err)
		}
		if messageType == websocket.BinaryMessage {
			return payload
		}
	}

	t.Fatal("timed out waiting for terminal binary payload")
	return nil
}

func closeBrowserTerminalStream(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	if err := conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	); err != nil {
		t.Fatalf("write browser websocket close frame: %v", err)
	}
	_ = conn.Close()
}

func waitForTerminalBridgeCleanup(t *testing.T, srv *apiServer, sessionID string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := srv.terminalBridges.Load(sessionID); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for terminal bridge cleanup for session %s", sessionID)
}

func assertDoesNotPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	fn()
}
