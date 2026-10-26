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
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
)

type actionTargetCommander func(string, agentmgr.Message) error

func (f actionTargetCommander) SendToAgent(id string, msg agentmgr.Message) error { return f(id, msg) }

// Use the real coordinator so assertions cover which agent receives the wire
// command, rather than only inspecting the handler's requested target string.
func actionTargetFixture(t *testing.T, blocked string) (*Deps, *[]string) {
	t.Helper()
	var sent []string
	var coord *docker.Coordinator
	coord = docker.NewCoordinator(actionTargetCommander(func(id string, msg agentmgr.Message) error {
		sent = append(sent, id)
		if msg.Type == agentmgr.MsgDockerComposeActionV2 {
			raw, _ := json.Marshal(agentmgr.DockerComposeResultData{RequestID: msg.ID, Success: true})
			coord.HandleComposeResult(id, agentmgr.Message{ID: msg.ID, Type: agentmgr.MsgDockerComposeResult, Data: raw})
		} else {
			raw, _ := json.Marshal(agentmgr.DockerActionResultData{RequestID: msg.ID, Success: true})
			coord.HandleActionResult(id, agentmgr.Message{ID: msg.ID, Type: agentmgr.MsgDockerActionResult, Data: raw})
		}
		return nil
	}))
	for _, id := range []string{"agent-a", "agent-b"} {
		raw, _ := json.Marshal(agentmgr.DockerDiscoveryData{
			HostID:               id,
			Containers:           []agentmgr.DockerContainerInfo{{ID: "abc123def456ab", Name: "nginx"}},
			ComposeActionVersion: 2, ComposeEndpointID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ComposeStacks: []agentmgr.DockerComposeStack{{Name: "lab", ConfigFile: "/srv/lab/compose.yaml", ConfigDir: "/srv/lab", ConfigFiles: []string{"/srv/shared/base.yaml", "/srv/lab/override.yml"}}},
		})
		coord.HandleDiscovery(id, agentmgr.Message{Data: raw})
	}
	return &Deps{
		DockerCoordinator:   coord,
		ExecuteDockerAction: coord.ExecuteAction,
		DecodeJSONBody: func(_ http.ResponseWriter, r *http.Request, dst any) error {
			return json.NewDecoder(r.Body).Decode(dst)
		},
		SanitizeUpstreamError: func(value string) string { return value },
		EvaluateAssetGuardrails: func(assetID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
			return groupfeatures.GroupMaintenanceGuardrails{BlockActions: assetID == blocked}, nil
		},
	}, &sent
}

