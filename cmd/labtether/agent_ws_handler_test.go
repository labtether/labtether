package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

type blockingTokenIDValidationStore struct {
	persistence.EnrollmentStore
	persistence.AgentEnrollmentTransactionStore
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (s *blockingTokenIDValidationStore) ValidateActiveAgentTokenID(ctx context.Context, tokenID, assetID string) error {
	s.once.Do(func() {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
		}
	})
	return s.AgentEnrollmentTransactionStore.ValidateActiveAgentTokenID(ctx, tokenID, assetID)
}

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
		agentmgr.MsgPowerResult,
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

func TestHandleAgentWebSocketRejectsSharedOwnerTokenByDefault(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.authValidator = auth.NewTokenValidator("owner-token")
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "owner-agent", Type: "node", Name: "owner-agent", Source: "agent",
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/ws/agent", nil)
	req.Header.Set("Authorization", "Bearer owner-token")
	req.Header.Set("X-Asset-ID", "owner-agent")
	rec := httptest.NewRecorder()
	sut.handleAgentWebSocket(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("shared owner token status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentWSInboundBudgetAccountsForMessagesAndBytes(t *testing.T) {
	base := time.Now()
	budget := &agentWSInboundBudget{
		messagesPerSecond: 1,
		messageBurst:      2,
		bytesPerSecond:    4,
		byteBurst:         5,
		messageTokens:     2,
		byteTokens:        5,
		lastRefill:        base,
	}
	if !budget.allow(2, base) || !budget.allow(3, base) {
		t.Fatal("initial burst was rejected")
	}
	if budget.allow(1, base) {
		t.Fatal("message/byte burst overrun was admitted")
	}
	if !budget.allow(1, base.Add(time.Second)) {
		t.Fatal("refilled budget was rejected")
	}
}

func TestConfiguredAgentWSCredentialLeaseIsHardBounded(t *testing.T) {
	t.Setenv("LABTETHER_AGENT_WS_CREDENTIAL_LEASE", "1ms")
	if got := configuredAgentWSCredentialLease(); got != minAgentWSCredentialLease {
		t.Fatalf("minimum lease=%s", got)
	}
	t.Setenv("LABTETHER_AGENT_WS_CREDENTIAL_LEASE", "1h")
	if got := configuredAgentWSCredentialLease(); got != maxAgentWSCredentialLease {
		t.Fatalf("maximum lease=%s", got)
	}
}

func TestAgentWSMessageTypeIsBoundedBeforeDispatchOrLogging(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "known shape", value: string(agentmgr.MsgHeartbeat), valid: true},
		{name: "empty", value: "", valid: false},
		{name: "control", value: "heartbeat\nforged", valid: false},
		{name: "oversized", value: strings.Repeat("x", maxAgentWSMessageTypeBytes+1), valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, bounded := boundedAgentWebSocketHeader(test.value, maxAgentWSMessageTypeBytes)
			got := bounded && value != ""
			if got != test.valid {
				t.Fatalf("message type valid=%v, want %v", got, test.valid)
			}
		})
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

func TestHandleAgentWebSocketRejectsUnboundedOrUnsafeHeaders(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		value   string
		assetID string
	}{
		{name: "asset too long", header: "X-Asset-ID", value: strings.Repeat("a", maxAgentWSAssetIDBytes+1)},
		{name: "asset control", header: "X-Asset-ID", value: "node\nforged"},
		{name: "asset invalid utf8", header: "X-Asset-ID", value: string([]byte{0xff})},
		{name: "platform too long", header: "X-Platform", value: strings.Repeat("p", maxAgentWSPlatformBytes+1), assetID: "node-safe"},
		{name: "platform control", header: "X-Platform", value: "linux\rforged", assetID: "node-safe"},
		{name: "version too long", header: "X-Agent-Version", value: strings.Repeat("v", maxAgentWSAgentVersionBytes+1), assetID: "node-safe"},
		{name: "version control", header: "X-Agent-Version", value: "1.0\nforged", assetID: "node-safe"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sut := newTestAPIServer(t)
			req := httptest.NewRequest(http.MethodGet, "/ws/agent", nil)
			if tc.assetID != "" {
				req.Header.Set("X-Asset-ID", tc.assetID)
			}
			req.Header[tc.header] = []string{tc.value}
			rr := httptest.NewRecorder()
			sut.handleAgentWebSocket(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s, want 400", rr.Code, rr.Body.String())
			}
		})
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

func TestHandleAgentWebSocketUsesTokenBoundAssetForStaleHeader(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	raw, hash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate agent token: %v", err)
	}
	if _, err := sut.enrollmentStore.CreateAgentToken("node-allowed", hash, "test", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create agent token: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-allowed", Type: "node", Name: "node-allowed", Source: "agent", Status: "online",
	}); err != nil {
		t.Fatalf("seed token-bound asset: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handleAgentWebSocket(w, r)
	}))
	defer server.Close()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+raw)
	headers.Set("X-Asset-ID", "node-other")
	conn, response, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+"/ws/agent", headers)
	if err != nil {
		t.Fatalf("dial with stale asset header: %v", err)
	}
	defer conn.Close()
	if response == nil {
		t.Fatal("expected websocket upgrade response")
	}
	if response.Header.Get("X-LabTether-Asset-ID") != "node-allowed" {
		t.Fatalf("upgrade canonical asset header=%q, want node-allowed", response.Header.Get("X-LabTether-Asset-ID"))
	}
	deadline := time.Now().Add(time.Second)
	for !sut.agentMgr.IsConnected("node-allowed") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !sut.agentMgr.IsConnected("node-allowed") {
		t.Fatal("expected token-bound asset connection to be registered")
	}
	if sut.agentMgr.IsConnected("node-other") {
		t.Fatal("stale untrusted header must not select the registered asset")
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

func TestHandleAgentWebSocketRevalidatesTokenIDAfterUpgrade(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	transactions, ok := sut.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	if !ok {
		t.Fatal("test enrollment store lacks transaction interface")
	}
	now := time.Now().UTC()
	rawEnrollment, enrollmentHash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	_ = rawEnrollment
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(enrollmentHash, "handshake", now.Add(time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	rawAgent, agentHash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	result, err := transactions.CommitAgentEnrollment(context.Background(), persistence.AgentEnrollmentCommitRequest{
		AssetID: "node-handshake", Hostname: "node-handshake", EnrollmentTokenHash: enrollmentHash,
		AgentTokenHash: agentHash, AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil || result.AgentToken.ID == "" {
		t.Fatalf("seed enrollment: result=%+v err=%v", result, err)
	}
	wrapped := &blockingTokenIDValidationStore{
		EnrollmentStore:                 sut.enrollmentStore,
		AgentEnrollmentTransactionStore: transactions,
		entered:                         make(chan struct{}),
		release:                         make(chan struct{}),
	}
	sut.enrollmentStore = wrapped

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handleAgentWebSocket(w, r)
	}))
	defer server.Close()
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+rawAgent)
	headers.Set("X-Asset-ID", "node-handshake")
	dialDone := make(chan *websocket.Conn, 1)
	go func() {
		conn, _, _ := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):], headers)
		dialDone <- conn
	}()

	select {
	case <-wrapped.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("token-ID revalidation was not reached after upgrade")
	}
	if err := transactions.DecommissionAgentAsset(context.Background(), "node-handshake"); err != nil {
		t.Fatalf("decommission during handshake: %v", err)
	}
	close(wrapped.release)
	conn := <-dialDone
	if conn != nil {
		defer conn.Close()
	}
	deadline := time.Now().Add(time.Second)
	for sut.agentMgr.IsConnected("node-handshake") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if sut.agentMgr.IsConnected("node-handshake") {
		t.Fatal("revoked token registered after decommission during handshake")
	}
	if _, exists, err := sut.assetStore.GetAsset("node-handshake"); err != nil || exists {
		t.Fatalf("decommissioned handshake asset exists=%v err=%v", exists, err)
	}
}

