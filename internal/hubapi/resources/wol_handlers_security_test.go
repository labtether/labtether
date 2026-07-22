package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/servicehttp"
)

const testWakeMAC = "aa:bb:cc:dd:ee:ff"

type wolAuditRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *wolAuditRecorder) append(event audit.Event, _ string) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
}

func (r *wolAuditRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

func newWoLHandlerTestDeps() (*Deps, *wolAuditRecorder) {
	recorder := &wolAuditRecorder{}
	return &Deps{
		AssetStore:                 testutil.NewAssetStore(),
		EnforceRateLimit:           testutil.NoopRateLimit,
		PrincipalActorID:           func(context.Context) string { return "operator-1" },
		AppendAuditEventBestEffort: recorder.append,
		WoLPending:                 &WoLPendingRegistry{},
	}, recorder
}

func seedWoLAsset(t *testing.T, deps *Deps, id, platform string, metadata map[string]string) {
	t.Helper()
	if _, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  id,
		Type:     "host",
		Name:     id,
		Source:   "agent",
		Platform: platform,
		Metadata: metadata,
	}); err != nil {
		t.Fatalf("seed asset %q: %v", id, err)
	}
}

func invokeWake(deps *Deps, assetID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/assets/"+assetID+"/wake", nil)
	rec := httptest.NewRecorder()
	deps.HandleWakeOnLAN(rec, req, assetID)
	return rec
}

func requireWakeOutcome(t *testing.T, events []audit.Event, decision, reason string) audit.Event {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != "asset.wake.outcome" {
			continue
		}
		if events[i].Decision != decision || events[i].Reason != reason {
			t.Fatalf("wake outcome decision/reason = %q/%q, want %q/%q: %+v", events[i].Decision, events[i].Reason, decision, reason, events[i])
		}
		return events[i]
	}
	t.Fatalf("missing asset.wake.outcome event in %+v", events)
	return audit.Event{}
}

func requireAuditOmits(t *testing.T, events []audit.Event, forbidden ...string) {
	t.Helper()
	raw, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal audit events: %v", err)
	}
	serialized := strings.ToLower(string(raw))
	for _, value := range forbidden {
		if value != "" && strings.Contains(serialized, strings.ToLower(value)) {
			t.Fatalf("audit events exposed forbidden value %q: %s", value, serialized)
		}
	}
}

func TestHandleWakeOnLANRateLimitIsBoundedAndAudited(t *testing.T) {
	deps, recorder := newWoLHandlerTestDeps()
	seedWoLAsset(t, deps, "sleepy-node", "linux", map[string]string{"mac_address": testWakeMAC})

	var gotBucket string
	var gotLimit int
	var gotWindow time.Duration
	deps.EnforceRateLimit = func(w http.ResponseWriter, _ *http.Request, bucket string, limit int, window time.Duration) bool {
		gotBucket, gotLimit, gotWindow = bucket, limit, window
		servicehttp.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return false
	}

	rec := invokeWake(deps, "sleepy-node")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	second := invokeWake(deps, "sleepy-node")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429: %s", second.Code, second.Body.String())
	}
	if gotBucket != wakeRateLimitBucket || gotLimit != wakeRateLimitCount || gotWindow != wakeRateLimitWindow {
		t.Fatalf("rate limit = %q/%d/%s, want %q/%d/%s", gotBucket, gotLimit, gotWindow, wakeRateLimitBucket, wakeRateLimitCount, wakeRateLimitWindow)
	}

	events := recorder.snapshot()
	if len(events) != 1 || events[0].Type != "asset.wake.outcome" {
		t.Fatalf("unexpected audit sequence: %+v", events)
	}
	requireWakeOutcome(t, events, "denied", "rate_limited")
	requireAuditOmits(t, events, testWakeMAC)
}

func TestHandleWakeOnLANDirectSuccessAuditsSanitizedOutcome(t *testing.T) {
	deps, recorder := newWoLHandlerTestDeps()
	seedWoLAsset(t, deps, "sleepy-node", "linux", map[string]string{"mac_address": testWakeMAC})

	originalSend := SendWakeOnLAN
	t.Cleanup(func() { SendWakeOnLAN = originalSend })
	var gotMAC, gotBroadcast string
	SendWakeOnLAN = func(mac net.HardwareAddr, broadcast string) error {
		gotMAC, gotBroadcast = mac.String(), broadcast
		return nil
	}

	rec := invokeWake(deps, "sleepy-node")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if gotMAC != testWakeMAC || gotBroadcast != "255.255.255.255:9" {
		t.Fatalf("direct send = %q via %q", gotMAC, gotBroadcast)
	}
	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["status"] != "sent" || response["method"] != "direct" {
		t.Fatalf("unexpected response: %+v", response)
	}

	events := recorder.snapshot()
	outcome := requireWakeOutcome(t, events, "succeeded", "")
	if outcome.ActorID != "operator-1" || outcome.Target != "sleepy-node" || outcome.Details["method"] != "direct" {
		t.Fatalf("unexpected direct outcome: %+v", outcome)
	}
	requireAuditOmits(t, events, testWakeMAC)
}

