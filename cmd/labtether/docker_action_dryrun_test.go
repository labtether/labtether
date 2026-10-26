package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/modelmap"
)

type dryRunDockerCommander func(string, agentmgr.Message) error

func (f dryRunDockerCommander) SendToAgent(id string, msg agentmgr.Message) error { return f(id, msg) }

// The same production coordinator serves direct connector calls and queued
// jobs. Capture its real agent messages, with synchronous correlated replies
// so a regression cannot leave a test worker waiting for the action timeout.
func TestDockerActionDryRunNeverSendsAgentRPC(t *testing.T) {
	const fullID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, action := range []struct{ id, target, wireType string }{
		{"container.remove", "docker-ct-agent-dryrun-aaaaaaaaaaaa", agentmgr.MsgDockerAction},
		{"stack.down", "docker-stack-agent-dryrun-lab", agentmgr.MsgDockerComposeActionV2},
	} {
		for _, route := range []string{"coordinator", "direct", "queued"} {
			t.Run(action.id+"/"+route, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				sut := newTestAPIServer(t)
				sent := 0
				var coordinator *docker.Coordinator
				coordinator = docker.NewCoordinator(dryRunDockerCommander(func(id string, msg agentmgr.Message) error {
					sent++
					if id != "agent-dryrun" || msg.Type != action.wireType || msg.ID == "" {
						return fmt.Errorf("wrong wire owner: %s/%s", id, msg.Type)
					}
					if msg.Type == agentmgr.MsgDockerAction {
						var payload agentmgr.DockerActionData
						if err := json.Unmarshal(msg.Data, &payload); err != nil || payload.Action != action.id || payload.ContainerID != fullID || payload.RequestID != msg.ID {
							return fmt.Errorf("incorrect container action payload")
						}
						raw, _ := json.Marshal(agentmgr.DockerActionResultData{RequestID: msg.ID, Success: true})
						coordinator.HandleActionResult(id, agentmgr.Message{Type: agentmgr.MsgDockerActionResult, ID: msg.ID, Data: raw})
					} else {
						var payload agentmgr.DockerComposeActionData
						if err := json.Unmarshal(msg.Data, &payload); err != nil || payload.Action != "down" || payload.StackName != "lab" || payload.ConfigDir != "/srv/lab" || payload.RequestID != msg.ID {
							return fmt.Errorf("incorrect compose action payload")
						}
						raw, _ := json.Marshal(agentmgr.DockerComposeResultData{RequestID: msg.ID, Success: true})
						coordinator.HandleComposeResult(id, agentmgr.Message{Type: agentmgr.MsgDockerComposeResult, ID: msg.ID, Data: raw})
					}
					return nil
				}))
				coordinator.HandleDiscovery("agent-dryrun", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
					Containers:           []agentmgr.DockerContainerInfo{{ID: fullID, Name: "fixture"}},
					ComposeActionVersion: 2, ComposeEndpointID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					ComposeStacks: []agentmgr.DockerComposeStack{{Name: "lab", ConfigFile: "/srv/lab/compose.yaml", ConfigDir: "/srv/lab", ConfigFiles: []string{"/srv/shared/base.yaml", "/srv/lab/override.yml"}}},
				}))
				sut.dockerCoordinator = coordinator
				sut.connectorRegistry.Register(modelmap.WrapConnector(coordinator))
				for _, dryRun := range []bool{true, false} {
					request := connectorsdk.ActionRequest{TargetID: action.target, DryRun: dryRun}
					var result connectorsdk.ActionResult
					switch route {
					case "coordinator":
						var err error
						result, err = coordinator.ExecuteAction(ctx, action.id, request)
						if err != nil {
							t.Fatal(err)
						}
					case "direct":
						body, _ := json.Marshal(request)
						r := httptest.NewRequest(http.MethodPost, "/connectors/docker/actions/"+action.id+"/execute", bytes.NewReader(body))
						r = r.WithContext(contextWithPrincipal(ctx, "owner", "owner"))
						rec := httptest.NewRecorder()
						sut.handleConnectorActions(rec, r)
						want := http.StatusOK
						if dryRun {
							want = http.StatusBadRequest
						}
						var reply struct {
							Result connectorsdk.ActionResult `json:"result"`
						}
						if rec.Code != want || json.Unmarshal(rec.Body.Bytes(), &reply) != nil {
							t.Fatalf("direct status=%d body=%s", rec.Code, rec.Body.String())
						}
						result = reply.Result
					case "queued":
						queue := &trueNASActionQueue{}
						deps := sut.buildActionsDeps()
						deps.JobQueue = queue
						body, _ := json.Marshal(actions.ExecuteRequest{Type: actions.RunTypeConnectorAction, ConnectorID: "docker", ActionID: action.id, Target: action.target, DryRun: dryRun})
						r := httptest.NewRequest(http.MethodPost, "/actions/execute", bytes.NewReader(body))
						r = r.WithContext(contextWithScopes(contextWithPrincipal(ctx, "owner", "owner"), []string{"actions:exec"}))
						rec := httptest.NewRecorder()
						deps.HandleActionExecute(rec, r)
						if rec.Code != http.StatusAccepted || len(queue.jobs) != 1 || queue.jobs[0].DryRun != dryRun {
							t.Fatalf("queue lost request: status=%d jobs=%+v body=%s", rec.Code, queue.jobs, rec.Body.String())
						}
						completed := sut.ensureProxmoxDeps().ExecuteActionInProcessContext(ctx, queue.jobs[0])
						result = connectorsdk.ActionResult{Status: completed.Status, Message: completed.Output}
					}
					if dryRun {
						if result.Status != "failed" || !strings.Contains(result.Message, "dry-run is not supported") || !strings.Contains(result.Message, "no changes applied") || sent != 0 {
							t.Fatalf("dry run dispatched or misreported: result=%+v sent=%d", result, sent)
						}
					} else if result.Status != "succeeded" || sent != 1 {
						t.Fatalf("ordinary action failed: result=%+v sent=%d", result, sent)
					}
				}
			})
		}
	}
}
