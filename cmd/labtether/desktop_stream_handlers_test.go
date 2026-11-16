package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	desktoppkg "github.com/labtether/labtether/internal/hubapi/desktop"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDesktopBridgeCloseReasonRoundTrip(t *testing.T) {
	bridge := &desktoppkg.DesktopBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}

	if got := bridge.CloseReason(); got != "" {
		t.Fatalf("expected empty close reason, got %q", got)
	}

	bridge.SetReason("failed to start VNC: x11vnc not found")
	if got := bridge.CloseReason(); got != "failed to start VNC: x11vnc not found" {
		t.Fatalf("unexpected close reason %q", got)
	}
}

func TestNormalizeWebSocketCloseReasonTruncatesToProtocolLimit(t *testing.T) {
	longReason := strings.Repeat("x", desktoppkg.MaxWebSocketCloseReasonBytes+64)
	got := desktoppkg.NormalizeWebSocketCloseReason(longReason)
	if got == "" {
		t.Fatal("expected non-empty normalized close reason")
	}
	if len([]byte(got)) > desktoppkg.MaxWebSocketCloseReasonBytes {
		t.Fatalf("expected close reason <= %d bytes, got %d", desktoppkg.MaxWebSocketCloseReasonBytes, len([]byte(got)))
	}
}

func TestSafeWriteCloseHandlesOversizedReason(t *testing.T) {
	serverConnCh := make(chan *websocket.Conn, 1)
	serverErrCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := terminalWebSocketUpgrader.Upgrade(w, r, nil)
		if err != nil {
			serverErrCh <- err
			return
		}
		serverConnCh <- conn
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer clientConn.Close()

	var serverConn *websocket.Conn
	select {
	case err := <-serverErrCh:
		t.Fatalf("upgrade failed: %v", err)
	case serverConn = <-serverConnCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server websocket")
	}
	defer serverConn.Close()

	desktoppkg.SafeWriteClose(serverConn, websocket.CloseTryAgainLater, strings.Repeat("a", 500))

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, readErr := clientConn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(readErr, &closeErr) {
		t.Fatalf("expected close error, got %v", readErr)
	}
	if closeErr.Code != websocket.CloseTryAgainLater {
		t.Fatalf("expected close code %d, got %d (%q)", websocket.CloseTryAgainLater, closeErr.Code, closeErr.Text)
	}
	if len([]byte(closeErr.Text)) > desktoppkg.MaxWebSocketCloseReasonBytes {
		t.Fatalf("expected close reason <= %d bytes, got %d", desktoppkg.MaxWebSocketCloseReasonBytes, len([]byte(closeErr.Text)))
	}
}

func TestHandleDesktopStreamRejectsDirectProxyWhenDisabled(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "desktop-offline-node",
		Type:    "node",
		Name:    "Offline Desktop Node",
		Source:  "manual",
		Status:  "offline",
	}); err != nil {
		t.Fatalf("failed to seed asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/desktop/sessions/sess-1/stream", nil)
	rec := httptest.NewRecorder()
	sut.ensureDesktopDeps().SetDesktopSessionOptions("sess-1", desktoppkg.DesktopSessionOptions{
		Protocol: "webrtc",
		Display:  ":0",
		Quality:  "high",
	})
	sut.ensureDesktopDeps().HandleDesktopStream(rec, req, terminal.Session{
		ID:     "sess-1",
		Target: "desktop-offline-node",
		Mode:   "desktop",
	})

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when no route can reach the device, got %d: %s", rec.Code, rec.Body.String())
	}
	opts := sut.ensureDesktopDeps().GetDesktopSessionOptions("sess-1")
	if opts.Protocol != "webrtc" || opts.Display != ":0" || opts.Quality != "high" {
		t.Fatalf("expected desktop session options to remain available after failed stream attempt, got %+v", opts)
	}
}

