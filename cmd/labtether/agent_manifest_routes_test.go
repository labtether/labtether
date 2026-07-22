package main

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labtether/labtether/internal/auth"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
)

func TestAgentManifestAndCacheRefreshRoutesAreRegisteredAndGuarded(t *testing.T) {
	dir := t.TempDir()
	manifest := &agentspkg.AgentManifest{
		SchemaVersion: 1,
		HubVersion:    "qa-route-test",
		Agents: map[string]agentspkg.AgentEntry{
			"labtether-agent": {Version: "qa-route-test"},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, agentspkg.ManifestFilename), manifestJSON, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cache := &agentspkg.AgentCache{RuntimeDir: dir, BakedInDir: dir}
	cache.SetManifest(manifest)
	sut := newTestAPIServer(t)
	sut.agentCache = cache
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	manifestHandler := handlers["/api/v1/agent/manifest"]
	if manifestHandler == nil {
		t.Fatal("expected /api/v1/agent/manifest to be registered")
	}
	refreshHandler := handlers["/api/v1/agent/cache/refresh"]
	if refreshHandler == nil {
		t.Fatal("expected /api/v1/agent/cache/refresh to be registered")
	}

	invokeWithoutAuth := func(handler http.HandlerFunc, method, path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.TLS = &tls.ConnectionState{}
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}

	if rec := invokeWithoutAuth(manifestHandler, http.MethodGet, "/api/v1/agent/manifest"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated manifest status = %d, want 401", rec.Code)
	}
	missingRead := createLegacyRouteAPIKey(t, sut, []string{"assets:read"}, nil)
	if rec := invokeLegacyRoute(t, manifestHandler, http.MethodGet, "/api/v1/agent/manifest", missingRead, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("manifest without agents:read status = %d, want 403", rec.Code)
	}
	readKey := createLegacyRouteAPIKey(t, sut, []string{"agents:read"}, nil)
	if rec := invokeLegacyRoute(t, manifestHandler, http.MethodGet, "/api/v1/agent/manifest", readKey, ""); rec.Code != http.StatusOK {
		t.Fatalf("manifest with agents:read status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if rec := invokeLegacyRoute(t, manifestHandler, http.MethodPost, "/api/v1/agent/manifest", readKey, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("manifest POST status = %d, want 405", rec.Code)
	}

	if rec := invokeWithoutAuth(refreshHandler, http.MethodPost, "/api/v1/agent/cache/refresh"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated cache refresh status = %d, want 401", rec.Code)
	}
	operatorWrite := createLegacyRouteAPIKey(t, sut, []string{"agents:write"}, nil)
	if rec := invokeLegacyRoute(t, refreshHandler, http.MethodPost, "/api/v1/agent/cache/refresh", operatorWrite, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("operator cache refresh status = %d, want 403", rec.Code)
	}
	adminMissingWrite := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleAdmin, []string{"agents:read"}, nil)
	if rec := invokeLegacyRoute(t, refreshHandler, http.MethodPost, "/api/v1/agent/cache/refresh", adminMissingWrite, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("admin cache refresh without agents:write status = %d, want 403", rec.Code)
	}
	restrictedAdmin := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleAdmin, []string{"agents:write"}, []string{"node-1"})
	if rec := invokeLegacyRoute(t, refreshHandler, http.MethodPost, "/api/v1/agent/cache/refresh", restrictedAdmin, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("asset-restricted admin cache refresh status = %d, want 403", rec.Code)
	}
	adminWrite := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleAdmin, []string{"agents:write"}, nil)
	if rec := invokeLegacyRoute(t, refreshHandler, http.MethodGet, "/api/v1/agent/cache/refresh", adminWrite, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("cache refresh GET status = %d, want 405", rec.Code)
	}
	if rec := invokeLegacyRoute(t, refreshHandler, http.MethodPost, "/api/v1/agent/cache/refresh", adminWrite, ""); rec.Code != http.StatusOK {
		t.Fatalf("admin cache refresh status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}
