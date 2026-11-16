package main

import (
	"net/http"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/connectors/docker"
)

// TestDockerRoutesReachHandlersThroughRouteTable sends one signed-in admin
// request to each Docker route through the real route table and auth, and
// checks the answer only that route's handler gives.
//
// The Docker handler tests live in internal/hubapi/dockerpkg and build their
// Deps with testutil/hubdeps. This test catches a fault in cmd's own wiring
// that those tests cannot see: a route bound to the wrong handler, or
// buildDockerDeps leaving out the Docker coordinator or the action runner.
// The coordinator has no agent manager, so the stack action fails before
// anything is sent to a device.
func TestDockerRoutesReachHandlersThroughRouteTable(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.dockerCoordinator = docker.NewCoordinator(nil)
	sut.dockerCoordinator.HandleDiscovery("node-1", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "node-1", Engine: agentmgr.DockerEngineInfo{Version: "24.0.7"}, ComposeActionVersion: 2,
		ComposeEndpointID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Containers:        []agentmgr.DockerContainerInfo{{ID: "abc123def456ab", Name: "wiring-web", Image: "nginx:latest", State: "running"}},
		ComposeStacks: []agentmgr.DockerComposeStack{{
			Name: "webstack", Status: "running(1)", ConfigFile: "/opt/stacks/web/docker-compose.yml",
			ConfigDir: "/opt/stacks/web", ConfigFiles: []string{"/opt/stacks/web/docker-compose.yml"}, Containers: []string{},
		}},
	}))
	admin := createRoleSessionToken(t, sut, "admin-docker-wiring", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	call("/api/v1/docker/hosts", http.MethodGet, "/api/v1/docker/hosts", "", http.StatusOK, `"engine_version":"24.0.7"`)
	call("/api/v1/docker/hosts/", http.MethodGet, "/api/v1/docker/hosts/node-1/stacks", "", http.StatusOK, `"name":"webstack"`)
	call("/api/v1/docker/containers/", http.MethodGet, "/api/v1/docker/containers/docker-ct-node-1-abc123def456", "", http.StatusOK, `"wiring-web"`)
	call("/api/v1/docker/stacks/", http.MethodPost, "/api/v1/docker/stacks/docker-stack-node-1-webstack/action", `{"action":"stack.restart"}`, http.StatusBadRequest, `"status":"failed"`)
}
