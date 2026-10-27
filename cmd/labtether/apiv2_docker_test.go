package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectors/docker"
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

// The v2 host container list returns raw Docker IDs; the CLI passes them back
// to the container routes, so those IDs must resolve to the right container.
func TestHandleV2DockerContainersAcceptListedDockerIDs(t *testing.T) {
	const listed = "439760c7acfa471c64148cb88b74143af5c5ae307f5904d9856b203e64106799"
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID:     "agent-01",
		Containers: []agentmgr.DockerContainerInfo{{ID: listed, Name: "probe"}},
	}))
	coord.HandleDiscovery("agent-02", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID:     "agent-02",
		Containers: []agentmgr.DockerContainerInfo{{ID: listed[:12] + strings.Repeat("0", 52), Name: "other"}},
	}))
	s := newTestAPIServer(t)
	s.dockerCoordinator = coord

	for _, tc := range []struct {
		id   string
		want int
	}{
		{listed, http.StatusOK},
		{listed[:16], http.StatusOK},
		{"docker-ct-agent-01-" + listed[:12], http.StatusOK},
		{listed[:12], http.StatusNotFound}, // matches containers on both hosts
		{listed[:11], http.StatusNotFound},
		{"zz" + listed[2:], http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v2/docker/containers/"+tc.id, nil)
		ctx := contextWithPrincipal(req.Context(), "apikey:k1", "owner")
		req = req.WithContext(contextWithScopes(ctx, []string{"docker:read"}))
		rec := httptest.NewRecorder()
		s.handleV2DockerContainerActions(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s: got %d, want %d: %s", tc.id, rec.Code, tc.want, rec.Body.String())
		}
		if tc.want == http.StatusOK && !strings.Contains(rec.Body.String(), `"agent-01"`) {
			t.Fatalf("%s resolved to the wrong host: %s", tc.id, rec.Body.String())
		}
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
