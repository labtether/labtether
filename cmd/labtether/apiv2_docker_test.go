package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleV2DockerHosts_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/docker/hosts", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2DockerHosts(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2DockerHosts_ScopeAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/docker/hosts", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"docker:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2DockerHosts(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatal("scope check should have passed")
	}
}

func TestHandleV2DockerContainers_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("POST", "/api/v2/docker/containers/abc/start", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"docker:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2DockerContainerActions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