func dockerActionRequest(path, action, target string, allowed []string) *http.Request {
	body, _ := json.Marshal(map[string]any{
		"action": action, "target_id": target,
		"params": map[string]string{"image": "nginx:latest", "stack_name": "new-lab", "compose_yaml": "services: {}"},
	})
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	ctx := apiv2.ContextWithScopes(request.Context(), []string{"docker:write"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, allowed)
	return request.WithContext(ctx)
}

func TestDockerHostActionCannotRetargetAnotherHost(t *testing.T) {
	for _, restricted := range []bool{true, false} {
		for _, action := range []struct{ id, target string }{
			{"image.pull", "docker-host-agent-b"},
			{"container.create", "agent-b"},
			{"stack.deploy", "agent-b"},
			{"container.restart", "docker-ct-agent-b-abc123def456"},
			{"stack.restart", "docker-stack-agent-b-lab"},
		} {
			t.Run(action.id+"/"+map[bool]string{true: "restricted", false: "session"}[restricted], func(t *testing.T) {
				deps, sent := actionTargetFixture(t, "")
				var allowed []string
				if restricted {
					allowed = []string{"docker-host-agent-a"}
				}
				recorder := httptest.NewRecorder()
				deps.HandleDockerHostActions(recorder, dockerActionRequest("/api/v1/docker/hosts/agent-a/action", action.id, action.target, allowed))
				if recorder.Code != http.StatusForbidden || len(*sent) != 0 {
					t.Fatalf("cross-host status=%d sent=%v body=%s", recorder.Code, *sent, recorder.Body.String())
				}
			})
		}
	}
}

func TestDockerHostActionPreservesAuthorizedTargets(t *testing.T) {
	for _, test := range []struct{ action, target string }{
		{"image.pull", ""}, {"container.create", "docker-host-agent-a"},
		{"stack.deploy", "Agent.A"}, {"container.restart", "docker-ct-agent-a-abc123def456"},
		{"stack.restart", "docker-stack-agent-a-lab"},
	} {
		t.Run(test.action, func(t *testing.T) {
			deps, sent := actionTargetFixture(t, "")
			recorder := httptest.NewRecorder()
			allowed := []string{"docker-host-agent-a", "docker-ct-agent-a-abc123def456", "docker-stack-agent-a-lab"}
			deps.HandleDockerHostActions(recorder, dockerActionRequest("/api/v1/docker/hosts/docker-host-agent-a/action", test.action, test.target, allowed))
			if recorder.Code != http.StatusOK || len(*sent) != 1 || (*sent)[0] != "agent-a" {
				t.Fatalf("same-host status=%d sent=%v body=%s", recorder.Code, *sent, recorder.Body.String())
			}
		})
	}
}

func TestDockerHostActionHonorsResourceAccessAndMaintenance(t *testing.T) {
	const containerID = "docker-ct-agent-a-abc123def456"
	for _, test := range []struct {
		name, blocked string
		allowed       []string
		code          int
	}{
		{"child scope", "", []string{"docker-host-agent-a"}, http.StatusForbidden},
		{"child maintenance", containerID, nil, http.StatusLocked},
		{"host asset maintenance", "docker-host-agent-a", nil, http.StatusLocked},
	} {
		t.Run(test.name, func(t *testing.T) {
			deps, sent := actionTargetFixture(t, test.blocked)
			recorder := httptest.NewRecorder()
			deps.HandleDockerHostActions(recorder, dockerActionRequest("/api/v1/docker/hosts/agent-a/action", "container.restart", containerID, test.allowed))
			if recorder.Code != test.code || len(*sent) != 0 {
				t.Fatalf("status=%d sent=%v body=%s", recorder.Code, *sent, recorder.Body.String())
			}
		})
	}
}

func TestDockerChildActionHonorsHostMaintenance(t *testing.T) {
	for _, test := range []struct{ kind, target, action string }{
		{"containers", "docker-ct-agent-a-abc123def456", "container.restart"},
		{"stacks", "docker-stack-agent-a-lab", "stack.restart"},
	} {
		for _, blocked := range []string{"agent-a", "docker-host-agent-a", ""} {
			t.Run(test.kind+"/"+blocked, func(t *testing.T) {
				deps, sent := actionTargetFixture(t, blocked)
				recorder := httptest.NewRecorder()
				request := dockerActionRequest("/api/v1/docker/"+test.kind+"/"+test.target+"/action", test.action, "", []string{test.target})
				if test.kind == "containers" {
					deps.HandleDockerContainerActions(recorder, request)
				} else {
					deps.HandleDockerStackActions(recorder, request)
				}
				wantCode, wantSent := http.StatusLocked, 0
				if blocked == "" {
					wantCode, wantSent = http.StatusOK, 1
				}
				if recorder.Code != wantCode || len(*sent) != wantSent {
					t.Fatalf("status=%d sent=%v body=%s", recorder.Code, *sent, recorder.Body.String())
				}
				if wantSent == 1 && (*sent)[0] != "agent-a" {
					t.Fatalf("wrong agent received child action: %v", *sent)
				}
			})
		}
	}
}

func TestDockerActionDispatchPreservesCheckedHost(t *testing.T) {
	for _, action := range []string{"image.pull", "stack.deploy"} {
		t.Run(action, func(t *testing.T) {
			deps, sent := actionTargetFixture(t, "")
			// Model a target resolving to a different owner after API admission.
			result, err := deps.ExecuteDockerAction(docker.WithActionHost(context.Background(), "agent-a"), action, connectorsdk.ActionRequest{
				TargetID: "agent-b", Params: map[string]string{"stack_name": "new-lab"},
			})
			if err != nil || result.Status != "failed" || !strings.Contains(result.Message, "authorized docker host") || len(*sent) != 0 {
				t.Fatalf("result=%+v err=%v sent=%v", result, err, *sent)
			}
		})
	}
}
