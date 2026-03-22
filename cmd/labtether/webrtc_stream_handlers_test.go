package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

func TestCloseWebRTCBridgesForAssetClosesMatchingSessionsOnly(t *testing.T) {
	var srv apiServer
	matching := &webrtcSignalingBridge{
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
	}
	nonMatching := &webrtcSignalingBridge{
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-2",
	}

	srv.webrtcBridges.Store("sess-node-1", matching)
	srv.webrtcBridges.Store("sess-node-2", nonMatching)
	srv.webrtcBridges.Store("non-bridge-entry", "ignore-me")

	srv.closeWebRTCBridgesForAsset("node-1")

	select {
	case <-matching.ClosedCh:
	default:
		t.Fatal("expected matching webrtc bridge to be closed")
	}

	select {
	case <-nonMatching.ClosedCh:
		t.Fatal("expected non-matching webrtc bridge to remain open")
	default:
	}
}

func TestProcessAgentWebRTCHandlersIgnoreNonBridgeEntries(t *testing.T) {
	var srv apiServer
	srv.webrtcBridges.Store("sess-non-bridge", "probe-ish-value")

	startedData, err := json.Marshal(agentmgr.WebRTCStartedData{SessionID: "sess-non-bridge"})
	if err != nil {
		t.Fatalf("marshal webrtc.started payload: %v", err)
	}
	answerData, err := json.Marshal(agentmgr.WebRTCSDPData{SessionID: "sess-non-bridge", Type: "answer", SDP: "v=0"})
	if err != nil {
		t.Fatalf("marshal webrtc.answer payload: %v", err)
	}
	iceData, err := json.Marshal(agentmgr.WebRTCICEData{SessionID: "sess-non-bridge", Candidate: "candidate:1 1 UDP 123 10.0.0.10 10000 typ host"})
	if err != nil {
		t.Fatalf("marshal webrtc.ice payload: %v", err)
	}
	stoppedData, err := json.Marshal(agentmgr.WebRTCStoppedData{SessionID: "sess-non-bridge"})
	if err != nil {
		t.Fatalf("marshal webrtc.stopped payload: %v", err)
	}

	assertNoPanicWebRTC(t, func() {
		srv.processAgentWebRTCStarted(nil, agentmgr.Message{Data: startedData})
	})
	assertNoPanicWebRTC(t, func() {
		srv.processAgentWebRTCAnswer(nil, agentmgr.Message{Data: answerData})
	})
	assertNoPanicWebRTC(t, func() {
		srv.processAgentWebRTCICE(nil, agentmgr.Message{Data: iceData})
	})
	assertNoPanicWebRTC(t, func() {
		srv.processAgentWebRTCStopped(nil, agentmgr.Message{Data: stoppedData})
	})
}

func TestProcessAgentWebRTCStoppedSkipsMismatchedAgent(t *testing.T) {
	var srv apiServer
	bridge := &webrtcSignalingBridge{
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
	}
	srv.webrtcBridges.Store("sess-mismatch", bridge)

	stoppedData, err := json.Marshal(agentmgr.WebRTCStoppedData{SessionID: "sess-mismatch", Reason: "agent stopped"})
	if err != nil {
		t.Fatalf("marshal webrtc.stopped payload: %v", err)
	}

	srv.processAgentWebRTCStopped(&agentmgr.AgentConn{AssetID: "node-2"}, agentmgr.Message{Data: stoppedData})

	select {
	case <-bridge.ClosedCh:
		t.Fatal("expected bridge to remain open for mismatched agent event")
	default:
	}
}

func TestProcessAgentWebRTCCapabilitiesStoresUnavailableReason(t *testing.T) {
	var srv apiServer
	conn := &agentmgr.AgentConn{AssetID: "node-1"}

	data, err := json.Marshal(agentmgr.WebRTCCapabilitiesData{
		Available:         false,
		UnavailableReason: "gst_launch_not_found",
	})
	if err != nil {
		t.Fatalf("marshal webrtc capabilities payload: %v", err)
	}

	srv.processAgentWebRTCCapabilities(conn, agentmgr.Message{Data: data})

	if got := conn.Meta("webrtc_available"); got != "false" {
		t.Fatalf("expected webrtc_available=false, got %q", got)
	}
	if got := conn.Meta("webrtc_unavailable_reason"); got != "gst_launch_not_found" {
		t.Fatalf("expected webrtc_unavailable_reason to be stored, got %q", got)
	}
}

