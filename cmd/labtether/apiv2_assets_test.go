package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

func TestHandleV2Assets_List(t *testing.T) {
	s := newTestAPIServer(t)
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "Server 1", Status: "online", Platform: "linux",
		Source: "agent", Type: "host",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v2/assets", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2Assets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["data"] == nil {
		t.Error("response should have data")
	}
	if resp["request_id"] == nil {
		t.Error("response should have request_id")
	}
}

func TestHandleV2Assets_Get(t *testing.T) {
	s := newTestAPIServer(t)
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "Server 1", Status: "online", Platform: "linux",
		Source: "agent", Type: "host",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v2/assets/srv1", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2Assets_NotFound(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("GET", "/api/v2/assets/nonexistent", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleV2Assets_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("GET", "/api/v2/assets", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"docker:read"}) // no assets:read
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2Assets(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2Assets_AssetDenied(t *testing.T) {
	s := newTestAPIServer(t)
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "Server 1", Status: "online", Platform: "linux",
		Source: "agent", Type: "host",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v2/assets/srv1", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	ctx = contextWithAllowedAssets(ctx, []string{"other-server"}) // srv1 not allowed
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetActions(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2Assets_Create(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{"name":"pi4","ip":"192.168.1.50","platform":"linux"}`
	req := httptest.NewRequest("POST", "/api/v2/assets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2Assets(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2Assets_Delete(t *testing.T) {
	s := newTestAPIServer(t)
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "todelete", Name: "Delete Me", Status: "offline", Platform: "linux",
		Source: "manual", Type: "host",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/api/v2/assets/todelete", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2Assets_UpdateMissingAssetReturnsNotFound(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("PATCH", "/api/v2/assets/missing", strings.NewReader(`{"name":"Renamed"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2Assets_DeleteMissingAssetReturnsNotFound(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("DELETE", "/api/v2/assets/missing", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
