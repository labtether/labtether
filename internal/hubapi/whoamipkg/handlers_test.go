package whoamipkg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
)

func newTestDeps(t *testing.T) *Deps {
	t.Helper()
	return &Deps{
		AssetStore:  testutil.NewAssetStore(),
		APIKeyStore: persistence.NewMemoryAPIKeyStore(),
	}
}

func TestHandleV2Whoami_MethodNotAllowed(t *testing.T) {
	d := newTestDeps(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/whoami", nil)
	rec := httptest.NewRecorder()
	d.HandleV2Whoami(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleV2Whoami_SessionAuth(t *testing.T) {
	d := newTestDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/whoami", nil)
	ctx := apiv2.ContextWithPrincipal(req.Context(), "user1", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	d.HandleV2Whoami(rec, req)

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
	if data["role"] != "admin" {
		t.Errorf("role = %v, want admin", data["role"])
	}
	scopes, ok := data["scopes"].([]any)
	if !ok || len(scopes) != 1 || scopes[0] != "*" {
		t.Errorf("scopes = %v, want [*]", data["scopes"])
	}
}

func TestHandleV2Whoami_APIKeyAuth(t *testing.T) {
	d := newTestDeps(t)

	// Seed an asset so available_assets can be populated.
	assetStore := d.AssetStore
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "srv1",
		Name:     "Server 1",
		Status:   "online",
		Platform: "linux",
		Source:   "agent",
		Type:     "host",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	now := time.Now().UTC()
	_ = d.APIKeyStore.CreateAPIKey(context.Background(), apikeys.APIKey{
		ID: "key1", Name: "my-key", Prefix: "ab12", SecretHash: "hash",
		Role: "operator", Scopes: []string{"assets:read"},
		AllowedAssets: []string{"srv1"}, CreatedBy: "admin", CreatedAt: now,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v2/whoami", nil)
	ctx := apiv2.ContextWithPrincipal(req.Context(), "apikey:key1", "operator")
	ctx = apiv2.ContextWithScopes(ctx, []string{"assets:read"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"srv1"})
	ctx = apiv2.ContextWithAPIKeyID(ctx, "key1")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	d.HandleV2Whoami(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]any)
	if data["auth_type"] != "api_key" {
		t.Errorf("auth_type = %v, want api_key", data["auth_type"])
	}
	if data["key_name"] != "my-key" {
		t.Errorf("key_name = %v, want my-key", data["key_name"])
	}
	availAssets, ok := data["available_assets"].([]any)
	if !ok || len(availAssets) == 0 {
		t.Errorf("available_assets = %v, want non-empty list", data["available_assets"])
	}
}

func TestHandleV2Whoami_APIKey_AssetNotAllowed(t *testing.T) {
	d := newTestDeps(t)

	// Seed an asset but allow a different one in context.
	assetStore := d.AssetStore
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "srv1",
		Name:     "Server 1",
		Status:   "online",
		Platform: "linux",
		Source:   "agent",
		Type:     "host",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/whoami", nil)
	ctx := apiv2.ContextWithPrincipal(req.Context(), "apikey:key2", "operator")
	ctx = apiv2.ContextWithScopes(ctx, []string{"assets:read"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"other-server"}) // srv1 not allowed
	ctx = apiv2.ContextWithAPIKeyID(ctx, "key2")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	d.HandleV2Whoami(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]any)
	availAssets, ok := data["available_assets"].([]any)
	if !ok {
		// nil is fine — no accessible assets
		return
	}
	if len(availAssets) != 0 {
		t.Errorf("expected no accessible assets, got %v", availAssets)
	}
}