func TestHandleWebRTCStreamForwardsBrowserSignalsAndSendsStop(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	agentServerConn, agentClientConn, agentCleanup := createWSPairForNetworkTest(t)
	defer agentCleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(agentServerConn, "node-1", "linux"))
	defer sut.agentMgr.Unregister("node-1")

	session := terminal.Session{ID: "sess-webrtc", Target: "node-1"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handleWebRTCStream(w, r, session)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?width=1280&height=720&fps=60&display=%3A1&quality=high&audio=false"
	browserConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("browser dial failed: %v", err)
	}
	defer browserConn.Close()

	startMsg := readWebRTCAgentMessage(t, agentClientConn)
	if startMsg.Type != agentmgr.MsgWebRTCStart {
		t.Fatalf("start type=%q, want %q", startMsg.Type, agentmgr.MsgWebRTCStart)
	}
	var start agentmgr.WebRTCSessionData
	if err := json.Unmarshal(startMsg.Data, &start); err != nil {
		t.Fatalf("decode start payload: %v", err)
	}
	if start.SessionID != session.ID || start.Display != ":1" {
		t.Fatalf("unexpected start payload %+v", start)
	}
	if start.Quality != "high" {
		t.Fatalf("quality=%q, want high", start.Quality)
	}
	if start.Width != 1280 || start.Height != 720 || start.FPS != 60 {
		t.Fatalf("unexpected dimensions in %+v", start)
	}
	if start.AudioEnabled {
		t.Fatalf("expected audio to be disabled in %+v", start)
	}

	if err := browserConn.WriteJSON(map[string]any{
		"type": "offer",
		"data": map[string]any{
			"type": "offer",
			"sdp":  "v=0",
		},
	}); err != nil {
		t.Fatalf("write offer: %v", err)
	}
	offerMsg := readWebRTCAgentMessage(t, agentClientConn)
	if offerMsg.Type != agentmgr.MsgWebRTCOffer {
		t.Fatalf("offer type=%q, want %q", offerMsg.Type, agentmgr.MsgWebRTCOffer)
	}
	var offer agentmgr.WebRTCSDPData
	if err := json.Unmarshal(offerMsg.Data, &offer); err != nil {
		t.Fatalf("decode offer payload: %v", err)
	}
	if offer.SessionID != session.ID || offer.Type != "offer" || offer.SDP != "v=0" {
		t.Fatalf("unexpected offer payload %+v", offer)
	}

	if err := browserConn.WriteJSON(map[string]any{
		"type": "ice",
		"data": map[string]any{
			"candidate":       "candidate:1 1 UDP 123 10.0.0.10 10000 typ host",
			"sdp_mid":         "0",
			"sdp_mline_index": 0,
		},
	}); err != nil {
		t.Fatalf("write ice: %v", err)
	}
	iceMsg := readWebRTCAgentMessage(t, agentClientConn)
	if iceMsg.Type != agentmgr.MsgWebRTCICE {
		t.Fatalf("ice type=%q, want %q", iceMsg.Type, agentmgr.MsgWebRTCICE)
	}
	var ice agentmgr.WebRTCICEData
	if err := json.Unmarshal(iceMsg.Data, &ice); err != nil {
		t.Fatalf("decode ice payload: %v", err)
	}
	if ice.SessionID != session.ID || ice.Candidate == "" || ice.SDPMid != "0" {
		t.Fatalf("unexpected ice payload %+v", ice)
	}
	if ice.SDPMLineIndex == nil || *ice.SDPMLineIndex != 0 {
		t.Fatalf("unexpected ice m-line index %+v", ice.SDPMLineIndex)
	}

	if err := browserConn.WriteJSON(map[string]any{
		"type": "input",
		"data": map[string]any{
			"type": "mousemove",
			"x":    44,
			"y":    55,
		},
	}); err != nil {
		t.Fatalf("write input: %v", err)
	}
	inputMsg := readWebRTCAgentMessage(t, agentClientConn)
	if inputMsg.Type != agentmgr.MsgWebRTCInput {
		t.Fatalf("input type=%q, want %q", inputMsg.Type, agentmgr.MsgWebRTCInput)
	}
	var input agentmgr.WebRTCInputData
	if err := json.Unmarshal(inputMsg.Data, &input); err != nil {
		t.Fatalf("decode input payload: %v", err)
	}
	if input.SessionID != session.ID || input.Type != "mousemove" || input.X != 44 || input.Y != 55 {
		t.Fatalf("unexpected input payload %+v", input)
	}

	if err := browserConn.WriteJSON(map[string]any{"type": "stop"}); err != nil {
		t.Fatalf("write stop: %v", err)
	}
	stopMsg := readWebRTCAgentMessage(t, agentClientConn)
	if stopMsg.Type != agentmgr.MsgWebRTCStop {
		t.Fatalf("stop type=%q, want %q", stopMsg.Type, agentmgr.MsgWebRTCStop)
	}
	var stopped agentmgr.WebRTCStoppedData
	if err := json.Unmarshal(stopMsg.Data, &stopped); err != nil {
		t.Fatalf("decode stop payload: %v", err)
	}
	if stopped.SessionID != session.ID {
		t.Fatalf("unexpected stop payload %+v", stopped)
	}

	waitForWebRTCBridgeRemoval(t, sut, session.ID)
}