func TestRevokedLiveAgentHeartbeatCannotResurrectDecommissionedAsset(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	transactions, ok := sut.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	if !ok {
		t.Fatal("test enrollment store lacks transaction interface")
	}
	now := time.Now().UTC()
	_, enrollmentHash, _ := auth.GenerateSessionToken()
	_, agentHash, _ := auth.GenerateSessionToken()
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(enrollmentHash, "heartbeat", now.Add(time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	result, err := transactions.CommitAgentEnrollment(context.Background(), persistence.AgentEnrollmentCommitRequest{
		AssetID: "node-revoked-heartbeat", Hostname: "node-revoked-heartbeat", EnrollmentTokenHash: enrollmentHash,
		AgentTokenHash: agentHash, AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	serverConn, _, cleanup := createWSPairForPendingEnrollmentTest(t)
	defer cleanup()
	conn := agentmgr.NewAgentConn(serverConn, "node-revoked-heartbeat", "linux")
	conn.SetMeta("auth.mode", "agent-token")
	conn.SetMeta("auth.agent_token_id", result.AgentToken.ID)
	sut.agentMgr.Register(conn)
	if err := transactions.DecommissionAgentAsset(context.Background(), "node-revoked-heartbeat"); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(agentmgr.HeartbeatData{
		Type: "node", Name: "node-revoked-heartbeat", Source: "agent", Status: "online", Platform: "linux",
	})
	sut.processAgentHeartbeat(conn, agentmgr.Message{Type: agentmgr.MsgHeartbeat, Data: payload})
	if _, exists, err := sut.assetStore.GetAsset("node-revoked-heartbeat"); err != nil || exists {
		t.Fatalf("revoked heartbeat resurrected asset exists=%v err=%v", exists, err)
	}
	if sut.agentMgr.IsConnected("node-revoked-heartbeat") {
		t.Fatal("revoked live connection remained registered after heartbeat")
	}
}

func TestRevokedLiveAgentCannotSendOrDispatchNonHeartbeatMessages(t *testing.T) {
	t.Setenv("LABTETHER_AGENT_WS_CREDENTIAL_LEASE", "250ms")
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	transactions := sut.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	now := time.Now().UTC()
	_, enrollmentHash, _ := auth.GenerateSessionToken()
	rawAgent, agentHash, _ := auth.GenerateSessionToken()
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(enrollmentHash, "live-revoke", now.Add(time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	result, err := transactions.CommitAgentEnrollment(context.Background(), persistence.AgentEnrollmentCommitRequest{
		AssetID: "node-live-revoke", Hostname: "node-live-revoke", EnrollmentTokenHash: enrollmentHash,
		AgentTokenHash: agentHash, AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(sut.handleAgentWebSocket))
	defer server.Close()
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+rawAgent)
	headers.Set("X-Asset-ID", "node-live-revoke")
	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):], headers)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	deadline := time.Now().Add(2 * time.Second)
	for !sut.agentMgr.IsConnected("node-live-revoke") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	conn, ok := sut.agentMgr.Get("node-live-revoke")
	if !ok {
		t.Fatal("agent connection was not registered")
	}
	if err := sut.enrollmentStore.RevokeAgentToken(result.AgentToken.ID); err != nil {
		t.Fatal(err)
	}
	// This direct store mutation models revocation by another hub replica. The
	// local HTTP revocation path closes its matching socket immediately; remote
	// changes are deliberately bounded by the configured positive-cache lease.
	time.Sleep(300 * time.Millisecond)
	if err := conn.Send(agentmgr.Message{Type: agentmgr.MsgConfigUpdate}); !errors.Is(err, agentmgr.ErrAgentCredentialRejected) {
		t.Fatalf("revoked outbound send error=%v, want credential rejection", err)
	}
	logData, _ := json.Marshal(agentmgr.LogStreamData{Source: "agent", Level: "info", Message: "must-not-persist"})
	if err := client.WriteJSON(agentmgr.Message{Type: agentmgr.MsgLogStream, Data: logData}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for sut.agentMgr.IsConnected("node-live-revoke") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if sut.agentMgr.IsConnected("node-live-revoke") {
		t.Fatal("revoked socket remained registered after non-heartbeat message")
	}
	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		AssetID: "node-live-revoke", From: now.Add(-time.Minute), To: time.Now().UTC().Add(time.Minute), Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("revoked inbound log was persisted: %+v", events)
	}
}

func TestRevokedLiveAgentMalformedHeartbeatIsRejectedBeforeInnerDecode(t *testing.T) {
	t.Setenv("LABTETHER_AGENT_WS_CREDENTIAL_LEASE", "250ms")
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	transactions := sut.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	now := time.Now().UTC()
	_, enrollmentHash, _ := auth.GenerateSessionToken()
	rawAgent, agentHash, _ := auth.GenerateSessionToken()
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(enrollmentHash, "malformed-revoke", now.Add(time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	result, err := transactions.CommitAgentEnrollment(context.Background(), persistence.AgentEnrollmentCommitRequest{
		AssetID: "node-malformed-revoke", Hostname: "node-malformed-revoke", EnrollmentTokenHash: enrollmentHash,
		AgentTokenHash: agentHash, AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(sut.handleAgentWebSocket))
	defer server.Close()
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+rawAgent)
	headers.Set("X-Asset-ID", "node-malformed-revoke")
	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):], headers)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	deadline := time.Now().Add(2 * time.Second)
	for !sut.agentMgr.IsConnected("node-malformed-revoke") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if err := sut.enrollmentStore.RevokeAgentToken(result.AgentToken.ID); err != nil {
		t.Fatal(err)
	}
	// Direct persistence mutation represents cross-replica revocation, which
	// becomes authoritative once the bounded validation lease expires.
	time.Sleep(300 * time.Millisecond)
	if err := client.WriteJSON(agentmgr.Message{Type: agentmgr.MsgHeartbeat, Data: json.RawMessage(`{"metadata":"invalid"}`)}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for sut.agentMgr.IsConnected("node-malformed-revoke") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if sut.agentMgr.IsConnected("node-malformed-revoke") {
		t.Fatal("revoked malformed heartbeat reached the message handler")
	}
}

func TestAuthenticatedWebSocketHeartbeatCannotMoveAgentGroup(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	transactions := sut.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	now := time.Now().UTC()
	_, enrollmentHash, _ := auth.GenerateSessionToken()
	rawAgent, agentHash, _ := auth.GenerateSessionToken()
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(enrollmentHash, "group-bound", now.Add(time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	result, err := transactions.CommitAgentEnrollment(context.Background(), persistence.AgentEnrollmentCommitRequest{
		AssetID: "ws-group-bound", Hostname: "ws-group-bound", GroupID: "trusted-group",
		EnrollmentTokenHash: enrollmentHash, AgentTokenHash: agentHash, AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transactions.ValidateActiveAgentTokenID(context.Background(), result.AgentToken.ID, "ws-group-bound"); err != nil {
		t.Fatalf("seeded group-bound token invalid before connect: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "ws-group-bound", Type: "node", Name: "ws-group-bound", Source: "agent", GroupID: "trusted-group", Status: "offline",
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(sut.handleAgentWebSocket))
	defer server.Close()
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+rawAgent)
	headers.Set("X-Asset-ID", "ws-group-bound")
	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):], headers)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	heartbeat, _ := json.Marshal(agentmgr.HeartbeatData{
		Type: "node", Name: "ws-group-bound", Source: "manual", GroupID: "attacker-group", Status: "online", Platform: "linux",
	})
	if err := client.WriteJSON(agentmgr.Message{Type: agentmgr.MsgHeartbeat, Data: heartbeat}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stored, exists, err := sut.assetStore.GetAsset("ws-group-bound")
		if err != nil {
			t.Fatal(err)
		}
		if exists && stored.Status == "online" {
			if stored.GroupID != "trusted-group" {
				t.Fatalf("authenticated WS heartbeat moved group to %q", stored.GroupID)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("authenticated WS heartbeat was not persisted")
}

func TestHandleAgentWebSocketReconnectDoesNotEmitStaleDisconnect(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.authValidator = auth.NewTokenValidator("owner-token")
	sut.allowLegacySharedAgentAuth = true
	sut.agentMgr = agentmgr.NewManager()
	sut.broadcaster = newEventBroadcaster()
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-01", Type: "node", Name: "node-01", Source: "agent", Status: "online",
	}); err != nil {
		t.Fatalf("seed owner-token agent asset: %v", err)
	}

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
