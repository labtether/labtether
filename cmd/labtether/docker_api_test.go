package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectors/docker"
)

// makeDockerDiscovery constructs a discovery message from the given data.
func makeDockerDiscovery(data agentmgr.DockerDiscoveryData) agentmgr.Message {
	raw, _ := json.Marshal(data)
	return agentmgr.Message{Type: agentmgr.MsgDockerDiscovery, Data: raw}
}

func TestHandleDockerHostsEmpty(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts", nil)
	w := httptest.NewRecorder()
	srv.handleDockerHosts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	hosts, ok := resp["hosts"]
	if !ok {
		t.Fatal("response missing 'hosts' key")
	}
	list, ok := hosts.([]any)
	if !ok {
		t.Fatalf("hosts is not a list, got %T", hosts)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 hosts, got %d", len(list))
	}
}

func TestHandleDockerHostsNilCoordinator(t *testing.T) {
	srv := &apiServer{dockerCoordinator: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts", nil)
	w := httptest.NewRecorder()
	srv.handleDockerHosts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	hosts, ok := resp["hosts"]
	if !ok {
		t.Fatal("response missing 'hosts' key")
	}
	list, ok := hosts.([]any)
	if !ok {
		t.Fatalf("hosts is not a list, got %T", hosts)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 hosts, got %d", len(list))
	}
}

func TestHandleDockerHostsMethodNotAllowed(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/docker/hosts", nil)
	w := httptest.NewRecorder()
	srv.handleDockerHosts(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleDockerHostsListOne(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Engine: agentmgr.DockerEngineInfo{Version: "24.0.7", OS: "linux", Arch: "amd64"},
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123def456ab", Name: "nginx", Image: "nginx:latest", State: "running"},
		},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts", nil)
	w := httptest.NewRecorder()
	srv.handleDockerHosts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	hosts, ok := resp["hosts"].([]any)
	if !ok {
		t.Fatal("hosts is not a list")
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}

	host, ok := hosts[0].(map[string]any)
	if !ok {
		t.Fatal("host entry is not a map")
	}
	if host["agent_id"] != "agent-01" {
		t.Errorf("agent_id = %q, want agent-01", host["agent_id"])
	}
	if host["engine_version"] != "24.0.7" {
		t.Errorf("engine_version = %q, want 24.0.7", host["engine_version"])
	}
	if containerCount, ok := host["container_count"].(float64); !ok || containerCount != 1 {
		t.Errorf("container_count = %v, want 1", host["container_count"])
	}
}

func TestHandleDockerHostDetail(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Engine: agentmgr.DockerEngineInfo{Version: "24.0.7"},
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123def456ab", Name: "nginx", Image: "nginx:latest", State: "running"},
		},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts/agent-01", nil)
	req.URL.Path = "/api/v1/docker/hosts/agent-01"
	w := httptest.NewRecorder()
	srv.handleDockerHostActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp["host"]; !ok {
		t.Error("response missing 'host' key")
	}
}

func TestHandleDockerHostNotFound(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts/nonexistent", nil)
	req.URL.Path = "/api/v1/docker/hosts/nonexistent"
	w := httptest.NewRecorder()
	srv.handleDockerHostActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleDockerHostContainersSub(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123def456ab", Name: "nginx", State: "running"},
			{ID: "def456abc123de", Name: "redis", State: "running"},
		},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts/agent-01/containers", nil)
	req.URL.Path = "/api/v1/docker/hosts/agent-01/containers"
	w := httptest.NewRecorder()
	srv.handleDockerHostActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	containers, ok := resp["containers"].([]any)
	if !ok {
		t.Fatal("containers is not a list")
	}
	if len(containers) != 2 {
		t.Errorf("expected 2 containers, got %d", len(containers))
	}
}

func TestHandleDockerHostContainersSubRejectsLegacyDockerPrefixAlias(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123def456ab", Name: "nginx", State: "running"},
			{ID: "def456abc123de", Name: "redis", State: "running"},
		},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts/docker-agent-01/containers", nil)
	req.URL.Path = "/api/v1/docker/hosts/docker-agent-01/containers"
	w := httptest.NewRecorder()
	srv.handleDockerHostActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDockerHostDetailDockerHostAssetAlias(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Engine: agentmgr.DockerEngineInfo{Version: "24.0.7"},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts/docker-host-agent-01", nil)
	req.URL.Path = "/api/v1/docker/hosts/docker-host-agent-01"
	w := httptest.NewRecorder()
	srv.handleDockerHostActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDockerHostStacksSub(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		ComposeStacks: []agentmgr.DockerComposeStack{
			{Name: "webstack", Status: "running(1)", ConfigFile: "/opt/stacks/web/docker-compose.yml"},
		},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts/agent-01/stacks", nil)
	req.URL.Path = "/api/v1/docker/hosts/agent-01/stacks"
	w := httptest.NewRecorder()
	srv.handleDockerHostActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	stacks, ok := resp["stacks"].([]any)
	if !ok {
		t.Fatal("stacks is not a list")
	}
	if len(stacks) != 1 {
		t.Errorf("expected 1 stack, got %d", len(stacks))
	}
}

func TestHandleDockerContainerDetail(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123def456ab", Name: "nginx", Image: "nginx:latest", State: "running"},
		},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	// docker-ct-agent-01-abc123def456 (12-char prefix of abc123def456ab)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/containers/docker-ct-agent-01-abc123def456", nil)
	req.URL.Path = "/api/v1/docker/containers/docker-ct-agent-01-abc123def456"
	w := httptest.NewRecorder()
	srv.handleDockerContainerActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp["container"]; !ok {
		t.Error("response missing 'container' key")
	}
	if resp["agent_id"] != "agent-01" {
		t.Errorf("agent_id = %q, want agent-01", resp["agent_id"])
	}
}

func TestHandleDockerContainerNotFound(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/containers/docker-ct-agent-01-notexist", nil)
	req.URL.Path = "/api/v1/docker/containers/docker-ct-agent-01-notexist"
	w := httptest.NewRecorder()
	srv.handleDockerContainerActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleDockerContainerStats(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123def456ab", Name: "nginx", State: "running"},
		},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/containers/docker-ct-agent-01-abc123def456/stats", nil)
	req.URL.Path = "/api/v1/docker/containers/docker-ct-agent-01-abc123def456/stats"
	w := httptest.NewRecorder()
	srv.handleDockerContainerActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	// No stats available yet — should return null stats.
	if _, ok := resp["stats"]; !ok {
		t.Error("response missing 'stats' key")
	}
}

func TestHandleDockerContainerActionNoAgent(t *testing.T) {
	// With agentMgr=nil, ExecuteAction returns a failed result (not an error),
	// so the handler should return 400 with the failed status.
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123def456ab", Name: "nginx", State: "running"},
		},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	body := `{"action": "container.restart"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docker/containers/docker-ct-agent-01-abc123def456/action",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.URL.Path = "/api/v1/docker/containers/docker-ct-agent-01-abc123def456/action"
	w := httptest.NewRecorder()
	srv.handleDockerContainerActions(w, req)

	// With nil agent manager, coordinator returns a failed ActionResult (not an error).
	// The handler maps "failed" status -> 400.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when agent not available, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatal("response missing 'result' key")
	}
	if result["status"] != "failed" {
		t.Errorf("result.status = %q, want failed", result["status"])
	}
}

func TestHandleDockerContainerActionMissingAction(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123def456ab", Name: "nginx", State: "running"},
		},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	body := `{"action": ""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docker/containers/docker-ct-agent-01-abc123def456/action",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.URL.Path = "/api/v1/docker/containers/docker-ct-agent-01-abc123def456/action"
	w := httptest.NewRecorder()
	srv.handleDockerContainerActions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing action, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDockerStackActionNoAgent(t *testing.T) {
	// With agentMgr=nil, ExecuteAction returns a failed result.
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		ComposeStacks: []agentmgr.DockerComposeStack{
			{Name: "webstack", Status: "running(1)", ConfigFile: "/opt/stacks/web/docker-compose.yml", Containers: []string{}},
		},
	}))

	srv := &apiServer{dockerCoordinator: coord}

	body := `{"action": "stack.restart"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docker/stacks/docker-stack-agent-01-webstack/action",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.URL.Path = "/api/v1/docker/stacks/docker-stack-agent-01-webstack/action"
	w := httptest.NewRecorder()
	srv.handleDockerStackActions(w, req)

	// With nil agent manager, the coordinator returns a failed ActionResult.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when agent not available, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatal("response missing 'result' key")
	}
	if result["status"] != "failed" {
		t.Errorf("result.status = %q, want failed", result["status"])
	}
}

func TestHandleDockerStackActionMissingSubResource(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/docker/stacks/some-stack", nil)
	req.URL.Path = "/api/v1/docker/stacks/some-stack"
	w := httptest.NewRecorder()
	srv.handleDockerStackActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleDockerHostUnknownSubResource(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
	}))

	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/hosts/agent-01/unknown", nil)
	req.URL.Path = "/api/v1/docker/hosts/agent-01/unknown"
	w := httptest.NewRecorder()
	srv.handleDockerHostActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleDockerHostActionNoAgent(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
	}))
	srv := &apiServer{dockerCoordinator: coord}

	body := `{"action":"container.create","params":{"image":"nginx:latest"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docker/hosts/agent-01/action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.URL.Path = "/api/v1/docker/hosts/agent-01/action"
	w := httptest.NewRecorder()
	srv.handleDockerHostActions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatal("response missing 'result' key")
	}
	if result["status"] != "failed" {
		t.Fatalf("result.status = %v, want failed", result["status"])
	}
}

func TestHandleDockerHostActionMissingAction(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
	}))
	srv := &apiServer{dockerCoordinator: coord}

	body := `{"action":" "}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docker/hosts/agent-01/action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.URL.Path = "/api/v1/docker/hosts/agent-01/action"
	w := httptest.NewRecorder()
	srv.handleDockerHostActions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDockerContainerLogsNoAgent(t *testing.T) {
	coord := docker.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123def456ab", Name: "nginx", State: "running"},
		},
	}))
	srv := &apiServer{dockerCoordinator: coord}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/containers/docker-ct-agent-01-abc123def456/logs?tail=100", nil)
	req.URL.Path = "/api/v1/docker/containers/docker-ct-agent-01-abc123def456/logs"
	w := httptest.NewRecorder()
	srv.handleDockerContainerActions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "agent manager not configured") {
		t.Fatalf("expected response to include agent-manager error, got: %s", w.Body.String())
	}
}
