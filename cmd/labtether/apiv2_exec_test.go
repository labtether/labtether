package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

func TestHandleV2AssetExec_NoAgent(t *testing.T) {
	s := newTestAPIServer(t)
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "srv1", Status: "online", Platform: "linux",
		Source: "agent", Type: "host",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	body := `{"command":"uptime"}`
	req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetExec(rec, req, "srv1")

	// No agent connected → 409
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 (no agent), got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2AssetExec_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{"command":"uptime"}`
	req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"}) // no exec scope
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetExec(rec, req, "srv1")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetExec_MissingCommand(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{}`
	req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetExec(rec, req, "srv1")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleV2Exec_MultiTarget(t *testing.T) {
	s := newTestAPIServer(t)
	for _, id := range []string{"srv1", "srv2"} {
		_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id, Name: id, Status: "online", Platform: "linux",
			Source: "agent", Type: "host",
		})
		if err != nil {
			t.Fatalf("failed to create test asset %s: %v", id, err)
		}
	}

	body := `{"targets":["srv1","srv2"],"command":"uptime"}`
	req := httptest.NewRequest("POST", "/api/v2/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2ExecMulti(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	data := resp["data"].(map[string]any)
	if data["results"] == nil {
		t.Error("should have results")
	}
	if data["summary"] == nil {
		t.Error("should have summary")
	}
}
