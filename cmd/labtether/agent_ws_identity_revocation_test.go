package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(testEnrollmentTokenParams(enrollmentHash, "handshake", now.Add(time.Hour), 1)); err != nil {
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
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(testEnrollmentTokenParams(enrollmentHash, "heartbeat", now.Add(time.Hour), 1)); err != nil {
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
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(testEnrollmentTokenParams(enrollmentHash, "live-revoke", now.Add(time.Hour), 1)); err != nil {
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
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(testEnrollmentTokenParams(enrollmentHash, "malformed-revoke", now.Add(time.Hour), 1)); err != nil {
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
	trustedGroup, err := sut.groupStore.CreateGroup(groups.CreateRequest{Name: "Trusted", Slug: "trusted"})
	if err != nil {
		t.Fatal(err)
	}
	_, enrollmentHash, _ := auth.GenerateSessionToken()
	rawAgent, agentHash, _ := auth.GenerateSessionToken()
	if _, err := sut.enrollmentStore.CreateEnrollmentToken(testGroupEnrollmentTokenParams(enrollmentHash, trustedGroup.ID, now.Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	result, err := transactions.CommitAgentEnrollment(context.Background(), persistence.AgentEnrollmentCommitRequest{
		AssetID: "ws-group-bound", Hostname: "ws-group-bound", GroupID: trustedGroup.ID,
		EnrollmentTokenHash: enrollmentHash, AgentTokenHash: agentHash, AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transactions.ValidateActiveAgentTokenID(context.Background(), result.AgentToken.ID, "ws-group-bound"); err != nil {
		t.Fatalf("seeded group-bound token invalid before connect: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "ws-group-bound", Type: "node", Name: "ws-group-bound", Source: "agent", GroupID: trustedGroup.ID, Status: "offline",
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
			if stored.GroupID != trustedGroup.ID {
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
