package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
)

const dockerOtherContainer = "aaaaaaaaaaaabbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// Colliding short IDs and normalized names must fail before any agent RPC,
// whether reached directly, through admission, or through the coordinator.
func TestDockerActionsRejectAmbiguousResources(t *testing.T) {
	for _, tc := range []struct {
		name, action, target string
		collide              func(*dockerActionOwnerFixture)
	}{
		{"container host normalization", "container.stop", "docker-ct-agent-a-aaaaaaaaaaaa", func(f *dockerActionOwnerFixture) { f.discover("agent-a") }},
		{"stack host normalization", "stack.down", "docker-stack-agent-a-lab", func(f *dockerActionOwnerFixture) { f.discover("agent-a") }},
		{"host normalization", "image.pull", dockerOwnerAgent, func(f *dockerActionOwnerFixture) { f.discover("agent-a") }},
		{"host alias overlap", "image.pull", "docker-host-agent-a", func(f *dockerActionOwnerFixture) { f.discover("docker-host-agent-a") }},
		{"same host container prefix", "container.stop", "docker-ct-agent-a-aaaaaaaaaaaa", func(f *dockerActionOwnerFixture) {
			f.coord.HandleDiscovery(dockerOwnerAgent, makeDockerDiscovery(agentmgr.DockerDiscoveryData{Containers: []agentmgr.DockerContainerInfo{
				{ID: dockerOwnerContainer}, {ID: dockerOtherContainer},
			}}))
		}},
		{"same host stack directory", "stack.down", "docker-stack-agent-a-lab", func(f *dockerActionOwnerFixture) {
			f.coord.HandleDiscovery(dockerOwnerAgent, makeDockerDiscovery(agentmgr.DockerDiscoveryData{ComposeActionVersion: 2, ComposeEndpointID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ComposeStacks: []agentmgr.DockerComposeStack{
				{Name: "lab", ConfigFile: "/srv/lab/compose.yaml", ConfigDir: "/srv/lab", ConfigFiles: []string{"/srv/shared/base.yaml", "/srv/lab/override.yml"}}, {Name: "lab", ConfigFile: "/srv/other/compose.yaml", ConfigDir: "/srv/other", ConfigFiles: []string{"/srv/shared/base.yaml", "/srv/lab/override.yml"}},
			}}))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDockerActionOwnerFixture(t)
			f.action = tc.action
			tc.collide(f)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result, err := f.coord.ExecuteAction(ctx, tc.action, connectorsdk.ActionRequest{TargetID: tc.target})
			if err != nil || result.Status != "failed" || !strings.Contains(result.Message, "ambiguous") {
				t.Fatalf("coordinator did not reject ambiguous target: %+v err=%v", result, err)
			}
			f.direct(tc.action, tc.target, http.StatusConflict)
			f.queue(tc.action, tc.target, http.StatusConflict)
			if len(f.sent) != 0 {
				t.Fatal("ambiguous target dispatched an agent RPC")
			}
			f.coord.RemoveHost("agent-a")
			f.coord.RemoveHost("docker-host-agent-a")
			f.discover(dockerOwnerAgent)
			job := f.queue(tc.action, tc.target, http.StatusAccepted)
			if result := f.sut.ensureProxmoxDeps().ExecuteActionInProcessContext(ctx, job); result.Status != actions.StatusSucceeded || len(f.sent) != 1 {
				t.Fatalf("unique target positive control failed: %+v sent=%d", result, len(f.sent))
			}
		})
	}
}

// Keep the same agent and logical asset while replacing its full container ID,
// native stack name, or working directory. Both queue wait and final guard races
// must preserve the originally approved tuple, not merely its short alias.
func TestDockerConfiguredActionKeepsQueuedResource(t *testing.T) {
	for _, tc := range []struct {
		name, action, target, binding, value string
		replacement                          agentmgr.DockerDiscoveryData
	}{
		{"container", "container.stop", "docker-ct-agent-a-aaaaaaaaaaaa", "docker_container_id", dockerOwnerContainer,
			agentmgr.DockerDiscoveryData{Containers: []agentmgr.DockerContainerInfo{{ID: dockerOtherContainer}}}},
		{"stack name", "stack.down", "docker-stack-agent-a-lab", "docker_stack_name", "lab",
			agentmgr.DockerDiscoveryData{ComposeActionVersion: 2, ComposeEndpointID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ComposeStacks: []agentmgr.DockerComposeStack{{Name: "LAB", ConfigFile: "/srv/lab/compose.yaml", ConfigDir: "/srv/lab", ConfigFiles: []string{"/srv/shared/base.yaml", "/srv/lab/override.yml"}}}}},
		{"stack directory", "stack.down", "docker-stack-agent-a-lab", "docker_config_dir", "/srv/lab",
			agentmgr.DockerDiscoveryData{ComposeActionVersion: 2, ComposeEndpointID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ComposeStacks: []agentmgr.DockerComposeStack{{Name: "lab", ConfigFile: "/srv/other/compose.yaml", ConfigDir: "/srv/other", ConfigFiles: []string{"/srv/shared/base.yaml", "/srv/lab/override.yml"}}}}},
	} {
		for _, timing := range []string{"before worker", "during final guard", "old agent-only binding"} {
			t.Run(tc.name+"/"+timing, func(t *testing.T) {
				f := newDockerActionOwnerFixture(t)
				f.action = tc.action
				job := f.queue(tc.action, tc.target, http.StatusAccepted)
				if value, present := job.Params[tc.binding]; !present || value != tc.value {
					t.Fatalf("queue did not overwrite forged resource binding: %+v", job.Params)
				}
				switched := false
				replace := func() {
					f.coord.HandleDiscovery(dockerOwnerAgent, makeDockerDiscovery(tc.replacement))
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
				case "old agent-only binding":
					delete(job.Params, tc.binding)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				result := f.sut.ensureProxmoxDeps().ExecuteActionInProcessContext(ctx, job)
				if result.Status != actions.StatusFailed || len(f.sent) != 0 {
					t.Fatalf("saved resource owner was bypassed: %+v sent=%d", result, len(f.sent))
				}
				diagnostic := result.Error
				for _, step := range result.Steps {
					diagnostic += " " + step.Error
				}
				want := "resource identity"
				if timing == "during final guard" {
					want = "authorized docker resource"
				} else if timing == "old agent-only binding" {
					want = "resubmit"
				}
				if !strings.Contains(diagnostic, want) || (timing != "old agent-only binding" && !switched) {
					t.Fatalf("wrong rejection or race not exercised: %+v switched=%v", result, switched)
				}
				if timing != "old agent-only binding" && job.Params[tc.binding] != tc.value {
					t.Fatal("failed execution changed the saved resource binding")
				}
			})
		}
	}
}
