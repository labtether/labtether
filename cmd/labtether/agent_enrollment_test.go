package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentidentity"
	"github.com/labtether/labtether/internal/agentmgr"
)

func TestBuildPendingEnrollmentAssetID(t *testing.T) {
	id := buildPendingEnrollmentAssetID(" Lab Host/01 ")
	if !strings.HasPrefix(id, "pending-lab-host-01-") {
		t.Fatalf("expected normalized host prefix, got %q", id)
	}

	unknownID := buildPendingEnrollmentAssetID(" !!! ")
	if !strings.HasPrefix(unknownID, "pending-unknown-") {
		t.Fatalf("expected unknown host fallback, got %q", unknownID)
	}

	longHostID := buildPendingEnrollmentAssetID(strings.Repeat("X", 200))
	if !strings.HasPrefix(longHostID, "pending-"+strings.Repeat("x", maxPendingHostnameIDLen)+"-") {
		t.Fatalf("expected long hostname to be truncated, got %q", longHostID)
	}
	if len(longHostID) > 100 {
		t.Fatalf("expected bounded pending asset id length, got %d (%q)", len(longHostID), longHostID)
	}

	seen := make(map[string]struct{}, 64)
	for i := 0; i < 64; i++ {
		candidate := buildPendingEnrollmentAssetID("alpha")
		if _, exists := seen[candidate]; exists {
			t.Fatalf("expected unique pending asset ids, duplicate %q", candidate)
		}
		seen[candidate] = struct{}{}
	}
}

func createWSPairForPendingEnrollmentTest(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	serverConnCh := make(chan *websocket.Conn, 1)
	done := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConnCh <- conn
		<-done
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		close(done)
		server.Close()
		t.Fatalf("dial failed: %v", err)
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

	cleanup := func() {
		close(done)
		_ = clientConn.Close()
		_ = serverConn.Close()
		server.Close()
	}
	return serverConn, clientConn, cleanup
}

