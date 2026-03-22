package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
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

func TestProbeAgentTmuxUsesCachedConnectionMetadata(t *testing.T) {
	var srv apiServer
	conn := &agentmgr.AgentConn{AssetID: "node-1"}
	conn.SetMeta("terminal.tmux.has", "true")
	conn.SetMeta("terminal.tmux.path", "/usr/bin/tmux")

	resp := srv.probeAgentTmux(conn)
	if !resp.HasTmux {
		t.Fatal("expected cached tmux capability to be used")
	}
	if resp.TmuxPath != "/usr/bin/tmux" {
		t.Fatalf("unexpected tmux path: %q", resp.TmuxPath)
	}
}

func TestProcessAgentTerminalProbedCachesProbeResultOnConnection(t *testing.T) {
	var srv apiServer
	conn := &agentmgr.AgentConn{AssetID: "node-1"}

	payload, err := json.Marshal(agentmgr.TerminalProbeResponse{
		HasTmux:  true,
		TmuxPath: "/bin/tmux",
	})
	if err != nil {
		t.Fatalf("marshal terminal probe payload: %v", err)
	}

	srv.processAgentTerminalProbed(conn, agentmgr.Message{Data: payload})

	if got := conn.Meta("terminal.tmux.has"); got != "true" {
		t.Fatalf("expected cached tmux flag true, got %q", got)
	}
	if got := conn.Meta("terminal.tmux.path"); got != "/bin/tmux" {
		t.Fatalf("expected cached tmux path /bin/tmux, got %q", got)
	}
	if got := conn.Meta("terminal.tmux.probe_pending"); got != "false" {
		t.Fatalf("expected probe pending flag reset to false, got %q", got)
	}
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

func TestStartAgentTmuxProbeAsyncResetsPendingOnSendFailure(t *testing.T) {
	var srv apiServer
	conn := &agentmgr.AgentConn{AssetID: "node-1"}

	if !srv.startAgentTmuxProbeAsync(conn) {
		t.Fatal("expected probe dispatch to be accepted")
	}
	if got := conn.Meta("terminal.tmux.probe_pending"); got != "true" {
		t.Fatalf("expected probe pending flag true immediately, got %q", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if conn.Meta("terminal.tmux.probe_pending") == "false" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected probe pending flag to reset after async send failure")
}

func TestDialSSHWithRetryRetriesOnceThenSucceeds(t *testing.T) {
	baseConfig := &ssh.ClientConfig{
		User:            "tester",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	seenTimeouts := make([]time.Duration, 0, 2)
	seenAttempts := make([]int, 0, 2)
	sleepCalls := 0
	dialCalls := 0

	client, attemptsUsed, err := dialSSHWithRetry(
		"127.0.0.1:22",
		baseConfig,
		3*time.Second,
		2,
		150*time.Millisecond,
		func(_ string, _ string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
			dialCalls++
			seenTimeouts = append(seenTimeouts, cfg.Timeout)
			if dialCalls == 1 {
				return nil, errors.New("dial timeout")
			}
			return &ssh.Client{}, nil
		},
		func(delay time.Duration) {
			sleepCalls++
			if delay != 150*time.Millisecond {
				t.Fatalf("unexpected retry delay: %s", delay)
			}
		},
		func(attempt, _ int) {
			seenAttempts = append(seenAttempts, attempt)
		},
	)
	if err != nil {
		t.Fatalf("expected retry dial success, got error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client on successful retry")
	}
	if attemptsUsed != 2 {
		t.Fatalf("expected success on attempt 2, got %d", attemptsUsed)
	}
	if dialCalls != 2 {
		t.Fatalf("expected 2 dial calls, got %d", dialCalls)
	}
	if sleepCalls != 1 {
		t.Fatalf("expected one retry sleep, got %d", sleepCalls)
	}
	if len(seenAttempts) != 2 || seenAttempts[0] != 1 || seenAttempts[1] != 2 {
		t.Fatalf("unexpected attempt callback sequence: %v", seenAttempts)
	}
	if len(seenTimeouts) != 2 || seenTimeouts[0] != 3*time.Second || seenTimeouts[1] != 3*time.Second {
		t.Fatalf("unexpected timeout values: %v", seenTimeouts)
	}
	if baseConfig.Timeout != 0 {
		t.Fatalf("expected base config timeout to remain unchanged, got %s", baseConfig.Timeout)
	}
}

func TestDialSSHWithRetryReturnsLastErrorAfterMaxAttempts(t *testing.T) {
	baseConfig := &ssh.ClientConfig{
		User:            "tester",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	sleepCalls := 0
	dialCalls := 0

	client, attemptsUsed, err := dialSSHWithRetry(
		"127.0.0.1:22",
		baseConfig,
		2*time.Second,
		2,
		100*time.Millisecond,
		func(_ string, _ string, _ *ssh.ClientConfig) (*ssh.Client, error) {
			dialCalls++
			return nil, errors.New("connection refused")
		},
		func(_ time.Duration) {
			sleepCalls++
		},
		nil,
	)
	if err == nil {
		t.Fatal("expected retry dial to fail")
	}
	if client != nil {
		t.Fatal("expected nil client on failure")
	}
	if attemptsUsed != 2 {
		t.Fatalf("expected max attempts used to be 2, got %d", attemptsUsed)
	}
	if dialCalls != 2 {
		t.Fatalf("expected 2 dial calls, got %d", dialCalls)
	}
	if sleepCalls != 1 {
		t.Fatalf("expected one retry sleep, got %d", sleepCalls)
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
