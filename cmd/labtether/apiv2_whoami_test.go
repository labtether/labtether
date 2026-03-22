package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/assets"
)

func TestHandleV2Whoami_WithAPIKey(t *testing.T) {
	s := newTestAPIServer(t)

	now := time.Now().UTC()
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "server1",
		Name:     "Server 1",
		Status:   "online",
		Platform: "linux",
		Source:   "agent",
		Type:     "host",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	_ = s.apiKeyStore.CreateAPIKey(context.Background(), apikeys.APIKey{
		ID: "key_test1", Name: "test-key", Prefix: "ab12", SecretHash: "hash",
		Role: "operator", Scopes: []string{"assets:read", "assets:exec"},
		AllowedAssets: []string{"server1"}, CreatedBy: "admin", CreatedAt: now,
	})

	req := httptest.NewRequest("GET", "/api/v2/whoami", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:key_test1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read", "assets:exec"})
	ctx = contextWithAllowedAssets(ctx, []string{"server1"})
	ctx = contextWithAPIKeyID(ctx, "key_test1")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2Whoami(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]any)
	if data["key_name"] == nil {
		t.Error("should include key_name")
	}
	if data["role"] != "operator" {
		t.Errorf("role = %v, want operator", data["role"])
	}
	if data["auth_type"] != "api_key" {
		t.Errorf("auth_type = %v, want api_key", data["auth_type"])
	}
}

func TestHandleV2Whoami_SessionAuth(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("GET", "/api/v2/whoami", nil)
	ctx := contextWithPrincipal(req.Context(), "user1", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2Whoami(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]any)
	if data["auth_type"] != "session" {
		t.Errorf("auth_type = %v, want session", data["auth_type"])
	}
}