func TestHandleWakeOnLANDirectFailureIsActionableAndDoesNotLeak(t *testing.T) {
	deps, recorder := newWoLHandlerTestDeps()
	seedWoLAsset(t, deps, "sleepy-node", "linux", map[string]string{"mac_address": testWakeMAC})

	originalSend := SendWakeOnLAN
	t.Cleanup(func() { SendWakeOnLAN = originalSend })
	SendWakeOnLAN = func(net.HardwareAddr, string) error {
		return errors.New("dial failed token=supersecret mac=" + testWakeMAC)
	}

	var logs bytes.Buffer
	originalLogWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(originalLogWriter) })

	rec := invokeWake(deps, "sleepy-node")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", rec.Code, rec.Body.String())
	}
	body := strings.ToLower(rec.Body.String())
	if !strings.Contains(strings.ToLower(logs.String()), "verify udp broadcast access") {
		t.Fatalf("sanitized operator log is not actionable: %s", logs.String())
	}
	for _, forbidden := range []string{"supersecret", testWakeMAC} {
		if strings.Contains(body, forbidden) || strings.Contains(strings.ToLower(logs.String()), forbidden) {
			t.Fatalf("failure leaked %q; body=%s logs=%s", forbidden, rec.Body.String(), logs.String())
		}
	}

	events := recorder.snapshot()
	requireWakeOutcome(t, events, "failed", "direct_send_failed")
	requireAuditOmits(t, events, "supersecret", testWakeMAC)
}

func TestHandleWakeOnLANRejectsMissingAndMalformedMACWithoutEchoingMetadata(t *testing.T) {
	tests := []struct {
		name       string
		metadata   map[string]string
		wantReason string
		forbidden  string
	}{
		{name: "missing", metadata: nil, wantReason: "mac_missing"},
		{name: "malformed", metadata: map[string]string{"mac_address": "token=supersecret"}, wantReason: "mac_invalid", forbidden: "supersecret"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps, recorder := newWoLHandlerTestDeps()
			seedWoLAsset(t, deps, "sleepy-node", "linux", tc.metadata)

			rec := invokeWake(deps, "sleepy-node")
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body.String())
			}
			if tc.forbidden != "" && strings.Contains(strings.ToLower(rec.Body.String()), tc.forbidden) {
				t.Fatalf("response echoed malformed metadata: %s", rec.Body.String())
			}
			events := recorder.snapshot()
			requireWakeOutcome(t, events, "denied", tc.wantReason)
			requireAuditOmits(t, events, tc.forbidden)
		})
	}
}