func TestHandleDesktopStreamRejectsWaylandVNCFallbackWhenWebRTCUnavailable(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	conn := &agentmgr.AgentConn{AssetID: "wayland-node"}
	conn.SetMeta("webrtc_available", "false")
	conn.SetMeta("webrtc_unavailable_reason", "wayland_pipewire_node_missing")
	conn.SetMeta("desktop_session_type", "wayland")
	conn.SetMeta("desktop_vnc_real_desktop_supported", "false")
	sut.agentMgr.Register(conn)

	req := httptest.NewRequest(http.MethodGet, "/desktop/sessions/sess-wayland/stream", nil)
	rec := httptest.NewRecorder()
	sut.ensureDesktopDeps().SetDesktopSessionOptions("sess-wayland", desktoppkg.DesktopSessionOptions{
		Protocol: "webrtc",
	})

	sut.ensureDesktopDeps().HandleDesktopStream(rec, req, terminal.Session{
		ID:     "sess-wayland",
		Target: "wayland-node",
		Mode:   "desktop",
	})

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for unsupported Wayland VNC fallback, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message in response, got %s", rec.Body.String())
	}
}

func TestProcessAgentDesktopClosedStoresReasonAndSignalsBridge(t *testing.T) {
	var srv apiServer
	bridge := &desktoppkg.DesktopBridge{
		OutputCh:        make(chan []byte, 1),
		ExpectedAgentID: "node-1",
		ClosedCh:        make(chan struct{}),
	}
	srv.desktopBridges.Store("sess-1", bridge)

	payload, err := json.Marshal(agentmgr.DesktopCloseData{
		SessionID: "sess-1",
		Reason:    "failed to start VNC: x11vnc not found",
	})
	if err != nil {
		t.Fatalf("marshal close payload: %v", err)
	}

	srv.ensureDesktopDeps().ProcessAgentDesktopClosed(&agentmgr.AgentConn{AssetID: "node-1"}, agentmgr.Message{Data: payload})

	if got := bridge.CloseReason(); got != "failed to start VNC: x11vnc not found" {
		t.Fatalf("unexpected close reason %q", got)
	}
	select {
	case <-bridge.ClosedCh:
	default:
		t.Fatal("expected bridge closed channel to be closed")
	}
}

func TestProcessAgentDesktopAudioDataRoutesBinaryToBridge(t *testing.T) {
	var srv apiServer
	bridge := &desktoppkg.DesktopBridge{
		AudioCh:         make(chan desktoppkg.DesktopAudioOutbound, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
		SessionID:       "sess-audio",
	}
	srv.desktopBridges.Store("sess-audio", bridge)

	conn := &agentmgr.AgentConn{AssetID: "node-1"}
	payload, err := json.Marshal(agentmgr.DesktopAudioDataPayload{
		SessionID: "sess-audio",
		Data:      base64.StdEncoding.EncodeToString([]byte("ogg-payload")),
		Timestamp: time.Now().UnixMilli(),
	})
	if err != nil {
		t.Fatalf("marshal audio payload: %v", err)
	}

	srv.ensureDesktopDeps().ProcessAgentDesktopAudioData(conn, agentmgr.Message{Data: payload})

	select {
	case outbound := <-bridge.AudioCh:
		if outbound.MessageType != websocket.BinaryMessage {
			t.Fatalf("expected binary message, got %d", outbound.MessageType)
		}
		if got := string(outbound.Payload); got != "ogg-payload" {
			t.Fatalf("unexpected audio payload %q", got)
		}
	default:
		t.Fatal("expected audio payload to be routed to bridge")
	}
}

func TestProcessAgentDesktopAudioStateRoutesTextToBridge(t *testing.T) {
	var srv apiServer
	bridge := &desktoppkg.DesktopBridge{
		AudioCh:         make(chan desktoppkg.DesktopAudioOutbound, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
		SessionID:       "sess-audio",
	}
	srv.desktopBridges.Store("sess-audio", bridge)

	conn := &agentmgr.AgentConn{AssetID: "node-1"}
	payload, err := json.Marshal(agentmgr.DesktopAudioStateData{
		SessionID: "sess-audio",
		State:     "unavailable",
		Error:     "ffmpeg missing",
	})
	if err != nil {
		t.Fatalf("marshal audio state payload: %v", err)
	}

	srv.ensureDesktopDeps().ProcessAgentDesktopAudioState(conn, agentmgr.Message{Data: payload})

	select {
	case outbound := <-bridge.AudioCh:
		if outbound.MessageType != websocket.TextMessage {
			t.Fatalf("expected text message, got %d", outbound.MessageType)
		}
		if !strings.Contains(string(outbound.Payload), "\"state\":\"unavailable\"") {
			t.Fatalf("unexpected state payload %s", string(outbound.Payload))
		}
	default:
		t.Fatal("expected audio state payload to be routed to bridge")
	}
}

func TestBridgeDesktopInputIdleNoPanicAndClosesOnSessionEnd(t *testing.T) {
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
		_, _ = srv.ensureDesktopDeps().BridgeDesktopInput(serverConn, nil, "sess-idle", closedCh, nil)
	}()

	// Keep the bridge idle long enough to cover the previous timeout/panic path.
	select {
	case <-done:
		t.Fatal("bridgeDesktopInput returned unexpectedly during idle period")
	case <-time.After(2500 * time.Millisecond):
	}

	select {
	case r := <-panicCh:
		t.Fatalf("bridgeDesktopInput panicked while idle: %v", r)
	default:
	}

	close(closedCh)
	_ = clientConn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bridgeDesktopInput did not exit after session close")
	}
}

