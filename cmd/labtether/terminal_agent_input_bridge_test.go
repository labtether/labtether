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

func TestBridgeAgentInputIdleNoPanicAndClosesOnSessionEnd(t *testing.T) {
	var srv apiServer

	serverConn, clientConn, cleanup := newTerminalBridgeWebSocketPair(t)
	defer cleanup()

	closedCh := make(chan struct{})
	done := make(chan struct{})
	panicCh := make(chan any, 1)
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				panicCh <- r
			}
		}()
		srv.bridgeAgentInput(serverConn, nil, "sess-idle", closedCh, nil)
	}()

	// Keep the bridge idle long enough to cover the previous timeout/panic path.
	select {
	case <-done:
		t.Fatal("bridgeAgentInput returned unexpectedly during idle period")
	case <-time.After(2500 * time.Millisecond):
	}

	select {
	case r := <-panicCh:
		t.Fatalf("bridgeAgentInput panicked while idle: %v", r)
	default:
	}

	close(closedCh)
	_ = clientConn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bridgeAgentInput did not exit after session close")
	}
}

func TestBridgeAgentInputForwardsResizeAndInputToAgent(t *testing.T) {
	var srv apiServer

	browserServerConn, browserClientConn, browserCleanup := newTerminalBridgeWebSocketPair(t)
	defer browserCleanup()

	agentServerConn, agentClientConn, agentCleanup := newWebSocketPair(t)
	defer agentCleanup()

	agentConn := agentmgr.NewAgentConn(agentServerConn, "node-1", "linux")
	closedCh := make(chan struct{})
	bridge := &terminalBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        closedCh,
		ExpectedAgentID: "node-1",
	}

	resultCh := make(chan struct {
		reason string
		err    error
	}, 1)
	go func() {
		reason, err := srv.bridgeAgentInput(browserServerConn, agentConn, "sess-forward", closedCh, bridge)
		resultCh <- struct {
			reason string
			err    error
		}{reason: reason, err: err}
	}()

	if err := browserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":132,"rows":51}`)); err != nil {
		t.Fatalf("failed to write resize control message: %v", err)
	}

	_ = agentClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resizeMsg agentmgr.Message
	if err := agentClientConn.ReadJSON(&resizeMsg); err != nil {
		t.Fatalf("failed to read resize message sent to agent: %v", err)
	}
	if resizeMsg.Type != agentmgr.MsgTerminalResize {
		t.Fatalf("message type=%q, want %q", resizeMsg.Type, agentmgr.MsgTerminalResize)
	}
	var resize agentmgr.TerminalResizeData
	if err := json.Unmarshal(resizeMsg.Data, &resize); err != nil {
		t.Fatalf("decode terminal resize payload: %v", err)
	}
	if resize.SessionID != "sess-forward" || resize.Cols != 132 || resize.Rows != 51 {
		t.Fatalf("unexpected resize payload: %+v", resize)
	}

	if err := browserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"input","data":"pwd\n"}`)); err != nil {
		t.Fatalf("failed to write input control message: %v", err)
	}

	_ = agentClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var inputMsg agentmgr.Message
	if err := agentClientConn.ReadJSON(&inputMsg); err != nil {
		t.Fatalf("failed to read input message sent to agent: %v", err)
	}
	if inputMsg.Type != agentmgr.MsgTerminalData {
		t.Fatalf("message type=%q, want %q", inputMsg.Type, agentmgr.MsgTerminalData)
	}
	var input agentmgr.TerminalDataPayload
	if err := json.Unmarshal(inputMsg.Data, &input); err != nil {
		t.Fatalf("decode terminal data payload: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil {
		t.Fatalf("decode terminal data chunk: %v", err)
	}
	if input.SessionID != "sess-forward" || string(decoded) != "pwd\n" {
		t.Fatalf("unexpected terminal input payload session=%q data=%q", input.SessionID, string(decoded))
	}

	if err := browserClientConn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	); err != nil {
		t.Fatalf("failed to write websocket close frame: %v", err)
	}
	_ = browserClientConn.Close()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("bridgeAgentInput returned unexpected error: %v", result.err)
		}
		if result.reason != "browser_ws_closed_normal" {
			t.Fatalf("endReason=%q, want browser_ws_closed_normal", result.reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for bridgeAgentInput to exit")
	}
}

func TestBridgeAgentInputReturnsAgentDisconnectReason(t *testing.T) {
	var srv apiServer

	serverConn, clientConn, cleanup := newTerminalBridgeWebSocketPair(t)
	defer cleanup()

	bridge := &terminalBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
	}

	resultCh := make(chan struct {
		reason string
		err    error
	}, 1)
	go func() {
		reason, err := srv.bridgeAgentInput(serverConn, nil, "sess-disconnect", bridge.ClosedCh, bridge)
		resultCh <- struct {
			reason string
			err    error
		}{reason: reason, err: err}
	}()

	bridge.CloseWithReason("agent disconnected")

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("bridgeAgentInput returned unexpected error: %v", result.err)
		}
		if result.reason != "agent_stream_closed_agent_disconnected" {
			t.Fatalf("endReason=%q, want agent_stream_closed_agent_disconnected", result.reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for bridgeAgentInput to exit on agent disconnect")
	}

	_ = clientConn.Close()
}

func TestHandleAgentTerminalStreamAllowsFreshStreamAfterPriorDisconnect(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	agentServerConn, agentClientConn, agentCleanup := newWebSocketPair(t)
	defer agentCleanup()

	agentConn := agentmgr.NewAgentConn(agentServerConn, "node-1", "linux")
	agentConn.SetMeta("terminal.tmux.has", "false")
	sut.agentMgr.Register(agentConn)
	defer sut.agentMgr.Unregister("node-1")

	session := terminal.Session{ID: "sess-reconnect", Target: "node-1"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handleAgentTerminalStream(w, r, session)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?cols=132&rows=51"

	dialBrowser := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial browser websocket failed: %v", err)
		}
		return conn
	}

	sendStarted := func() {
		t.Helper()
		startedData, err := json.Marshal(agentmgr.TerminalStartedData{SessionID: session.ID})
		if err != nil {
			t.Fatalf("marshal terminal started payload: %v", err)
		}
		sut.processAgentTerminalStarted(agentConn, agentmgr.Message{Data: startedData})
	}

	sendOutput := func(data string) {
		t.Helper()
		payload, err := json.Marshal(agentmgr.TerminalDataPayload{
			SessionID: session.ID,
			Data:      base64.StdEncoding.EncodeToString([]byte(data)),
		})
		if err != nil {
			t.Fatalf("marshal terminal data payload: %v", err)
		}
		sut.processAgentTerminalData(agentConn, agentmgr.Message{Data: payload})
	}

	browserConn1 := dialBrowser()
	startMsg1 := waitForAgentTerminalMessage(t, agentClientConn, agentmgr.MsgTerminalStart)
	start1 := decodeTerminalStartData(t, startMsg1)
	if start1.SessionID != session.ID {
		t.Fatalf("first start session_id=%q, want %q", start1.SessionID, session.ID)
	}
	if start1.Cols != 132 || start1.Rows != 51 {
		t.Fatalf("first start size=%dx%d, want 132x51", start1.Cols, start1.Rows)
	}

	sendStarted()
	waitForTerminalReadyEvent(t, browserConn1)

	sendOutput("first-stream\n")
	if got := string(waitForTerminalBinaryPayload(t, browserConn1)); got != "first-stream\n" {
		t.Fatalf("first browser payload=%q, want %q", got, "first-stream\n")
	}

	closeBrowserTerminalStream(t, browserConn1)
	closeMsg1 := waitForAgentTerminalMessage(t, agentClientConn, agentmgr.MsgTerminalClose)
	close1 := decodeTerminalCloseData(t, closeMsg1)
	if close1.SessionID != session.ID {
		t.Fatalf("first close session_id=%q, want %q", close1.SessionID, session.ID)
	}
	waitForTerminalBridgeCleanup(t, sut, session.ID)

	browserConn2 := dialBrowser()
	startMsg2 := waitForAgentTerminalMessage(t, agentClientConn, agentmgr.MsgTerminalStart)
	start2 := decodeTerminalStartData(t, startMsg2)
	if start2.SessionID != session.ID {
		t.Fatalf("second start session_id=%q, want %q", start2.SessionID, session.ID)
	}
	if start2.Cols != 132 || start2.Rows != 51 {
		t.Fatalf("second start size=%dx%d, want 132x51", start2.Cols, start2.Rows)
	}

	sendStarted()
	waitForTerminalReadyEvent(t, browserConn2)

	sendOutput("second-stream\n")
	if got := string(waitForTerminalBinaryPayload(t, browserConn2)); got != "second-stream\n" {
		t.Fatalf("second browser payload=%q, want %q", got, "second-stream\n")
	}

	closeBrowserTerminalStream(t, browserConn2)
	closeMsg2 := waitForAgentTerminalMessage(t, agentClientConn, agentmgr.MsgTerminalClose)
	close2 := decodeTerminalCloseData(t, closeMsg2)
	if close2.SessionID != session.ID {
		t.Fatalf("second close session_id=%q, want %q", close2.SessionID, session.ID)
	}
	waitForTerminalBridgeCleanup(t, sut, session.ID)
}
