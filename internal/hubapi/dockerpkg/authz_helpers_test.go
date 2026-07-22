package dockerpkg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
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

func TestDockerMutationHandlersHonorMaintenanceBlockActions(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", dockerDiscovery("agent-01", "abc123def456ab"))
	var evaluated []string
	deps := &Deps{
		DockerCoordinator: coord,
		EvaluateAssetGuardrails: func(assetID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
			evaluated = append(evaluated, assetID)
			return groupfeatures.GroupMaintenanceGuardrails{GroupID: "group-1", BlockActions: true}, nil
		},
	}
	ctx := apiv2.ContextWithScopes(context.Background(), []string{"docker:write"})

	tests := []struct {
		name string
		path string
		run  func(http.ResponseWriter, *http.Request)
		want string
	}{
		{name: "host", path: "/api/v1/docker/hosts/agent-01/action", run: deps.HandleDockerHostActions, want: "agent-01"},
		{name: "container", path: "/api/v1/docker/containers/docker-ct-agent-01-abc123def456/action", run: deps.HandleDockerContainerActions, want: "docker-ct-agent-01-abc123def456"},
		{name: "stack", path: "/api/v1/docker/stacks/docker-stack-agent-01-lab/action", run: deps.HandleDockerStackActions, want: "docker-stack-agent-01-lab"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evaluated = nil
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{"action":"restart"}`)).WithContext(ctx)
			test.run(recorder, request)
			if recorder.Code != http.StatusLocked {
				t.Fatalf("status = %d, want 423: %s", recorder.Code, recorder.Body.String())
			}
			if len(evaluated) != 1 || evaluated[0] != test.want {
				t.Fatalf("evaluated assets = %+v, want [%s]", evaluated, test.want)
			}
		})
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
