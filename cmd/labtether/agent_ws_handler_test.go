package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

type countingBatchLogStore struct {
	persistence.LogStore
	appendEventsCalls int
	appendEventsCount int
}

type browserEventEnvelope struct {
	Type string `json:"type"`
}

func (c *countingBatchLogStore) AppendEvents(events []logs.Event) error {
	c.appendEventsCalls++
	c.appendEventsCount += len(events)
	for _, event := range events {
		if err := c.LogStore.AppendEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func TestKnownMessageTypes(t *testing.T) {
	sut := newTestAPIServer(t)
	router := sut.buildWSRouter()

	expected := []string{
		agentmgr.MsgHeartbeat,
		agentmgr.MsgTelemetry,
		agentmgr.MsgCommandResult,
		agentmgr.MsgPong,
		agentmgr.MsgLogStream,
		agentmgr.MsgLogBatch,
		agentmgr.MsgJournalEntries,
		agentmgr.MsgUpdateProgress,
		agentmgr.MsgUpdateResult,
		agentmgr.MsgTerminalProbed,
		agentmgr.MsgTerminalStarted,
		agentmgr.MsgTerminalData,
		agentmgr.MsgTerminalClosed,
		agentmgr.MsgSSHKeyInstalled,
		agentmgr.MsgSSHKeyRemoved,
		agentmgr.MsgDesktopStarted,
		agentmgr.MsgDesktopData,
		agentmgr.MsgDesktopClosed,
		agentmgr.MsgDesktopDisplays,
		agentmgr.MsgDesktopAudioData,
		agentmgr.MsgDesktopAudioState,
		agentmgr.MsgDesktopDiagnosed,
		agentmgr.MsgWebRTCCapabilities,
		agentmgr.MsgWebRTCStarted,
		agentmgr.MsgWebRTCAnswer,
		agentmgr.MsgWebRTCICE,
		agentmgr.MsgWebRTCStopped,
		agentmgr.MsgClipboardData,
		agentmgr.MsgClipboardSetAck,
		agentmgr.MsgWoLResult,
		agentmgr.MsgFileListed,
		agentmgr.MsgFileData,
		agentmgr.MsgFileWritten,
		agentmgr.MsgFileResult,
		agentmgr.MsgProcessListed,
		agentmgr.MsgProcessKillResult,
		agentmgr.MsgServiceListed,
		agentmgr.MsgServiceResult,
		agentmgr.MsgDiskListed,
		agentmgr.MsgNetworkListed,
		agentmgr.MsgNetworkResult,
		agentmgr.MsgPackageListed,
		agentmgr.MsgPackageResult,
		agentmgr.MsgCronListed,
		agentmgr.MsgUsersListed,
		agentmgr.MsgConfigApplied,
		agentmgr.MsgAgentSettingsApplied,
		agentmgr.MsgAgentSettingsState,
		agentmgr.MsgDockerDiscovery,
		agentmgr.MsgDockerDiscoveryDelta,
		agentmgr.MsgDockerStats,
		agentmgr.MsgDockerEvents,
		agentmgr.MsgDockerActionResult,
		agentmgr.MsgDockerExecStarted,
		agentmgr.MsgDockerExecData,
		agentmgr.MsgDockerExecClosed,
		agentmgr.MsgDockerLogsStream,
		agentmgr.MsgDockerComposeResult,
		agentmgr.MsgWebServiceReport,
		agentmgr.MsgClipboardData,
		agentmgr.MsgClipboardSetAck,
	}

	expectedSet := make(map[string]struct{}, len(expected))
	for _, msgType := range expected {
		expectedSet[msgType] = struct{}{}
		if _, ok := router[msgType]; !ok {
			t.Fatalf("missing router handler for message type %q", msgType)
		}
	}

	var extras []string
	for msgType := range router {
		if _, ok := expectedSet[msgType]; !ok {
			extras = append(extras, msgType)
		}
	}

	if len(extras) > 0 {
		slices.Sort(extras)
		t.Fatalf("router contains unexpected message types: %v", extras)
	}

	if len(router) != len(expectedSet) {
		t.Fatalf("router size mismatch: got=%d want=%d", len(router), len(expectedSet))
	}
}

func TestWithAuthReturnsUnauthorizedWithoutAuthValidator(t *testing.T) {
	sut := newTestAPIServer(t)

	called := false
	protected := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/assets", nil)
	rr := httptest.NewRecorder()
	protected(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when auth validator is unavailable, got %d", rr.Code)
	}
	if called {
		t.Fatalf("protected handler should not run for unauthorized request")
	}
}

func TestHandleBrowserEventsReturnsUnauthorizedWithoutAuthValidator(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ws/events", nil)
	rr := httptest.NewRecorder()
	sut.handleBrowserEvents(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when auth validator is unavailable, got %d", rr.Code)
	}
}

func TestHandleAgentWebSocketReturnsUnauthorizedWithoutAuthValidator(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ws/agent", nil)
	req.Header.Set("X-Asset-ID", "node-01")
	rr := httptest.NewRecorder()
	sut.handleAgentWebSocket(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when auth validator is unavailable, got %d", rr.Code)
	}
}

func TestHandleAgentWebSocketRequiresAssetIDHeader(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.authValidator = auth.NewTokenValidator("owner-token")

	req := httptest.NewRequest(http.MethodGet, "/ws/agent", nil)
	req.Header.Set("Authorization", "Bearer owner-token")
	rr := httptest.NewRecorder()
	sut.handleAgentWebSocket(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when X-Asset-ID is missing, got %d", rr.Code)
	}
}

func TestBuildHTTPHandlers_DevModePprofRequiresAuth(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	sut := newTestAPIServer(t)

	handlers := sut.buildHTTPHandlers(nil, nil, nil)
	pprofHandler, ok := handlers["/debug/pprof/"]
	if !ok || pprofHandler == nil {
		t.Fatalf("expected /debug/pprof/ handler when DEV_MODE=true")
	}

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rr := httptest.NewRecorder()
	pprofHandler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated pprof request, got %d", rr.Code)
	}
}

func TestProcessAgentHeartbeatIgnoresPayloadAssetID(t *testing.T) {
	sut := newTestAPIServer(t)
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	payload, err := json.Marshal(agentmgr.HeartbeatData{
		AssetID:  "spoofed-node",
		Type:     "node",
		Name:     "trusted-node",
		Source:   "agent",
		Status:   "online",
		Platform: "linux",
	})
	if err != nil {
		t.Fatalf("failed to marshal heartbeat payload: %v", err)
	}

	sut.processAgentHeartbeat(conn, agentmgr.Message{Type: agentmgr.MsgHeartbeat, Data: payload})

	if _, ok, err := sut.assetStore.GetAsset("trusted-node"); err != nil {
		t.Fatalf("failed to get trusted asset: %v", err)
	} else if !ok {
		t.Fatalf("expected trusted asset to be updated")
	}
	if _, ok, err := sut.assetStore.GetAsset("spoofed-node"); err != nil {
		t.Fatalf("failed to get spoofed asset: %v", err)
	} else if ok {
		t.Fatalf("did not expect spoofed asset to be updated from payload asset_id")
	}
}

func TestProcessAgentHeartbeatStoresWebRTCUnavailabilityReason(t *testing.T) {
	sut := newTestAPIServer(t)
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	payload, err := json.Marshal(agentmgr.HeartbeatData{
		Type:     "node",
		Name:     "trusted-node",
		Source:   "agent",
		Status:   "online",
		Platform: "darwin",
		Metadata: map[string]string{
			"webrtc_available":          "false",
			"webrtc_unavailable_reason": "unsupported_platform:darwin",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal heartbeat payload: %v", err)
	}

	sut.processAgentHeartbeat(conn, agentmgr.Message{Type: agentmgr.MsgHeartbeat, Data: payload})

	if got := conn.Meta("webrtc_unavailable_reason"); got != "unsupported_platform:darwin" {
		t.Fatalf("expected heartbeat to store webrtc unavailability reason, got %q", got)
	}
	assetEntry, ok, err := sut.assetStore.GetAsset("trusted-node")
	if err != nil {
		t.Fatalf("failed to get trusted asset: %v", err)
	}
	if !ok {
		t.Fatal("expected trusted asset to exist")
	}
	if got := assetEntry.Metadata["webrtc_unavailable_reason"]; got != "unsupported_platform:darwin" {
		t.Fatalf("expected asset metadata to persist webrtc reason, got %q", got)
	}
}

func TestProcessAgentTelemetryIgnoresPayloadAssetID(t *testing.T) {
	sut := newTestAPIServer(t)
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	payload, err := json.Marshal(agentmgr.TelemetryData{
		AssetID:       "spoofed-node",
		CPUPercent:    42,
		MemoryPercent: 55,
		DiskPercent:   67,
	})
	if err != nil {
		t.Fatalf("failed to marshal telemetry payload: %v", err)
	}

	sut.processAgentTelemetry(conn, agentmgr.Message{Type: agentmgr.MsgTelemetry, Data: payload})

	at := time.Now().UTC().Add(time.Second)
	trustedSnapshot, err := sut.telemetryStore.Snapshot("trusted-node", at)
	if err != nil {
		t.Fatalf("failed to read trusted snapshot: %v", err)
	}
	if trustedSnapshot.CPUUsedPercent == nil {
		t.Fatalf("expected trusted asset telemetry sample")
	}
	spoofedSnapshot, err := sut.telemetryStore.Snapshot("spoofed-node", at)
	if err != nil {
		t.Fatalf("failed to read spoofed snapshot: %v", err)
	}
	if spoofedSnapshot.CPUUsedPercent != nil || spoofedSnapshot.MemoryUsedPercent != nil || spoofedSnapshot.DiskUsedPercent != nil {
		t.Fatalf("did not expect spoofed asset telemetry samples")
	}
}

func TestProcessAgentLogHandlersIgnorePayloadAssetID(t *testing.T) {
	sut := newTestAPIServer(t)
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	streamPayload, err := json.Marshal(agentmgr.LogStreamData{
		AssetID: "spoofed-node",
		Source:  "agent",
		Level:   "info",
		Message: "single message",
	})
	if err != nil {
		t.Fatalf("failed to marshal log stream payload: %v", err)
	}
	sut.processAgentLogStream(conn, agentmgr.Message{Type: agentmgr.MsgLogStream, Data: streamPayload})

	batchPayload, err := json.Marshal(agentmgr.LogBatchData{
		Entries: []agentmgr.LogStreamData{
			{
				AssetID: "spoofed-node",
				Source:  "agent",
				Level:   "warn",
				Message: "batched message",
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal log batch payload: %v", err)
	}
	sut.processAgentLogBatch(conn, agentmgr.Message{Type: agentmgr.MsgLogBatch, Data: batchPayload})

	from := time.Now().UTC().Add(-time.Minute)
	to := time.Now().UTC().Add(time.Minute)

	trustedEvents, err := sut.logStore.QueryEvents(logs.QueryRequest{
		AssetID: "trusted-node",
		From:    from,
		To:      to,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("failed to query trusted logs: %v", err)
	}
	if len(trustedEvents) != 2 {
		t.Fatalf("expected 2 trusted events, got %d", len(trustedEvents))
	}

	spoofedEvents, err := sut.logStore.QueryEvents(logs.QueryRequest{
		AssetID: "spoofed-node",
		From:    from,
		To:      to,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("failed to query spoofed logs: %v", err)
	}
	if len(spoofedEvents) != 0 {
		t.Fatalf("expected 0 spoofed events, got %d", len(spoofedEvents))
	}
}

func TestProcessAgentLogBatchUsesBatchAppenderWhenAvailable(t *testing.T) {
	sut := newTestAPIServer(t)
	counter := &countingBatchLogStore{LogStore: sut.logStore}
	sut.logStore = counter
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	batchPayload, err := json.Marshal(agentmgr.LogBatchData{
		Entries: []agentmgr.LogStreamData{
			{Source: "agent", Level: "info", Message: "one"},
			{Source: "agent", Level: "warning", Message: "two"},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal log batch payload: %v", err)
	}
	sut.processAgentLogBatch(conn, agentmgr.Message{Type: agentmgr.MsgLogBatch, Data: batchPayload})

	if counter.appendEventsCalls != 1 {
		t.Fatalf("appendEventsCalls=%d, want 1", counter.appendEventsCalls)
	}
	if counter.appendEventsCount != 2 {
		t.Fatalf("appendEventsCount=%d, want 2", counter.appendEventsCount)
	}
}

func TestHandleAgentWebSocketRejectsAgentTokenAssetMismatch(t *testing.T) {
	sut := newTestAPIServer(t)

	raw, hash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate agent token: %v", err)
	}
	if _, err := sut.enrollmentStore.CreateAgentToken("node-allowed", hash, "test", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create agent token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/agent", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	req.Header.Set("X-Asset-ID", "node-other")
	rr := httptest.NewRecorder()
	sut.handleAgentWebSocket(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for token/asset mismatch, got %d", rr.Code)
	}
}

func TestHandleAgentWebSocketRejectsInvalidAgentToken(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ws/agent", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	req.Header.Set("X-Asset-ID", "node-01")
	rr := httptest.NewRecorder()
	sut.handleAgentWebSocket(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid agent token, got %d", rr.Code)
	}
}

func TestHandleAgentWebSocketReconnectDoesNotEmitStaleDisconnect(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.authValidator = auth.NewTokenValidator("owner-token")
	sut.agentMgr = agentmgr.NewManager()
	sut.broadcaster = newEventBroadcaster()

	eventServerConn, eventClientConn, cleanupEvents := createWSPairForPendingEnrollmentTest(t)
	defer cleanupEvents()
	browserClient := sut.broadcaster.Register(eventServerConn)
	defer sut.broadcaster.Unregister(browserClient)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handleAgentWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):]
	dialAgent := func() *websocket.Conn {
		headers := http.Header{}
		headers.Set("Authorization", "Bearer owner-token")
		headers.Set("X-Asset-ID", "node-01")
		headers.Set("X-Platform", "linux")

		conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
		if err != nil {
			t.Fatalf("dial agent websocket: %v", err)
		}
		return conn
	}

	first := dialAgent()
	defer first.Close()
	if got := readBrowserEventType(t, eventClientConn, 2*time.Second); got != "agent.connected" {
		t.Fatalf("expected first browser event agent.connected, got %q", got)
	}

	second := dialAgent()
	defer second.Close()
	if got := readBrowserEventType(t, eventClientConn, 2*time.Second); got != "agent.connected" {
		t.Fatalf("expected second browser event agent.connected, got %q", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sut.agentMgr.IsConnected("node-01") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sut.agentMgr.IsConnected("node-01") {
		t.Fatalf("expected replacement agent connection to remain registered")
	}

	eventClientConn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	_, _, err := eventClientConn.ReadMessage()
	if err == nil {
		t.Fatal("expected no stale agent.disconnected browser event")
	}
	netErr, ok := err.(net.Error)
	if !ok || !netErr.Timeout() {
		t.Fatalf("expected timeout while waiting for stale disconnect event, got %v", err)
	}
	_ = eventClientConn.SetReadDeadline(time.Time{})

	if err := second.Close(); err != nil {
		t.Fatalf("close replacement agent websocket: %v", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !sut.agentMgr.IsConnected("node-01") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected replacement agent connection to be unregistered after close")
}

func readBrowserEventType(t *testing.T, conn *websocket.Conn, timeout time.Duration) string {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	defer func() {
		_ = conn.SetReadDeadline(time.Time{})
	}()

	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read browser event: %v", err)
	}

	var envelope browserEventEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode browser event: %v", err)
	}
	return envelope.Type
}

func TestProcessAgentCommandResultRejectsMismatchedSender(t *testing.T) {
	sut := newTestAPIServer(t)
	resultCh := make(chan agentmgr.CommandResultData, 1)
	sut.pendingAgentCmds.Store("job-1", pendingAgentCommand{
		ResultCh:          resultCh,
		ExpectedAssetID:   "node-1",
		ExpectedSessionID: "sess-1",
		ExpectedCommandID: "cmd-1",
	})

	payload, err := json.Marshal(agentmgr.CommandResultData{
		JobID:     "job-1",
		SessionID: "sess-1",
		CommandID: "cmd-1",
		Status:    "succeeded",
		Output:    "ok",
	})
	if err != nil {
		t.Fatalf("marshal command result payload: %v", err)
	}

	sut.processAgentCommandResult(&agentmgr.AgentConn{AssetID: "node-2"}, agentmgr.Message{Data: payload})
	select {
	case <-resultCh:
		t.Fatal("expected mismatched sender result to be ignored")
	default:
	}

	sut.processAgentCommandResult(&agentmgr.AgentConn{AssetID: "node-1"}, agentmgr.Message{Data: payload})
	select {
	case result := <-resultCh:
		if result.SessionID != "sess-1" || result.CommandID != "cmd-1" {
			t.Fatalf("unexpected correlated result %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("expected matched sender result to be delivered")
	}
}
