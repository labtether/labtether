// cmd/labtether/apiv2_resources_test.go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

func TestHandleV2AssetFiles_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/files?path=/", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"}) // no files:read
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetFiles(rec, req, "srv1", "")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetProcesses_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/processes", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"}) // no processes:read
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetProcesses(rec, req, "srv1")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetServices_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/services", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"}) // no services:read
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetServices(rec, req, "srv1")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetProcesses_ScopeAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "srv1",
		Name:     "srv1",
		Status:   "online",
		Platform: "linux",
		Source:   "agent",
		Type:     "host",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/processes", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"processes:read"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetProcesses(rec, req, "srv1")

	// Will fail with agent not connected (not 403), which is the right behavior
	// — the scope check passed.
	if rec.Code == http.StatusForbidden {
		t.Fatal("scope check should have passed")
	}
}