func TestShouldUseWebRTCReadsAgentCapabilities(t *testing.T) {
	srv := apiServer{agentMgr: agentmgr.NewManager()}

	if srv.ensureDesktopDeps().ShouldUseWebRTC("node-1") {
		t.Fatal("expected false when no agent is connected")
	}

	conn := &agentmgr.AgentConn{AssetID: "node-1"}
	srv.agentMgr.Register(conn)

	if srv.ensureDesktopDeps().ShouldUseWebRTC("node-1") {
		t.Fatal("expected false when agent has no webrtc capability metadata")
	}

	conn.SetMeta("webrtc_available", " TrUe ")
	if !srv.ensureDesktopDeps().ShouldUseWebRTC("node-1") {
		t.Fatal("expected true when webrtc_available=true")
	}

	conn.SetMeta("webrtc_available", "false")
	if srv.ensureDesktopDeps().ShouldUseWebRTC("node-1") {
		t.Fatal("expected false when capability is explicitly disabled")
	}
}

func TestProcessAgentDesktopHandlersIgnoreNonBridgeEntries(t *testing.T) {
	var srv apiServer
	srv.desktopBridges.Store("sess-non-bridge", "ignore-me")

	startedPayload, err := json.Marshal(agentmgr.DesktopStartedData{SessionID: "sess-non-bridge"})
	if err != nil {
		t.Fatalf("marshal desktop started payload: %v", err)
	}
	dataPayload, err := json.Marshal(agentmgr.DesktopDataPayload{
		SessionID: "sess-non-bridge",
		Data:      base64.StdEncoding.EncodeToString([]byte("pixels")),
	})
	if err != nil {
		t.Fatalf("marshal desktop data payload: %v", err)
	}
	closedPayload, err := json.Marshal(agentmgr.DesktopCloseData{
		SessionID: "sess-non-bridge",
		Reason:    "agent disconnected",
	})
	if err != nil {
		t.Fatalf("marshal desktop close payload: %v", err)
	}

	srv.ensureDesktopDeps().ProcessAgentDesktopStarted(nil, agentmgr.Message{Data: startedPayload})
	srv.ensureDesktopDeps().ProcessAgentDesktopData(nil, agentmgr.Message{Data: dataPayload})
	srv.ensureDesktopDeps().ProcessAgentDesktopClosed(nil, agentmgr.Message{Data: closedPayload})
}

