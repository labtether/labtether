package main

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type cancelOnFinalizeEnrollmentStore struct {
	persistence.EnrollmentStore
	persistence.AgentEnrollmentTransactionStore
	cancelRequest          context.CancelFunc
	finalizeContextWasLive bool
}

func (s *cancelOnFinalizeEnrollmentStore) FinalizeAgentApproval(ctx context.Context, req persistence.AgentApprovalFinalizeRequest) (assets.Asset, error) {
	s.cancelRequest()
	s.finalizeContextWasLive = ctx.Err() == nil
	return s.AgentEnrollmentTransactionStore.FinalizeAgentApproval(ctx, req)
}

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
