package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
)

type trackingEnrollmentAssetStore struct {
	*persistence.MemoryAssetStore
	listAssetsCalls int
	getAssetCalls   int
}

func (s *trackingEnrollmentAssetStore) ListAssets() ([]assets.Asset, error) {
	s.listAssetsCalls++
	return s.MemoryAssetStore.ListAssets()
}

func (s *trackingEnrollmentAssetStore) GetAsset(id string) (assets.Asset, bool, error) {
	s.getAssetCalls++
	return s.MemoryAssetStore.GetAsset(id)
}

func TestHandleEnrollmentTokenActionsReturnsNotFoundForMissingToken(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/settings/enrollment/missing-token", nil)
	rec := httptest.NewRecorder()
	sut.handleEnrollmentTokenActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAgentTokenActionsReturnsNotFoundForMissingToken(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/settings/agent-tokens/missing-token", nil)
	rec := httptest.NewRecorder()
	sut.handleAgentTokenActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAgentTokensUsesSingleAssetListLookup(t *testing.T) {
	sut := newTestAPIServer(t)
	trackingStore := &trackingEnrollmentAssetStore{MemoryAssetStore: persistence.NewMemoryAssetStore()}
	sut.assetStore = trackingStore
	sut.agentsDeps = nil
	sut.agentsDepsOnce = sync.Once{}

	if _, err := trackingStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "agent-node-01",
		Type:     "node",
		Name:     "agent-node-01",
		Source:   "agent",
		Status:   "online",
		Metadata: map[string]string{"agent_device_fingerprint": "fp-123"},
	}); err != nil {
		t.Fatalf("upsert asset heartbeat: %v", err)
	}

	_, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := sut.enrollmentStore.CreateAgentToken("agent-node-01", tokenHash, "test", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create agent token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/agent-tokens", nil)
	rec := httptest.NewRecorder()
	sut.handleAgentTokens(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Tokens []struct {
			AssetID           string `json:"asset_id"`
			DeviceFingerprint string `json:"device_fingerprint"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(resp.Tokens))
	}
	if resp.Tokens[0].DeviceFingerprint != "fp-123" {
		t.Fatalf("expected fingerprint to be preserved, got %q", resp.Tokens[0].DeviceFingerprint)
	}
	if trackingStore.listAssetsCalls != 1 {
		t.Fatalf("expected one ListAssets call, got %d", trackingStore.listAssetsCalls)
	}
	if trackingStore.getAssetCalls != 0 {
		t.Fatalf("expected zero GetAsset calls, got %d", trackingStore.getAssetCalls)
	}
}
