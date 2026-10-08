package main

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
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

type browserEventEnvelope struct {
	Type string `json:"type"`
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
		agentmgr.MsgDockerEndpointTestResult,
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

func TestLegacyOwnerWebSocketCannotReconnectRetiredIdentity(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.authValidator = auth.NewTokenValidator("owner-token")
	sut.allowLegacySharedAgentAuth = true
	sut.agentMgr = agentmgr.NewManager()
	transactions := sut.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	now := time.Now().UTC()
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(testEnrollmentTokenParams("retired-ws-enrollment", "retired ws", now.Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := transactions.CommitAgentEnrollment(context.Background(), persistence.AgentEnrollmentCommitRequest{
		AssetID: "retired-ws-agent", Hostname: "retired-ws-agent",
		EnrollmentTokenHash: "retired-ws-enrollment", AgentTokenHash: "retired-ws-token",
		AgentTokenExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := transactions.DecommissionAgentAsset(context.Background(), "retired-ws-agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "retired-ws-agent", Type: "host", Name: "laundered", Source: "manual", Status: "online",
	}); !errors.Is(err, persistence.ErrAgentIdentityRetired) {
		t.Fatalf("generic heartbeat reused retired identity: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(sut.handleAgentWebSocket))
	defer server.Close()
	headers := http.Header{}
	headers.Set("Authorization", "Bearer owner-token")
	headers.Set("X-Asset-ID", "retired-ws-agent")
	client, response, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):], headers)
	if client != nil {
		_ = client.Close()
	}
	if err == nil {
		t.Fatal("retired identity opened a legacy owner WebSocket")
	}
	if response == nil || response.StatusCode != http.StatusConflict {
		statusCode := 0
		if response != nil {
			statusCode = response.StatusCode
		}
		t.Fatalf("retired owner WebSocket status=%d err=%v, want 409", statusCode, err)
	}
	if sut.agentMgr.IsConnected("retired-ws-agent") {
		t.Fatal("retired identity remained connected")
	}
	if err := sut.agentMgr.SendToAgent("retired-ws-agent", agentmgr.Message{Type: agentmgr.MsgCommandRequest}); err == nil {
		t.Fatal("retired identity remained usable as a command target")
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
