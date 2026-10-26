package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/assetstatus"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/dockerpkg"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/modelmap"
)

const dockerOwnerAgent = "Agent.A"
const dockerOwnerComposeEndpoint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const dockerOwnerContainer = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type dockerActionOwnerFixture struct {
	t        *testing.T
	sut      *apiServer
	coord    *docker.Coordinator
	window   *trueNASActionWindows
	sent     []agentmgr.Message
	action   string
	response string
}

func newDockerActionOwnerFixture(t *testing.T) *dockerActionOwnerFixture {
	t.Helper()
	f := &dockerActionOwnerFixture{t: t, sut: newTestAPIServer(t), window: &trueNASActionWindows{}, action: "container.stop"}
	f.sut.assetStore = assetstatus.NewStore(f.sut.assetStore, nil)
	f.sut.groupMaintenanceStore = f.window
	f.coord = docker.NewCoordinator(dryRunDockerCommander(func(agent string, msg agentmgr.Message) error {
		f.sent = append(f.sent, msg)
		if agent != dockerOwnerAgent || msg.ID == "" {
			return fmt.Errorf("wrong actual agent or missing request ID: %s", agent)
		}
		switch msg.Type {
		case agentmgr.MsgDockerAction:
			var request agentmgr.DockerActionData
			if err := json.Unmarshal(msg.Data, &request); err != nil || request.RequestID != msg.ID || request.Action != f.action {
				return fmt.Errorf("invalid Docker request")
			}
			for _, key := range []string{dockerpkg.ActionAgentBindingParam, "docker_container_id", "docker_stack_name", "docker_config_dir", "docker_config_files", "docker_compose_endpoint_id"} {
				if _, leaked := request.Params[key]; leaked {
					return fmt.Errorf("internal resource binding leaked onto the wire")
				}
			}
			if request.Action == "container.stop" && request.ContainerID != dockerOwnerContainer {
				return fmt.Errorf("container dispatch lost its full ID")
			}
			if request.Action == "image.pull" && (request.ContainerID != "" || request.ImageRef != "fixture:tag") {
				return fmt.Errorf("image dispatch lost its host/image identity")
			}
			raw, _ := json.Marshal(agentmgr.DockerActionResultData{RequestID: msg.ID, Success: true})
			f.coord.HandleActionResult(agent, agentmgr.Message{Type: agentmgr.MsgDockerActionResult, ID: msg.ID, Data: raw})
		case agentmgr.MsgDockerComposeActionV2:
			var request agentmgr.DockerComposeActionData
			if err := json.Unmarshal(msg.Data, &request); err != nil || request.RequestID != msg.ID || f.action != "stack.down" || request.Action != "down" || request.StackName != "lab" || request.ConfigDir != "/srv/lab" || request.EndpointID != dockerOwnerComposeEndpoint || !slices.Equal(request.ConfigFiles, []string{"/srv/shared/base.yaml", "/srv/lab/override.yml"}) {
				return fmt.Errorf("invalid compose request")
			}
			raw, _ := json.Marshal(agentmgr.DockerComposeResultData{RequestID: msg.ID, Success: true})
			f.coord.HandleComposeResult(agent, agentmgr.Message{Type: agentmgr.MsgDockerComposeResult, ID: msg.ID, Data: raw})
		default:
			return fmt.Errorf("unexpected agent message: %s", msg.Type)
		}
		return nil
	}))
	f.discover(dockerOwnerAgent)
	f.sut.dockerCoordinator = f.coord
	f.sut.connectorRegistry.Register(modelmap.WrapConnector(f.coord))
	for _, row := range []struct{ id, kind, group, source string }{
		{dockerOwnerAgent, "host", "agent-group", "agent"},
		{"docker-host-agent-a", "container-host", "docker-host-group", "docker"},
		{"docker-ct-agent-a-aaaaaaaaaaaa", "docker-container", "container-group", "docker"},
		{"docker-stack-agent-a-lab", "compose-stack", "stack-group", "docker"},
	} {
		if _, err := f.sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: row.id, Name: row.id, Type: row.kind, Source: row.source, Status: "online", GroupID: row.group,
			Metadata: map[string]string{"agent_id": dockerOwnerAgent},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *dockerActionOwnerFixture) discover(agent string) {
	f.coord.HandleDiscovery(agent, makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		Containers:           []agentmgr.DockerContainerInfo{{ID: dockerOwnerContainer, Name: "fixture"}},
		ComposeActionVersion: 2, ComposeEndpointID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ComposeStacks: []agentmgr.DockerComposeStack{{Name: "lab", ConfigFile: "/srv/lab/compose.yaml", ConfigDir: "/srv/lab", ConfigFiles: []string{"/srv/shared/base.yaml", "/srv/lab/override.yml"}}},
	}))
}

