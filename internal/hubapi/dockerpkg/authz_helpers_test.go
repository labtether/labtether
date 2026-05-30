package dockerpkg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/connectors/docker"
)

func TestDockerV1HandlersRequireAPIKeyScopes(t *testing.T) {
	deps := &Deps{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts", nil)
	req = req.WithContext(apiv2.ContextWithScopes(req.Context(), []string{"assets:read"}))
	rec := httptest.NewRecorder()

	deps.HandleDockerHosts(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for docker hosts without docker:read, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDockerV1HandlersApplyAPIKeyAssetAllowlist(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", dockerDiscovery("agent-01", "abc123def456ab"))
	coord.HandleDiscovery("secret-01", dockerDiscovery("secret-01", "fff123def456ab"))
	deps := &Deps{DockerCoordinator: coord}

	ctx := apiv2.ContextWithScopes(context.Background(), []string{"docker:read"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"docker-host-agent-01"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	deps.HandleDockerHosts(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed docker host list, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent-01") {
		t.Fatalf("response omitted allowed host: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-01") {
		t.Fatalf("response leaked disallowed host: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts/secret-01", nil).WithContext(ctx)
	rec = httptest.NewRecorder()
	deps.HandleDockerHostActions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed docker host detail, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDockerV1ContainerActionsRequireWriteScope(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", dockerDiscovery("agent-01", "abc123def456ab"))
	deps := &Deps{DockerCoordinator: coord}

	ctx := apiv2.ContextWithScopes(context.Background(), []string{"docker:read"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"docker-ct-agent-01-abc123def456"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docker/containers/docker-ct-agent-01-abc123def456/action", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	deps.HandleDockerContainerActions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for container action without docker:write, got %d: %s", rec.Code, rec.Body.String())
	}
}

func dockerDiscovery(agentID, containerID string) agentmgr.Message {
	raw, _ := json.Marshal(agentmgr.DockerDiscoveryData{
		HostID: agentID,
		Engine: agentmgr.DockerEngineInfo{
			Version: "24.0.7",
			OS:      "linux",
			Arch:    "amd64",
		},
		Containers: []agentmgr.DockerContainerInfo{{
			ID:    containerID,
			Name:  "nginx",
			Image: "nginx:latest",
			State: "running",
		}},
	})
	return agentmgr.Message{Type: agentmgr.MsgDockerDiscovery, Data: raw}
}