func TestHandleWebRTCStreamIgnoresMalformedAndUnknownBrowserSignals(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	agentServerConn, agentClientConn, agentCleanup := createWSPairForNetworkTest(t)
	defer agentCleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(agentServerConn, "node-1", "linux"))
	defer sut.agentMgr.Unregister("node-1")

	session := terminal.Session{ID: "sess-webrtc-ignore", Target: "node-1"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handleWebRTCStream(w, r, session)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	browserConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("browser dial failed: %v", err)
	}
	defer browserConn.Close()

	_ = readWebRTCAgentMessage(t, agentClientConn) // start

	if err := browserConn.WriteMessage(websocket.TextMessage, []byte("{")); err != nil {
		t.Fatalf("write malformed signal: %v", err)
	}
	assertNoAgentMessageWithin(t, agentClientConn, 150*time.Millisecond)

	if err := browserConn.WriteJSON(map[string]any{"type": "bogus", "data": map[string]any{"ignored": true}}); err != nil {
		t.Fatalf("write unknown signal: %v", err)
	}
	assertNoAgentMessageWithin(t, agentClientConn, 150*time.Millisecond)

	if err := browserConn.WriteJSON(map[string]any{"type": "stop"}); err != nil {
		t.Fatalf("write stop: %v", err)
	}
	waitForWebRTCBridgeRemoval(t, sut, session.ID)
}

func TestProcessAgentWebRTCHandlersRelayToBrowserAndCloseOnStopped(t *testing.T) {
	var srv apiServer
	browserServerConn, browserClientConn, cleanup := newWebSocketPair(t)
	defer cleanup()

	bridge := &webrtcSignalingBridge{
		BrowserWS:       browserServerConn,
		ExpectedAgentID: "node-1",
		ClosedCh:        make(chan struct{}),
	}
	srv.webrtcBridges.Store("sess-relay", bridge)

	conn := &agentmgr.AgentConn{AssetID: "node-1"}

	startedData, err := json.Marshal(agentmgr.WebRTCStartedData{
		SessionID:    "sess-relay",
		VideoEncoder: "x264",
		AudioSource:  "pipewire",
	})
	if err != nil {
		t.Fatalf("marshal started payload: %v", err)
	}
	srv.processAgentWebRTCStarted(conn, agentmgr.Message{Data: startedData})
	startedMsg := readWebRTCBrowserMessage(t, browserClientConn)
	if startedMsg.Type != "ready" {
		t.Fatalf("browser message type=%q, want ready", startedMsg.Type)
	}

	answerData, err := json.Marshal(agentmgr.WebRTCSDPData{
		SessionID: "sess-relay",
		Type:      "answer",
		SDP:       "v=0",
	})
	if err != nil {
		t.Fatalf("marshal answer payload: %v", err)
	}
	srv.processAgentWebRTCAnswer(conn, agentmgr.Message{Data: answerData})
	answerMsg := readWebRTCBrowserMessage(t, browserClientConn)
	if answerMsg.Type != "answer" {
		t.Fatalf("browser message type=%q, want answer", answerMsg.Type)
	}

	iceData, err := json.Marshal(agentmgr.WebRTCICEData{
		SessionID: "sess-relay",
		Candidate: "candidate:1 1 UDP 123 10.0.0.10 10000 typ host",
	})
	if err != nil {
		t.Fatalf("marshal ice payload: %v", err)
	}
	srv.processAgentWebRTCICE(conn, agentmgr.Message{Data: iceData})
	iceMsg := readWebRTCBrowserMessage(t, browserClientConn)
	if iceMsg.Type != "ice" {
		t.Fatalf("browser message type=%q, want ice", iceMsg.Type)
	}

	stoppedData, err := json.Marshal(agentmgr.WebRTCStoppedData{
		SessionID: "sess-relay",
		Reason:    "peer connection failed",
	})
	if err != nil {
		t.Fatalf("marshal stopped payload: %v", err)
	}
	srv.processAgentWebRTCStopped(conn, agentmgr.Message{Data: stoppedData})
	stoppedMsg := readWebRTCBrowserMessage(t, browserClientConn)
	if stoppedMsg.Type != "stopped" {
		t.Fatalf("browser message type=%q, want stopped", stoppedMsg.Type)
	}

	select {
	case <-bridge.ClosedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected bridge to close after stopped message")
	}
}