func TestHandleWakeOnLANRelayResultIsBoundToRequestRelayAndMAC(t *testing.T) {
	deps, recorder := newWoLHandlerTestDeps()
	deps.AgentMgr = agentmgr.NewManager()
	seedWoLAsset(t, deps, "target-node", "linux", map[string]string{"mac_address": testWakeMAC})
	seedWoLAsset(t, deps, "relay-node", "linux", nil)

	serverConn, clientConn, cleanup := createWoLWebSocketPair(t)
	defer cleanup()
	relayConn := agentmgr.NewAgentConn(serverConn, "relay-node", "linux")
	deps.AgentMgr.Register(relayConn)
	defer deps.AgentMgr.Unregister("relay-node")

	originalSend := SendWakeOnLAN
	t.Cleanup(func() { SendWakeOnLAN = originalSend })
	SendWakeOnLAN = func(net.HardwareAddr, string) error {
		t.Fatal("direct fallback should not run when relay dispatch succeeds")
		return nil
	}

	rec := invokeWake(deps, "target-node")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["status"] != "queued" || response["method"] != "agent-assisted" {
		t.Fatalf("unexpected relay response: %+v", response)
	}

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var outbound agentmgr.Message
	if err := clientConn.ReadJSON(&outbound); err != nil {
		t.Fatalf("read relay request: %v", err)
	}
	var payload agentmgr.WoLSendData
	if err := json.Unmarshal(outbound.Data, &payload); err != nil {
		t.Fatalf("decode relay request: %v", err)
	}
	if outbound.ID != "target-node" || payload.RequestID == "" || payload.MAC != testWakeMAC {
		t.Fatalf("unexpected relay request: message=%+v payload=%+v", outbound, payload)
	}
	firstRequestID := payload.RequestID

	resultData, err := json.Marshal(agentmgr.WoLResultData{
		RequestID: payload.RequestID,
		MAC:       payload.MAC,
		OK:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	resultMessage := agentmgr.Message{ID: payload.RequestID, Data: resultData}

	deps.ProcessAgentWoLResult(&agentmgr.AgentConn{AssetID: "different-relay"}, resultMessage)
	events := recorder.snapshot()
	if last := events[len(events)-1]; last.Decision != "rejected" || last.Reason != "relay_mismatch" || last.Target != "target-node" {
		t.Fatalf("cross-relay result was not rejected: %+v", last)
	}
	wrongMACData, err := json.Marshal(agentmgr.WoLResultData{
		RequestID: payload.RequestID,
		MAC:       "11:22:33:44:55:66",
		OK:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	deps.ProcessAgentWoLResult(relayConn, agentmgr.Message{ID: payload.RequestID, Data: wrongMACData})
	events = recorder.snapshot()
	if last := events[len(events)-1]; last.Decision != "rejected" || last.Reason != "mac_mismatch" || last.Target != "target-node" {
		t.Fatalf("wrong-MAC result was not rejected: %+v", last)
	}

	deps.ProcessAgentWoLResult(relayConn, resultMessage)
	events = recorder.snapshot()
	if last := events[len(events)-1]; last.Decision != "succeeded" || last.Reason != "" || last.Target != "target-node" || last.ActorID != "relay-node" {
		t.Fatalf("correlated relay result was not audited: %+v", last)
	}

	deps.ProcessAgentWoLResult(relayConn, resultMessage)
	events = recorder.snapshot()
	if last := events[len(events)-1]; last.Decision != "rejected" || last.Reason != "unknown_or_replayed_request" {
		t.Fatalf("replayed relay result was not rejected: %+v", last)
	}
	auditCountAfterReplay := len(events)
	deps.ProcessAgentWoLResult(relayConn, resultMessage)
	if got := len(recorder.snapshot()); got != auditCountAfterReplay {
		t.Fatalf("duplicate replay audit was not throttled: got %d events, want %d", got, auditCountAfterReplay)
	}

	rec = invokeWake(deps, "target-node")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("second relay status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := clientConn.ReadJSON(&outbound); err != nil {
		t.Fatalf("read second relay request: %v", err)
	}
	if err := json.Unmarshal(outbound.Data, &payload); err != nil {
		t.Fatalf("decode second relay request: %v", err)
	}
	failureData, err := json.Marshal(agentmgr.WoLResultData{
		RequestID: payload.RequestID,
		MAC:       payload.MAC,
		OK:        false,
		Error:     "authorization=Bearer supersecret",
	})
	if err != nil {
		t.Fatal(err)
	}
	deps.ProcessAgentWoLResult(relayConn, agentmgr.Message{ID: payload.RequestID, Data: failureData})
	events = recorder.snapshot()
	if last := events[len(events)-1]; last.Decision != "failed" || last.Reason != "relay_send_failed" || last.Target != "target-node" {
		t.Fatalf("relay failure was not safely audited: %+v", last)
	}
	requireAuditOmits(t, events, testWakeMAC, "11:22:33:44:55:66", firstRequestID, payload.RequestID, "supersecret", "authorization")
}

func TestProcessAgentWoLResultRejectsMalformedUncorrelatedAndSecretBearingResults(t *testing.T) {
	deps, recorder := newWoLHandlerTestDeps()
	relayConn := &agentmgr.AgentConn{AssetID: "relay-node"}

	deps.ProcessAgentWoLResult(relayConn, agentmgr.Message{Data: []byte(`{"mac":`)})

	resultData, err := json.Marshal(agentmgr.WoLResultData{
		RequestID: "secret-request-token",
		MAC:       testWakeMAC,
		OK:        false,
		Error:     "authorization=Bearer supersecret",
	})
	if err != nil {
		t.Fatal(err)
	}
	deps.ProcessAgentWoLResult(relayConn, agentmgr.Message{ID: "different-request", Data: resultData})
	deps.ProcessAgentWoLResult(relayConn, agentmgr.Message{ID: "secret-request-token", Data: resultData})

	events := recorder.snapshot()
	if len(events) != 3 {
		t.Fatalf("audit events = %d, want 3: %+v", len(events), events)
	}
	if events[0].Reason != "invalid_payload" || events[1].Reason != "request_id_mismatch" || events[2].Reason != "unknown_or_replayed_request" {
		t.Fatalf("unexpected rejection reasons: %+v", events)
	}
	requireAuditOmits(t, events, testWakeMAC, "supersecret", "secret-request-token", "authorization")
}

func TestHandleWakeOnLANBoundsFailedRelayFanout(t *testing.T) {
	deps, recorder := newWoLHandlerTestDeps()
	deps.AgentMgr = agentmgr.NewManager()
	seedWoLAsset(t, deps, "target-node", "linux", map[string]string{"mac_address": testWakeMAC})
	for i := 0; i < 5; i++ {
		seedWoLAsset(t, deps, "relay-"+string(rune('a'+i)), "linux", nil)
	}

	serverConn, clientConn, cleanup := createWoLWebSocketPair(t)
	_ = serverConn.Close()
	_ = clientConn.Close()
	defer cleanup()
	for i := 0; i < 5; i++ {
		id := "relay-" + string(rune('a'+i))
		deps.AgentMgr.Register(agentmgr.NewAgentConn(serverConn, id, "linux"))
		defer deps.AgentMgr.Unregister(id)
	}

	originalSend := SendWakeOnLAN
	t.Cleanup(func() { SendWakeOnLAN = originalSend })
	directCalls := 0
	SendWakeOnLAN = func(net.HardwareAddr, string) error {
		directCalls++
		return nil
	}

	rec := invokeWake(deps, "target-node")
	if rec.Code != http.StatusAccepted || directCalls != 1 {
		t.Fatalf("fallback status/calls = %d/%d: %s", rec.Code, directCalls, rec.Body.String())
	}

	events := recorder.snapshot()
	relayFailures := 0
	for _, event := range events {
		if event.Type == "asset.wake.relay_dispatch" && event.Reason == "relay_unavailable" {
			relayFailures++
		}
	}
	if relayFailures != maxWoLRelayAttempts {
		t.Fatalf("relay dispatch failures = %d, want bounded %d: %+v", relayFailures, maxWoLRelayAttempts, events)
	}
	outcome := requireWakeOutcome(t, events, "succeeded", "")
	if outcome.Details["relay_attempts"] != maxWoLRelayAttempts {
		t.Fatalf("relay_attempts = %#v, want %d", outcome.Details["relay_attempts"], maxWoLRelayAttempts)
	}
	requireAuditOmits(t, events, testWakeMAC)
}

func TestWoLPendingRegistryIsBoundedAndOneTime(t *testing.T) {
	registry := &WoLPendingRegistry{}
	for i := 0; i < maxPendingWoLRelay; i++ {
		requestID := "request-" + strconv.Itoa(i)
		if !registry.store(requestID, pendingWoLRelay{
			TargetID:    "target",
			RelayID:     "relay",
			ExpectedMAC: testWakeMAC,
			ExpiresAt:   time.Now().UTC().Add(time.Minute),
		}) {
			t.Fatalf("registry rejected entry %d before reaching bound", i)
		}
	}
	if registry.store("overflow", pendingWoLRelay{ExpiresAt: time.Now().UTC().Add(time.Minute)}) {
		t.Fatal("registry accepted an entry past its bound")
	}

	requestID := "request-0"
	if _, status := registry.consume(requestID, "wrong-relay", testWakeMAC); status != wolPendingRelayMismatch {
		t.Fatalf("wrong relay status = %v", status)
	}
	if _, status := registry.consume(requestID, "relay", "11:22:33:44:55:66"); status != wolPendingMACMismatch {
		t.Fatalf("wrong MAC status = %v", status)
	}
	if _, status := registry.consume(requestID, "relay", testWakeMAC); status != wolPendingConsumed {
		t.Fatalf("correct result status = %v", status)
	}
	if _, status := registry.consume(requestID, "relay", testWakeMAC); status != wolPendingUnknown {
		t.Fatalf("replay status = %v", status)
	}
	if !registry.store("expired", pendingWoLRelay{
		TargetID:    "target",
		RelayID:     "relay",
		ExpectedMAC: testWakeMAC,
		ExpiresAt:   time.Now().UTC().Add(-time.Second),
	}) {
		t.Fatal("registry rejected expired test fixture")
	}
	if _, status := registry.consume("expired", "relay", testWakeMAC); status != wolPendingExpired {
		t.Fatalf("expired result status = %v", status)
	}
}

func createWoLWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()
	serverConnCh := make(chan *websocket.Conn, 1)
	done := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		serverConnCh <- conn
		<-done
	}))

	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		close(done)
		server.Close()
		t.Fatalf("dial websocket: %v", err)
	}
	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(2 * time.Second):
		_ = clientConn.Close()
		close(done)
		server.Close()
		t.Fatal("timed out waiting for server websocket")
	}

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			close(done)
			_ = clientConn.Close()
			_ = serverConn.Close()
			server.Close()
		})
	}
	return serverConn, clientConn, cleanup
}