// TestPendingAgents exercises Add, List, Get, and Remove on the registry.
func TestPendingAgents(t *testing.T) {
	t.Run("empty registry returns empty list", func(t *testing.T) {
		p := newPendingAgents()
		got := p.List()
		if len(got) != 0 {
			t.Fatalf("expected empty list, got %d entries", len(got))
		}
	})

	t.Run("Add and Get", func(t *testing.T) {
		p := newPendingAgents()
		agent := &pendingAgent{
			AssetID:     "pending-my-host-123",
			Hostname:    "my-host",
			Platform:    "linux",
			RemoteIP:    "192.168.1.100",
			ConnectedAt: time.Now().UTC(),
		}
		p.Add(agent)

		got, ok := p.Get("pending-my-host-123")
		if !ok {
			t.Fatal("expected to find agent after Add")
		}
		if got.AssetID != agent.AssetID {
			t.Fatalf("expected asset_id %q, got %q", agent.AssetID, got.AssetID)
		}
		if got.Hostname != agent.Hostname {
			t.Fatalf("expected hostname %q, got %q", agent.Hostname, got.Hostname)
		}
		if got.Platform != agent.Platform {
			t.Fatalf("expected platform %q, got %q", agent.Platform, got.Platform)
		}
	})

	t.Run("Get returns false for unknown asset_id", func(t *testing.T) {
		p := newPendingAgents()
		_, ok := p.Get("does-not-exist")
		if ok {
			t.Fatal("expected ok=false for unknown asset_id")
		}
	})

	t.Run("List returns all added agents", func(t *testing.T) {
		p := newPendingAgents()
		p.Add(&pendingAgent{AssetID: "pending-host-a-1", Hostname: "host-a", Platform: "linux", ConnectedAt: time.Now().UTC()})
		p.Add(&pendingAgent{AssetID: "pending-host-b-2", Hostname: "host-b", Platform: "darwin", ConnectedAt: time.Now().UTC()})

		list := p.List()
		if len(list) != 2 {
			t.Fatalf("expected 2 agents in list, got %d", len(list))
		}

		// Build a set of returned asset IDs for easy lookup.
		ids := make(map[string]bool, len(list))
		for _, a := range list {
			ids[a.AssetID] = true
		}
		if !ids["pending-host-a-1"] {
			t.Error("missing pending-host-a-1 in list")
		}
		if !ids["pending-host-b-2"] {
			t.Error("missing pending-host-b-2 in list")
		}
	})

	t.Run("Count and CountByRemoteIP", func(t *testing.T) {
		p := newPendingAgents()
		p.Add(&pendingAgent{AssetID: "pending-host-a-1", Hostname: "host-a", RemoteIP: "10.0.0.1", ConnectedAt: time.Now().UTC()})
		p.Add(&pendingAgent{AssetID: "pending-host-b-2", Hostname: "host-b", RemoteIP: "10.0.0.1", ConnectedAt: time.Now().UTC()})
		p.Add(&pendingAgent{AssetID: "pending-host-c-3", Hostname: "host-c", RemoteIP: "10.0.0.2", ConnectedAt: time.Now().UTC()})

		if got := p.Count(); got != 3 {
			t.Fatalf("expected pending count=3, got %d", got)
		}
		if got := p.CountByRemoteIP("10.0.0.1"); got != 2 {
			t.Fatalf("expected pending count by IP=2, got %d", got)
		}
		if got := p.CountByRemoteIP("10.0.0.2"); got != 1 {
			t.Fatalf("expected pending count by IP=1, got %d", got)
		}
		if got := p.CountByRemoteIP("10.0.0.3"); got != 0 {
			t.Fatalf("expected pending count by IP=0, got %d", got)
		}
	})

	t.Run("Remove deletes agent from registry", func(t *testing.T) {
		p := newPendingAgents()
		p.Add(&pendingAgent{AssetID: "pending-host-x-9", Hostname: "host-x", ConnectedAt: time.Now().UTC()})

		p.Remove("pending-host-x-9")

		_, ok := p.Get("pending-host-x-9")
		if ok {
			t.Fatal("expected agent to be removed")
		}
		if len(p.List()) != 0 {
			t.Fatalf("expected empty list after Remove, got %d entries", len(p.List()))
		}
	})

	t.Run("Remove on unknown ID is a no-op", func(t *testing.T) {
		p := newPendingAgents()
		p.Add(&pendingAgent{AssetID: "pending-host-y-1", Hostname: "host-y", ConnectedAt: time.Now().UTC()})

		// Removing a non-existent ID must not panic and must not affect other entries.
		p.Remove("pending-does-not-exist")

		if len(p.List()) != 1 {
			t.Fatalf("expected 1 remaining agent, got %d", len(p.List()))
		}
	})

	t.Run("List does not expose conn field in JSON", func(t *testing.T) {
		p := newPendingAgents()
		p.Add(&pendingAgent{AssetID: "pending-host-z-5", Hostname: "host-z", ConnectedAt: time.Now().UTC()})

		list := p.List()
		if len(list) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(list))
		}

		b, err := json.Marshal(list[0])
		if err != nil {
			t.Fatalf("failed to marshal pendingAgentInfo: %v", err)
		}

		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if _, exists := m["conn"]; exists {
			t.Error("conn field must not appear in JSON output")
		}
		if m["asset_id"] != "pending-host-z-5" {
			t.Errorf("expected asset_id=pending-host-z-5, got %v", m["asset_id"])
		}
	})
}

