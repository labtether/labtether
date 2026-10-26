package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
)

// The real direct, queue and worker paths must preserve file order, refuse old
// capability/selection data and reject a last-moment change on dedicated routes.
func TestDockerComposeActionsKeepExactFileSelection(t *testing.T) {
	const target = "docker-stack-agent-a-lab"
	const binding = "docker_config_files"
	files := []string{"/srv/shared/base.yaml", "/srv/lab/override.yml"}
	discovery := func() agentmgr.DockerDiscoveryData {
		return agentmgr.DockerDiscoveryData{ComposeActionVersion: 2, ComposeEndpointID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ComposeStacks: []agentmgr.DockerComposeStack{{
			Name: "lab", ConfigFile: "/legacy/wrong/docker-compose.yml", ConfigDir: "/srv/lab", ConfigFiles: slices.Clone(files),
		}}}
	}
	dedicated := func(t *testing.T, f *dockerActionOwnerFixture, want int) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/docker/stacks/"+target+"/action", strings.NewReader(`{"action":"stack.down"}`))
		ctx, cancel := context.WithTimeout(contextWithPrincipal(r.Context(), "owner", "owner"), 3*time.Second)
		defer cancel()
		w := httptest.NewRecorder()
		f.sut.handleDockerStackActions(w, r.WithContext(ctx))
		if w.Code != want {
			t.Fatalf("dedicated status=%d want=%d body=%s", w.Code, want, w.Body.String())
		}
	}
	t.Run("valid full discovery to wire", func(t *testing.T) {
		f := newDockerActionOwnerFixture(t)
		f.action = "stack.down"
		f.coord.HandleDiscovery(dockerOwnerAgent, makeDockerDiscovery(discovery()))
		// API readers cannot edit the coordinator's admitted file slice.
		host, _ := f.coord.GetHost(dockerOwnerAgent)
		host.ComposeStacks[0].ConfigFiles[0] = "/reader/forged.yaml"
		f.direct("stack.down", target, http.StatusOK)
		job := f.queue("stack.down", target, http.StatusAccepted)
		if job.Params["docker_compose_endpoint_id"] != dockerOwnerComposeEndpoint {
			t.Fatalf("queue lost endpoint binding: %q", job.Params["docker_compose_endpoint_id"])
		}
		var saved []string
		if err := json.Unmarshal([]byte(job.Params[binding]), &saved); err != nil || !slices.Equal(saved, files) {
			t.Fatalf("queue lost exact order: %q err=%v", job.Params[binding], err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if result := f.sut.ensureProxmoxDeps().ExecuteActionInProcessContext(ctx, job); result.Status != actions.StatusSucceeded {
			t.Fatalf("worker failed: %+v", result)
		}
		dedicated(t, f, http.StatusOK)
		if len(f.sent) != 3 {
			t.Fatalf("expected one v2 effect per path, sent=%d", len(f.sent))
		}
	})
	for _, invalid := range []string{"old capability", "unknown capability", "selection error", "no files", "no directory", "no endpoint", "invalid endpoint"} {
		t.Run(invalid, func(t *testing.T) {
			f := newDockerActionOwnerFixture(t)
			f.action = "stack.down"
			data := discovery()
			switch invalid {
			case "old capability":
				data.ComposeActionVersion = 0
			case "unknown capability":
				data.ComposeActionVersion = 3
			case "selection error":
				data.ComposeStacks[0].SelectionError = "inconsistent_labels"
			case "no files":
				data.ComposeStacks[0].ConfigFiles = nil
			case "no directory":
				data.ComposeStacks[0].ConfigDir = ""
			case "no endpoint":
				data.ComposeEndpointID = ""
			case "invalid endpoint":
				data.ComposeEndpointID = "not-a-sha256"
			}
			f.coord.HandleDiscovery(dockerOwnerAgent, makeDockerDiscovery(data))
			f.direct("stack.down", target, http.StatusConflict)
			if strings.Contains(invalid, "capability") && !strings.Contains(f.response, "upgraded agent") {
				t.Fatalf("direct upgrade guidance hidden: %s", f.response)
			}
			f.queue("stack.down", target, http.StatusConflict)
			if strings.Contains(invalid, "capability") && !strings.Contains(f.response, "upgraded agent") {
				t.Fatalf("queue upgrade guidance hidden: %s", f.response)
			}
			dedicated(t, f, http.StatusBadRequest)
			if len(f.sent) != 0 {
				t.Fatal("invalid discovery still dispatched")
			}
		})
	}
	for _, timing := range []string{"queued order change", "final guard order change", "old queued job", "capability delta", "selection error delta", "endpoint change", "final guard endpoint change", "old endpoint binding", "dedicated final guard"} {
		t.Run(timing, func(t *testing.T) {
			f := newDockerActionOwnerFixture(t)
			f.action = "stack.down"
			job := f.queue("stack.down", target, http.StatusAccepted)
			switched := false
			replace := func() {
				data := discovery()
				slices.Reverse(data.ComposeStacks[0].ConfigFiles)
				delta := agentmgr.DockerDiscoveryDeltaData{ComposeActionVersion: 2, ComposeEndpointID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReplaceComposeStacks: true, ComposeStacks: data.ComposeStacks}
				if strings.Contains(timing, "endpoint change") {
					delta.ComposeStacks[0].ConfigFiles = slices.Clone(files)
					delta.ComposeEndpointID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
				} else if timing == "capability delta" {
					delta = agentmgr.DockerDiscoveryDeltaData{} // Old agent must revoke cached v2 support.
				} else if timing == "selection error delta" {
					delta.ComposeStacks[0].SelectionError = "inconsistent_labels"
				}
				raw, _ := json.Marshal(delta)
				f.coord.HandleDiscoveryDelta(dockerOwnerAgent, agentmgr.Message{Type: agentmgr.MsgDockerDiscoveryDelta, Data: raw})
				switched = true
			}
			switch timing {
			case "old queued job":
				delete(job.Params, binding)
			case "old endpoint binding":
				delete(job.Params, "docker_compose_endpoint_id")
			case "final guard order change", "final guard endpoint change", "dedicated final guard":
				deps := f.sut.ensureDockerDeps()
				evaluate := deps.EvaluateAssetGuardrails
				deps.EvaluateAssetGuardrails = func(id string, at time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
					guard, err := evaluate(id, at)
					if id == "docker-host-agent-a" {
						replace()
					}
					return guard, err
				}
			default:
				replace()
			}
			if timing == "dedicated final guard" {
				dedicated(t, f, http.StatusBadRequest)
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				result := f.sut.ensureProxmoxDeps().ExecuteActionInProcessContext(ctx, job)
				if result.Status != actions.StatusFailed {
					t.Fatalf("changed selection executed: %+v", result)
				}
			}
			if len(f.sent) != 0 || (timing != "old queued job" && timing != "old endpoint binding" && !switched) {
				t.Fatalf("selection boundary missed: sent=%d switched=%v", len(f.sent), switched)
			}
		})
	}
}