func TestProcessAgentWebRTCStartedAnswerICESkipMismatchedAgent(t *testing.T) {
	var srv apiServer
	browserServerConn, browserClientConn, cleanup := newWebSocketPair(t)
	defer cleanup()

	bridge := &webrtcSignalingBridge{
		BrowserWS:       browserServerConn,
		ExpectedAgentID: "node-1",
		ClosedCh:        make(chan struct{}),
	}
	srv.webrtcBridges.Store("sess-mismatch", bridge)

	conn := &agentmgr.AgentConn{AssetID: "node-2"}

	startedData, err := json.Marshal(agentmgr.WebRTCStartedData{SessionID: "sess-mismatch"})
	if err != nil {
		t.Fatalf("marshal started payload: %v", err)
	}
	srv.processAgentWebRTCStarted(conn, agentmgr.Message{Data: startedData})
	assertNoBrowserMessageWithin(t, browserClientConn, 150*time.Millisecond)

	answerData, err := json.Marshal(agentmgr.WebRTCSDPData{SessionID: "sess-mismatch", Type: "answer", SDP: "v=0"})
	if err != nil {
		t.Fatalf("marshal answer payload: %v", err)
	}
	srv.processAgentWebRTCAnswer(conn, agentmgr.Message{Data: answerData})
	assertNoBrowserMessageWithin(t, browserClientConn, 150*time.Millisecond)

	iceData, err := json.Marshal(agentmgr.WebRTCICEData{SessionID: "sess-mismatch", Candidate: "candidate:1 1 UDP 123 10.0.0.10 10000 typ host"})
	if err != nil {
		t.Fatalf("marshal ice payload: %v", err)
	}
	srv.processAgentWebRTCICE(conn, agentmgr.Message{Data: iceData})
	assertNoBrowserMessageWithin(t, browserClientConn, 150*time.Millisecond)
}

func assertNoPanicWebRTC(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	fn()
}

func readWebRTCAgentMessage(t *testing.T, conn *websocket.Conn) agentmgr.Message {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read agent message: %v", err)
	}
	return msg
}

func readWebRTCBrowserMessage(t *testing.T, conn *websocket.Conn) struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
} {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read browser message: %v", err)
	}
	var payload struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode browser message: %v", err)
	}
	return payload
}

func waitForWebRTCBridgeRemoval(t *testing.T, srv *apiServer, sessionID string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := srv.webrtcBridges.Load(sessionID); !ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected webrtc bridge %s to be removed", sessionID)
}

func assertNoAgentMessageWithin(t *testing.T, conn *websocket.Conn, timeout time.Duration) {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var msg agentmgr.Message
	err := conn.ReadJSON(&msg)
	if err == nil {
		t.Fatalf("expected no agent message, got %+v", msg)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("expected timeout waiting for agent message, got %v", err)
	}
}

func assertNoBrowserMessageWithin(t *testing.T, conn *websocket.Conn, timeout time.Duration) {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, raw, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("expected no browser message, got %s", string(raw))
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("expected timeout waiting for browser message, got %v", err)
	}
}