func (f *dockerActionOwnerFixture) direct(action, target string, want int) {
	f.t.Helper()
	body, _ := json.Marshal(connectorsdk.ActionRequest{TargetID: target, Params: map[string]string{"image": "fixture:tag"}})
	r := httptest.NewRequest(http.MethodPost, "/connectors/docker/actions/"+action+"/execute", strings.NewReader(string(body)))
	ctx, cancel := context.WithTimeout(contextWithPrincipal(r.Context(), "owner", "owner"), 3*time.Second)
	defer cancel()
	rec := httptest.NewRecorder()
	f.sut.handleConnectorActions(rec, r.WithContext(ctx))
	f.response = rec.Body.String()
	if rec.Code != want {
		f.t.Fatalf("direct status=%d want=%d body=%s", rec.Code, want, rec.Body.String())
	}
}

func (f *dockerActionOwnerFixture) queue(action, target string, want int) actions.Job {
	f.t.Helper()
	q := &trueNASActionQueue{}
	d := f.sut.buildActionsDeps()
	d.JobQueue = q
	body, _ := json.Marshal(actions.ExecuteRequest{Type: actions.RunTypeConnectorAction, ConnectorID: "docker", ActionID: action, Target: target,
		Params: map[string]string{"image": "fixture:tag", dockerpkg.ActionAgentBindingParam: "forged-owner", "docker_container_id": "forged-container", "docker_stack_name": "forged-stack", "docker_config_dir": "forged-directory", "docker_config_files": `["forged-file"]`, "docker_compose_endpoint_id": "forged-endpoint"}})
	r := httptest.NewRequest(http.MethodPost, "/actions/execute", strings.NewReader(string(body)))
	ctx := contextWithScopes(contextWithPrincipal(r.Context(), "owner", "owner"), []string{"actions:exec"})
	rec := httptest.NewRecorder()
	d.HandleActionExecute(rec, r.WithContext(ctx))
	f.response = rec.Body.String()
	if rec.Code != want {
		f.t.Fatalf("queue status=%d want=%d body=%s", rec.Code, want, rec.Body.String())
	}
	if want != http.StatusAccepted {
		if len(q.jobs) != 0 {
			f.t.Fatal("denied action was queued")
		}
		return actions.Job{}
	}
	if len(q.jobs) != 1 || q.jobs[0].Target != target || q.jobs[0].Params[dockerpkg.ActionAgentBindingParam] != dockerOwnerAgent || q.jobs[0].Params["collector_id"] != "" {
		f.t.Fatalf("queue lost the logical target or actual agent owner: %+v", q.jobs)
	}
	return q.jobs[0]
}

// A separately grouped child must still honor both of its real parent owners,
// including maintenance that starts only after the action was queued.
func TestDockerConfiguredActionHonorsParentMaintenance(t *testing.T) {
	for _, action := range []struct{ id, target, wireAction string }{
		{"workload.stop", "docker-ct-agent-a-aaaaaaaaaaaa", "container.stop"},
		{"stack.down", "docker-stack-agent-a-lab", "stack.down"},
		{"image.pull", dockerOwnerAgent, "image.pull"},
	} {
		for _, group := range []string{"agent-group", "docker-host-group"} {
			t.Run(action.id+"/"+group, func(t *testing.T) {
				f := newDockerActionOwnerFixture(t)
				f.action = action.wireAction
				f.window.blocked = group
				f.direct(action.id, action.target, http.StatusLocked)
				f.queue(action.id, action.target, http.StatusLocked)
				if len(f.sent) != 0 {
					t.Fatal("paused parent still dispatched")
				}
				f.window.blocked = ""
				job := f.queue(action.id, action.target, http.StatusAccepted)
				worker := f.sut.ensureProxmoxDeps()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				f.window.blocked = group
				if result := worker.ExecuteActionInProcessContext(ctx, job); result.Status != actions.StatusFailed || !strings.Contains(result.Error, "maintenance") || len(f.sent) != 0 {
					t.Fatalf("queued parent maintenance bypassed: %+v sent=%d", result, len(f.sent))
				}
				f.window.blocked = ""
				if result := worker.ExecuteActionInProcessContext(ctx, job); result.Status != actions.StatusSucceeded || len(f.sent) != 1 {
					t.Fatalf("unpaused worker failed: %+v sent=%d", result, len(f.sent))
				}
				if job.Params[dockerpkg.ActionAgentBindingParam] != dockerOwnerAgent {
					t.Fatal("wire parameter cleanup mutated the saved binding")
				}
				f.direct(action.id, action.target, http.StatusOK)
				if len(f.sent) != 2 {
					t.Fatal("ordinary direct control did not send exactly once")
				}
			})
		}
	}
}

