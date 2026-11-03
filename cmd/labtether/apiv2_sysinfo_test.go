package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

func TestHandleV2AssetNetwork_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/network", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2AssetNetwork(rec, req, "srv1")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetDisks_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/disks", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2AssetDisks(rec, req, "srv1")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetPackages_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/packages", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2AssetPackages(rec, req, "srv1", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetPackagesExactRoutesAndScopes(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		subPath  string
		scopes   []string
		wantCode int
	}{
		{name: "upgradable read allowed", method: http.MethodGet, subPath: "/upgradable", scopes: []string{"packages:read"}, wantCode: http.StatusConflict},
		{name: "upgradable requires read", method: http.MethodGet, subPath: "/upgradable", scopes: []string{"packages:write"}, wantCode: http.StatusForbidden},
		{name: "update write allowed", method: http.MethodPost, subPath: "/update", scopes: []string{"packages:write"}, wantCode: http.StatusConflict},
		{name: "upgrade compatibility allowed", method: http.MethodPost, subPath: "/upgrade", scopes: []string{"packages:write"}, wantCode: http.StatusConflict},
		{name: "update requires write", method: http.MethodPost, subPath: "/update", scopes: []string{"packages:read"}, wantCode: http.StatusForbidden},
		{name: "upgradable is get only", method: http.MethodPost, subPath: "/upgradable", scopes: []string{"packages:write"}, wantCode: http.StatusMethodNotAllowed},
		{name: "update is post only", method: http.MethodGet, subPath: "/update", scopes: []string{"packages:read"}, wantCode: http.StatusMethodNotAllowed},
		{name: "unknown subpath", method: http.MethodGet, subPath: "/repair", scopes: []string{"packages:read"}, wantCode: http.StatusNotFound},
		{name: "nested subpath", method: http.MethodGet, subPath: "/upgradable/extra", scopes: []string{"packages:read"}, wantCode: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newTestAPIServer(t)
			seedOfflineTestAsset(t, s, "srv1")
			req := httptest.NewRequest(test.method, "/api/v2/assets/srv1/packages"+test.subPath, nil)
			ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
			ctx = contextWithScopes(ctx, test.scopes)
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			s.handleV2AssetPackages(rec, req, "srv1", test.subPath)
			if rec.Code != test.wantCode {
				t.Fatalf("status = %d, want %d: %s", rec.Code, test.wantCode, rec.Body.String())
			}
		})
	}
}

func TestHandleV2AssetLogs_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/logs", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2AssetLogs(rec, req, "srv1")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetMetrics_ScopeAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/metrics", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"metrics:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2AssetMetrics(rec, req, "srv1")
	if rec.Code == http.StatusForbidden {
		t.Fatal("scope check should have passed")
	}
}

// seedOfflineTestAsset adds a known device with no connected agent, so device
// routes answer 409 "agent is not connected" once they pass their scope checks.
func seedOfflineTestAsset(t *testing.T, s *apiServer, id string) {
	t.Helper()
	if _, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: id, Type: "host", Name: id, Source: "agent", Status: "online", Platform: "linux",
	}); err != nil {
		t.Fatalf("seed asset %s: %v", id, err)
	}
}