// TestHandleListPendingAgents tests the GET /api/v1/agents/pending handler.
func TestHandleListPendingAgents(t *testing.T) {
	t.Run("returns empty list when no pending agents", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/pending", nil)
		rec := httptest.NewRecorder()
		sut.handleListPendingAgents(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["count"] != float64(0) {
			t.Fatalf("expected count=0, got %v", resp["count"])
		}
		agents, ok := resp["agents"].([]any)
		if !ok {
			t.Fatalf("expected agents to be a list, got %T", resp["agents"])
		}
		if len(agents) != 0 {
			t.Fatalf("expected empty agents list, got %d entries", len(agents))
		}
	})

	t.Run("returns populated list", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		now := time.Now().UTC()
		sut.pendingAgents.Add(&pendingAgent{
			AssetID:     "pending-alpha-1000",
			Hostname:    "alpha",
			Platform:    "linux",
			RemoteIP:    "10.0.0.1",
			ConnectedAt: now,
		})
		sut.pendingAgents.Add(&pendingAgent{
			AssetID:     "pending-beta-2000",
			Hostname:    "beta",
			Platform:    "darwin",
			RemoteIP:    "10.0.0.2",
			ConnectedAt: now,
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/pending", nil)
		rec := httptest.NewRecorder()
		sut.handleListPendingAgents(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["count"] != float64(2) {
			t.Fatalf("expected count=2, got %v", resp["count"])
		}
		agents, ok := resp["agents"].([]any)
		if !ok {
			t.Fatalf("expected agents to be a list, got %T", resp["agents"])
		}
		if len(agents) != 2 {
			t.Fatalf("expected 2 agents, got %d", len(agents))
		}

		// Verify neither entry exposes the conn field.
		for _, raw := range agents {
			entry, ok := raw.(map[string]any)
			if !ok {
				t.Fatal("each agent entry should be a JSON object")
			}
			if _, exists := entry["conn"]; exists {
				t.Error("conn field must not appear in list response")
			}
			if entry["asset_id"] == "" {
				t.Error("expected asset_id to be set")
			}
		}
	})

	t.Run("rejects non-GET method", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/pending", nil)
		rec := httptest.NewRecorder()
		sut.handleListPendingAgents(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandlePendingEnrollmentGuards(t *testing.T) {
	t.Run("rejects when pending capacity is full", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()
		for i := 0; i < maxPendingEnrollmentAgents; i++ {
			sut.pendingAgents.Add(&pendingAgent{
				AssetID:  buildPendingEnrollmentAssetID("host"),
				Hostname: "host",
			})
		}

		req := httptest.NewRequest(http.MethodGet, "/ws/agent", nil)
		req.RemoteAddr = "10.0.0.10:1234"
		rec := httptest.NewRecorder()
		sut.handlePendingEnrollment(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
	})

	t.Run("rejects when source IP exceeds pending connection limit", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()
		for i := 0; i < maxPendingEnrollmentPerIP; i++ {
			sut.pendingAgents.Add(&pendingAgent{
				AssetID:  buildPendingEnrollmentAssetID("host"),
				Hostname: "host",
				RemoteIP: "10.0.0.20",
			})
		}

		req := httptest.NewRequest(http.MethodGet, "/ws/agent", nil)
		req.RemoteAddr = "10.0.0.20:9999"
		rec := httptest.NewRecorder()
		sut.handlePendingEnrollment(rec, req)

		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d", rec.Code)
		}
	})
}

func TestHandlePendingEnrollmentSendsChallengeAndCleansUpOnDisconnect(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handlePendingEnrollment(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Set("X-Hostname", "pending-node")
	headers.Set("X-Platform", "linux")
	headers.Set("X-Device-Key-Alg", "ed25519")
	headers.Set("X-Device-Public-Key", "device-public-key")
	headers.Set("X-Device-Fingerprint", "device-fingerprint")

	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer clientConn.Close()

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := clientConn.ReadJSON(&msg); err != nil {
		t.Fatalf("read challenge message: %v", err)
	}
	if msg.Type != agentmgr.MsgEnrollmentChallenge {
		t.Fatalf("expected enrollment.challenge, got %q", msg.Type)
	}

	var challenge agentmgr.EnrollmentChallengeData
	if err := json.Unmarshal(msg.Data, &challenge); err != nil {
		t.Fatalf("decode challenge payload: %v", err)
	}
	if !strings.HasPrefix(challenge.ConnectionID, "pending-pending-node-") {
		t.Fatalf("expected generated pending asset id, got %q", challenge.ConnectionID)
	}
	if strings.TrimSpace(challenge.Nonce) == "" {
		t.Fatalf("expected nonce in challenge payload")
	}

	if sut.pendingAgents.Count() != 1 {
		t.Fatalf("expected one pending agent after connect, got %d", sut.pendingAgents.Count())
	}

	if err := clientConn.Close(); err != nil {
		t.Fatalf("close client Conn: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sut.pendingAgents.Count() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected pending agent to be removed after disconnect")
}

func TestHandlePendingEnrollmentAcceptsLargeValidProof(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handlePendingEnrollment(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Set("X-Hostname", "pending-node")
	headers.Set("X-Platform", "linux")

	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer clientConn.Close()

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := clientConn.ReadJSON(&msg); err != nil {
		t.Fatalf("read challenge message: %v", err)
	}
	if msg.Type != agentmgr.MsgEnrollmentChallenge {
		t.Fatalf("expected enrollment.challenge, got %q", msg.Type)
	}

	var challenge agentmgr.EnrollmentChallengeData
	if err := json.Unmarshal(msg.Data, &challenge); err != nil {
		t.Fatalf("decode challenge payload: %v", err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
	signature := ed25519.Sign(privateKey, agentidentity.BuildEnrollmentProofPayload(challenge.ConnectionID, challenge.Nonce, fingerprint))

	proofEnvelope := map[string]any{
		"connection_id": challenge.ConnectionID,
		"nonce":         challenge.Nonce,
		"key_algorithm": agentidentity.KeyAlgorithmEd25519,
		"public_key":    base64.StdEncoding.EncodeToString(publicKey),
		"fingerprint":   fingerprint,
		"signature":     base64.StdEncoding.EncodeToString(signature),
		"padding":       strings.Repeat("x", 1500),
	}
	rawProof, err := json.Marshal(proofEnvelope)
	if err != nil {
		t.Fatalf("marshal proof envelope: %v", err)
	}
	rawMsg, err := json.Marshal(agentmgr.Message{
		Type: agentmgr.MsgEnrollmentProof,
		Data: rawProof,
	})
	if err != nil {
		t.Fatalf("marshal websocket message: %v", err)
	}
	if len(rawMsg) <= 1024 {
		t.Fatalf("expected proof message to exceed previous 1024-byte limit, got %d", len(rawMsg))
	}

	if err := clientConn.WriteMessage(websocket.TextMessage, rawMsg); err != nil {
		t.Fatalf("write large proof message: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sut.pendingAgents.IsIdentityVerified(challenge.ConnectionID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	_, ok := sut.pendingAgents.Get(challenge.ConnectionID)
	if !ok {
		t.Fatalf("expected pending agent %q to remain connected", challenge.ConnectionID)
	}
	t.Fatalf("expected pending agent %q to verify with large proof payload", challenge.ConnectionID)
}

func TestHandlePendingEnrollmentTimesOutAndCleansUp(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()

	type scheduledTimeout struct {
		duration time.Duration
		fn       func()
	}
	timeoutFnCh := make(chan scheduledTimeout, 1)
	prevAfterFunc := agentspkg.PendingEnrollmentAfterFunc
	agentspkg.PendingEnrollmentAfterFunc = func(d time.Duration, fn func()) *time.Timer {
		timeoutFnCh <- scheduledTimeout{duration: d, fn: fn}
		return time.NewTimer(time.Hour)
	}
	defer func() {
		agentspkg.PendingEnrollmentAfterFunc = prevAfterFunc
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handlePendingEnrollment(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Set("X-Hostname", "timed-out-node")
	headers.Set("X-Platform", "linux")

	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer clientConn.Close()

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := clientConn.ReadJSON(&msg); err != nil {
		t.Fatalf("read challenge message: %v", err)
	}
	if msg.Type != agentmgr.MsgEnrollmentChallenge {
		t.Fatalf("expected enrollment.challenge, got %q", msg.Type)
	}

	if sut.pendingAgents.Count() != 1 {
		t.Fatalf("expected one pending agent after connect, got %d", sut.pendingAgents.Count())
	}

	var scheduled scheduledTimeout
	select {
	case scheduled = <-timeoutFnCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pending enrollment timeout callback")
	}
	if scheduled.duration != maxPendingEnrollmentTimeout {
		t.Fatalf("expected timeout duration %s, got %s", maxPendingEnrollmentTimeout, scheduled.duration)
	}
	scheduled.fn()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sut.pendingAgents.Count() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected pending agent to be removed after timeout")
}

func TestVerifyPendingEnrollmentProof(t *testing.T) {
	t.Run("accepts valid signed proof", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)

		agent := &pendingAgent{
			AssetID:            "pending-alpha-1",
			Hostname:           "alpha",
			ChallengeNonce:     "nonce-123",
			ChallengeExpiresAt: time.Now().UTC().Add(time.Minute),
		}
		sut.pendingAgents.Add(agent)

		payload := agentidentity.BuildEnrollmentProofPayload(agent.AssetID, agent.ChallengeNonce, fingerprint)
		signature := ed25519.Sign(privateKey, payload)
		proof := agentmgr.EnrollmentProofData{
			ConnectionID: agent.AssetID,
			Nonce:        agent.ChallengeNonce,
			KeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
			PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
			Fingerprint:  fingerprint,
			Signature:    base64.StdEncoding.EncodeToString(signature),
		}
		raw, _ := json.Marshal(proof)

		if err := sut.verifyPendingEnrollmentProof(agent, agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: raw}); err != nil {
			t.Fatalf("expected proof verification success, got %v", err)
		}
		if !agent.IdentityVerified {
			t.Fatalf("expected identity_verified=true")
		}
		if agent.DeviceFingerprint != fingerprint {
			t.Fatalf("expected fingerprint %q, got %q", fingerprint, agent.DeviceFingerprint)
		}
		if agent.IdentityVerifiedAt == nil {
			t.Fatalf("expected identity_verified_at to be set")
		}
	})

	t.Run("rejects invalid signature", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		publicKey, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
		agent := &pendingAgent{
			AssetID:            "pending-bravo-1",
			Hostname:           "bravo",
			ChallengeNonce:     "nonce-456",
			ChallengeExpiresAt: time.Now().UTC().Add(time.Minute),
		}
		sut.pendingAgents.Add(agent)

		invalidSignature := make([]byte, ed25519.SignatureSize)
		if _, err := rand.Read(invalidSignature); err != nil {
			t.Fatalf("rand signature: %v", err)
		}
		proof := agentmgr.EnrollmentProofData{
			ConnectionID: agent.AssetID,
			Nonce:        agent.ChallengeNonce,
			KeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
			PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
			Fingerprint:  fingerprint,
			Signature:    base64.StdEncoding.EncodeToString(invalidSignature),
		}
		raw, _ := json.Marshal(proof)

		if err := sut.verifyPendingEnrollmentProof(agent, agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: raw}); err == nil {
			t.Fatalf("expected invalid signature rejection")
		}
		if agent.IdentityVerified {
			t.Fatalf("expected identity_verified=false")
		}
	})

	t.Run("rejects connection_id mismatch", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
		agent := &pendingAgent{
			AssetID:            "pending-charlie-1",
			ChallengeNonce:     "nonce-789",
			ChallengeExpiresAt: time.Now().UTC().Add(time.Minute),
		}
		sut.pendingAgents.Add(agent)

		payload := agentidentity.BuildEnrollmentProofPayload("pending-other-1", agent.ChallengeNonce, fingerprint)
		signature := ed25519.Sign(privateKey, payload)
		proof := agentmgr.EnrollmentProofData{
			ConnectionID: "pending-other-1",
			Nonce:        agent.ChallengeNonce,
			KeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
			PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
			Fingerprint:  fingerprint,
			Signature:    base64.StdEncoding.EncodeToString(signature),
		}
		raw, _ := json.Marshal(proof)

		if err := sut.verifyPendingEnrollmentProof(agent, agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: raw}); err == nil {
			t.Fatalf("expected connection_id mismatch rejection")
		}
	})

	t.Run("rejects expired challenge", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
		agent := &pendingAgent{
			AssetID:            "pending-delta-1",
			ChallengeNonce:     "nonce-expired",
			ChallengeExpiresAt: time.Now().UTC().Add(-time.Minute),
		}
		sut.pendingAgents.Add(agent)

		payload := agentidentity.BuildEnrollmentProofPayload(agent.AssetID, agent.ChallengeNonce, fingerprint)
		signature := ed25519.Sign(privateKey, payload)
		proof := agentmgr.EnrollmentProofData{
			ConnectionID: agent.AssetID,
			Nonce:        agent.ChallengeNonce,
			KeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
			PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
			Fingerprint:  fingerprint,
			Signature:    base64.StdEncoding.EncodeToString(signature),
		}
		raw, _ := json.Marshal(proof)

		if err := sut.verifyPendingEnrollmentProof(agent, agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: raw}); err == nil {
			t.Fatalf("expected expired challenge rejection")
		}
	})

	t.Run("rejects fingerprint mismatch", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
		agent := &pendingAgent{
			AssetID:            "pending-echo-1",
			ChallengeNonce:     "nonce-fingerprint",
			ChallengeExpiresAt: time.Now().UTC().Add(time.Minute),
		}
		sut.pendingAgents.Add(agent)

		payload := agentidentity.BuildEnrollmentProofPayload(agent.AssetID, agent.ChallengeNonce, fingerprint)
		signature := ed25519.Sign(privateKey, payload)
		proof := agentmgr.EnrollmentProofData{
			ConnectionID: agent.AssetID,
			Nonce:        agent.ChallengeNonce,
			KeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
			PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
			Fingerprint:  "sha256:wrong",
			Signature:    base64.StdEncoding.EncodeToString(signature),
		}
		raw, _ := json.Marshal(proof)

		if err := sut.verifyPendingEnrollmentProof(agent, agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: raw}); err == nil {
			t.Fatalf("expected fingerprint mismatch rejection")
		}
	})
}

func TestHandleApproveAgentRequiresIdentityVerified(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()
	sut.pendingAgents.Add(&pendingAgent{
		AssetID:          "pending-charlie-1",
		Hostname:         "charlie",
		Platform:         "linux",
		IdentityVerified: false,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"pending-charlie-1"}`)))
	rec := httptest.NewRecorder()
	sut.handleApproveAgent(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when identity is unverified, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDecodePendingEnrollmentAssetID(t *testing.T) {
	t.Run("rejects malformed body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{`)))
		rec := httptest.NewRecorder()

		if _, ok := decodePendingEnrollmentAssetID(rec, req); ok {
			t.Fatalf("expected malformed body to be rejected")
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("rejects missing asset_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"  "}`)))
		rec := httptest.NewRecorder()

		if _, ok := decodePendingEnrollmentAssetID(rec, req); ok {
			t.Fatalf("expected empty asset_id to be rejected")
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestResolveApprovedAssetID(t *testing.T) {
	if got := resolveApprovedAssetID(&pendingAgent{Hostname: "lab-host-01"}, "pending-lab-host-01"); got != "lab-host-01" {
		t.Fatalf("expected hostname to become stable asset id, got %q", got)
	}
	if got := resolveApprovedAssetID(&pendingAgent{Hostname: "unknown"}, "pending-unknown-1"); got != "pending-unknown-1" {
		t.Fatalf("expected unknown hostname to fall back to pending asset id, got %q", got)
	}
	if got := resolveApprovedAssetID(&pendingAgent{}, "pending-empty-1"); got != "pending-empty-1" {
		t.Fatalf("expected empty hostname to fall back to pending asset id, got %q", got)
	}
}

func TestHandleApproveAgentErrorPaths(t *testing.T) {
	t.Run("rejects non-POST method", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/approve", nil)
		rec := httptest.NewRecorder()
		sut.handleApproveAgent(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("returns not found for missing pending agent", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"pending-missing-1"}`)))
		rec := httptest.NewRecorder()
		sut.handleApproveAgent(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("returns service unavailable without enrollment store", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()
		sut.enrollmentStore = nil
		sut.pendingAgents.Add(&pendingAgent{
			AssetID:          "pending-foxtrot-1",
			Hostname:         "foxtrot",
			IdentityVerified: true,
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"pending-foxtrot-1"}`)))
		rec := httptest.NewRecorder()
		sut.handleApproveAgent(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
	})
}

func TestHandleApproveAgentSuccess(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()

	serverConn, clientConn, cleanup := createWSPairForPendingEnrollmentTest(t)
	defer cleanup()

	verifiedAt := time.Now().UTC()
	sut.pendingAgents.Add(&pendingAgent{
		AssetID:            "pending-golf-1",
		Hostname:           "golf",
		Platform:           "linux",
		ConnectedAt:        verifiedAt,
		DeviceFingerprint:  "sha256:test",
		IdentityVerified:   true,
		IdentityVerifiedAt: &verifiedAt,
		Conn:               serverConn,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"pending-golf-1"}`)))
	rec := httptest.NewRecorder()
	sut.handleApproveAgent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "approved" {
		t.Fatalf("expected approved status, got %v", resp["status"])
	}
	if resp["asset_id"] != "golf" {
		t.Fatalf("expected stable asset_id golf, got %v", resp["asset_id"])
	}

	if _, ok := sut.pendingAgents.Get("pending-golf-1"); ok {
		t.Fatalf("expected pending agent to be removed after approval")
	}

	tokens, err := sut.enrollmentStore.ListAgentTokens(10)
	if err != nil {
		t.Fatalf("list agent tokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].AssetID != "golf" {
		t.Fatalf("expected one issued token for golf, got %+v", tokens)
	}

	if _, ok, err := sut.assetStore.GetAsset("golf"); err != nil {
		t.Fatalf("get asset: %v", err)
	} else if !ok {
		t.Fatalf("expected approved asset to be upserted")
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := clientConn.ReadJSON(&msg); err != nil {
		t.Fatalf("read approval message: %v", err)
	}
	if msg.Type != agentmgr.MsgEnrollmentApproved {
		t.Fatalf("expected enrollment.approved message, got %q", msg.Type)
	}
	var approved agentmgr.EnrollmentApprovedData
	if err := json.Unmarshal(msg.Data, &approved); err != nil {
		t.Fatalf("decode approval payload: %v", err)
	}
	if approved.AssetID != "golf" {
		t.Fatalf("expected approved asset id golf, got %q", approved.AssetID)
	}
	if strings.TrimSpace(approved.Token) == "" {
		t.Fatalf("expected issued token in approval payload")
	}
}

func TestHandleRejectAgentSuccess(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()

	serverConn, clientConn, cleanup := createWSPairForPendingEnrollmentTest(t)
	defer cleanup()

	sut.pendingAgents.Add(&pendingAgent{
		AssetID:     "pending-hotel-1",
		Hostname:    "hotel",
		Platform:    "linux",
		ConnectedAt: time.Now().UTC(),
		Conn:        serverConn,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/reject", bytes.NewReader([]byte(`{"asset_id":"pending-hotel-1"}`)))
	rec := httptest.NewRecorder()
	sut.handleRejectAgent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, ok := sut.pendingAgents.Get("pending-hotel-1"); ok {
		t.Fatalf("expected pending agent to be removed after rejection")
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := clientConn.ReadJSON(&msg); err != nil {
		t.Fatalf("read rejection message: %v", err)
	}
	if msg.Type != agentmgr.MsgEnrollmentRejected {
		t.Fatalf("expected enrollment.rejected message, got %q", msg.Type)
	}
	var rejected agentmgr.EnrollmentRejectedData
	if err := json.Unmarshal(msg.Data, &rejected); err != nil {
		t.Fatalf("decode rejection payload: %v", err)
	}
	if !strings.Contains(rejected.Reason, "rejected") {
		t.Fatalf("expected rejection reason, got %q", rejected.Reason)
	}

	if _, _, err := clientConn.ReadMessage(); err == nil {
		t.Fatalf("expected close frame after rejection")
	}
}