func TestHandleAgentDesktopStreamClosesBeforeReadyWhenAgentCloses(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	agentServerConn, agentClientConn, agentCleanup := newWebSocketPair(t)
	defer agentCleanup()

	agentConn := agentmgr.NewAgentConn(agentServerConn, "node-desktop", "linux")
	sut.agentMgr.Register(agentConn)
	defer sut.agentMgr.Unregister("node-desktop")

	session := terminal.Session{ID: "desk-before-ready", Target: "node-desktop", Mode: "desktop"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.ensureDesktopDeps().HandleAgentDesktopStream(w, r, session)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?quality=high&display=%3A77"
	browserConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial browser websocket failed: %v", err)
	}
	defer browserConn.Close()

	_ = agentClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var startMsg agentmgr.Message
	if err := agentClientConn.ReadJSON(&startMsg); err != nil {
		t.Fatalf("read desktop.start: %v", err)
	}
	if startMsg.Type != agentmgr.MsgDesktopStart {
		t.Fatalf("message type=%q, want %q", startMsg.Type, agentmgr.MsgDesktopStart)
	}
	var start agentmgr.DesktopStartData
	if err := json.Unmarshal(startMsg.Data, &start); err != nil {
		t.Fatalf("decode desktop.start payload: %v", err)
	}
	if start.SessionID == "" || start.SessionID == session.ID || startMsg.ID != start.SessionID {
		t.Fatalf("expected a separate desktop wire ID, got %+v", start)
	}
	if start.Quality != "high" {
		t.Fatalf("quality=%q, want high", start.Quality)
	}
	if start.Display != ":77" {
		t.Fatalf("display=%q, want :77", start.Display)
	}

	closedPayload, err := json.Marshal(agentmgr.DesktopCloseData{
		SessionID: start.SessionID,
		Reason:    "failed to start VNC: x11vnc not found: install with 'apt install x11vnc' or 'pkg install x11vnc'",
	})
	if err != nil {
		t.Fatalf("marshal desktop close payload: %v", err)
	}
	sut.ensureDesktopDeps().ProcessAgentDesktopClosed(agentConn, agentmgr.Message{Data: closedPayload})

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, readErr := browserConn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(readErr, &closeErr) {
		t.Fatalf("expected browser close error, got %v", readErr)
	}
	if closeErr.Code != websocket.CloseTryAgainLater {
		t.Fatalf("close code=%d, want %d", closeErr.Code, websocket.CloseTryAgainLater)
	}
	// The viewer shows this reason. It said "agent desktop session closed
	// unexpectedly" for every cause, so nobody learnt x11vnc was missing.
	if want := "Remote View needs x11vnc on this device. Install it, then try again: sudo apt install x11vnc"; closeErr.Text != want {
		t.Fatalf("close reason=%q, want %q", closeErr.Text, want)
	}

	_ = agentClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var closeMsg agentmgr.Message
	if err := agentClientConn.ReadJSON(&closeMsg); err != nil {
		t.Fatalf("read desktop.close cleanup message: %v", err)
	}
	if closeMsg.Type != agentmgr.MsgDesktopClose {
		t.Fatalf("message type=%q, want %q", closeMsg.Type, agentmgr.MsgDesktopClose)
	}
	var closeReq agentmgr.DesktopCloseData
	if err := json.Unmarshal(closeMsg.Data, &closeReq); err != nil {
		t.Fatalf("decode desktop.close payload: %v", err)
	}
	if closeReq.SessionID != start.SessionID {
		t.Fatalf("cleanup session_id=%q, want %q", closeReq.SessionID, start.SessionID)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := sut.desktopBridges.Load(start.SessionID); !ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected desktop bridge cleanup after agent close-before-ready")
}