func TestDockerConfiguredActionKeepsQueuedAgent(t *testing.T) {
	const target = "docker-ct-agent-a-aaaaaaaaaaaa"
	for _, timing := range []string{"before worker", "during final guard", "missing old binding"} {
		t.Run(timing, func(t *testing.T) {
			f := newDockerActionOwnerFixture(t)
			job := f.queue("container.stop", target, http.StatusAccepted)
			switched := false
			replace := func() {
				f.coord.RemoveHost(dockerOwnerAgent)
				f.discover("agent-a") // Same normalized asset ID, different actual agent.
				switched = true
			}
			switch timing {
			case "before worker":
				replace()
			case "during final guard":
				deps := f.sut.ensureDockerDeps()
				evaluate := deps.EvaluateAssetGuardrails
				deps.EvaluateAssetGuardrails = func(id string, at time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
					guard, err := evaluate(id, at)
					if id == "docker-host-agent-a" {
						replace()
					}
					return guard, err
				}
			case "missing old binding":
				delete(job.Params, dockerpkg.ActionAgentBindingParam)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result := f.sut.ensureProxmoxDeps().ExecuteActionInProcessContext(ctx, job)
			if result.Status != actions.StatusFailed || len(f.sent) != 0 {
				t.Fatalf("saved agent owner was bypassed: %+v sent=%d", result, len(f.sent))
			}
			if timing != "missing old binding" && !switched {
				t.Fatal("discovery replacement was not exercised")
			}
			diagnostic := result.Error
			for _, step := range result.Steps {
				diagnostic += " " + step.Error
			}
			if timing == "before worker" && !strings.Contains(diagnostic, "controlling agent") {
				t.Fatalf("queued owner mismatch was not the rejection: %+v", result)
			}
			if timing == "during final guard" && !strings.Contains(diagnostic, "authorized docker host") {
				t.Fatalf("coordinator send owner did not reject the race: %+v", result)
			}
			if timing == "missing old binding" && !strings.Contains(result.Error, "resubmit") {
				t.Fatalf("old unbound job not explained: %+v", result)
			}
		})
	}
}

// Observe the context entering the real Docker executor, then delegate to its
// coordinator. A one-minute shared wrapper must not truncate compose's 5m cap.
func TestDockerConfiguredActionPreservesCallerTimeout(t *testing.T) {
	f := newDockerActionOwnerFixture(t)
	f.action = "stack.down"
	job := f.queue("stack.down", "docker-stack-agent-a-lab", http.StatusAccepted)
	worker := f.sut.ensureProxmoxDeps()
	configured := worker.ConfiguredConnectorActions["docker"]
	execute := configured.Execute
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	want, _ := ctx.Deadline()
	called := false
	configured.Execute = func(actual context.Context, action string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
		called = true
		if deadline, ok := actual.Deadline(); !ok || !deadline.Equal(want) {
			t.Fatalf("Docker inherited a new deadline: %v %v, want %v", deadline, ok, want)
		}
		return execute(actual, action, req)
	}
	worker.ConfiguredConnectorActions["docker"] = configured
	if result := worker.ExecuteActionInProcessContext(ctx, job); result.Status != actions.StatusSucceeded || !called || len(f.sent) != 1 {
		t.Fatalf("real compose control failed: %+v called=%v sent=%d", result, called, len(f.sent))
	}
	cancel()
	if result := worker.ExecuteActionInProcessContext(ctx, job); result.Status != actions.StatusFailed || len(f.sent) != 1 {
		t.Fatalf("canceled caller still dispatched: %+v sent=%d", result, len(f.sent))
	}
}
