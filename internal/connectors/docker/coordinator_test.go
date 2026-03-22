package docker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectorsdk"
)

type mockAgentCommander struct {
	sendFn func(assetID string, msg agentmgr.Message) error
}

func (m *mockAgentCommander) SendToAgent(assetID string, msg agentmgr.Message) error {
	if m.sendFn != nil {
		return m.sendFn(assetID, msg)
	}
	return nil
}

func makeDiscoveryMsg(data agentmgr.DockerDiscoveryData) agentmgr.Message {
	raw, _ := json.Marshal(data)
	return agentmgr.Message{Type: agentmgr.MsgDockerDiscovery, Data: raw}
}

func makeDiscoveryDeltaMsg(data agentmgr.DockerDiscoveryDeltaData) agentmgr.Message {
	raw, _ := json.Marshal(data)
	return agentmgr.Message{Type: agentmgr.MsgDockerDiscoveryDelta, Data: raw}
}

func makeStatsMsg(data agentmgr.DockerStatsData) agentmgr.Message {
	raw, _ := json.Marshal(data)
	return agentmgr.Message{Type: agentmgr.MsgDockerStats, Data: raw}
}

func TestCoordinatorDiscoverAggregatesHosts(t *testing.T) {
	coord := NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Engine: agentmgr.DockerEngineInfo{Version: "24.0.7", OS: "linux", Arch: "amd64"},
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123456789ab", Name: "nginx", Image: "nginx:1.25", State: "running", Status: "Up 3 days"},
		},
		ComposeStacks: []agentmgr.DockerComposeStack{
			{Name: "webstack", Status: "running(1)", ConfigFile: "/opt/stacks/web/docker-compose.yml", Containers: []string{"nginx"}},
		},
	}))

	assets, err := coord.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Expect: 1 container-host + 1 docker-container + 1 compose-stack = 3 assets
	if len(assets) != 3 {
		t.Errorf("expected 3 assets, got %d", len(assets))
		for _, a := range assets {
			t.Logf("  asset: %s (%s) %s", a.ID, a.Type, a.Name)
		}
	}

	// Verify container has correct metadata
	for _, a := range assets {
		if a.Type == "docker-container" {
			if a.Metadata["image"] != "nginx:1.25" {
				t.Errorf("container image = %q, want nginx:1.25", a.Metadata["image"])
			}
			if a.Metadata["state"] != "running" {
				t.Errorf("container state = %q, want running", a.Metadata["state"])
			}
		}
	}
}

func TestCoordinatorHandleStats(t *testing.T) {
	coord := NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID:     "agent-01",
		Containers: []agentmgr.DockerContainerInfo{{ID: "abc123", Name: "nginx"}},
	}))

	coord.HandleStats("agent-01", makeStatsMsg(agentmgr.DockerStatsData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerStats{
			{ID: "abc123", CPUPercent: 45.2, MemoryPercent: 25.0},
		},
	}))

	coord.mu.RLock()
	host := coord.hosts["agent-01"]
	stats := host.Stats["abc123"]
	coord.mu.RUnlock()

	if stats.CPUPercent != 45.2 {
		t.Errorf("CPUPercent = %f, want 45.2", stats.CPUPercent)
	}
}

func TestCoordinatorHandleStatsReplacesPreviousSnapshot(t *testing.T) {
	coord := NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123", Name: "nginx"},
		},
	}))

	coord.HandleStats("agent-01", makeStatsMsg(agentmgr.DockerStatsData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerStats{
			{ID: "abc123", CPUPercent: 10.1},
		},
	}))

	if _, ok := coord.GetContainerStats("agent-01", "abc123"); !ok {
		t.Fatal("expected first stats sample to exist")
	}

	coord.HandleStats("agent-01", makeStatsMsg(agentmgr.DockerStatsData{
		HostID:     "agent-01",
		Containers: []agentmgr.DockerContainerStats{},
	}))

	if _, ok := coord.GetContainerStats("agent-01", "abc123"); ok {
		t.Fatal("expected stale stats to be removed after empty stats snapshot")
	}
}

func TestCoordinatorHandleDiscoveryDeltaAppliesContainerDiff(t *testing.T) {
	coord := NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{
			{
				ID:     "ct-old",
				Name:   "old",
				Image:  "alpine:3.20",
				State:  "running",
				Status: "Up",
				Labels: map[string]string{"com.docker.compose.project": "stack-a"},
			},
			{
				ID:     "ct-keep",
				Name:   "keep",
				Image:  "redis:7",
				State:  "running",
				Status: "Up",
			},
		},
		ComposeStacks: []agentmgr.DockerComposeStack{
			{Name: "stack-a", Status: "running(1)", Containers: []string{"old"}},
		},
	}))
	coord.HandleStats("agent-01", makeStatsMsg(agentmgr.DockerStatsData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerStats{
			{ID: "ct-old", CPUPercent: 10},
			{ID: "ct-keep", CPUPercent: 20},
		},
	}))

	coord.HandleDiscoveryDelta("agent-01", makeDiscoveryDeltaMsg(agentmgr.DockerDiscoveryDeltaData{
		HostID:             "agent-01",
		RemoveContainerIDs: []string{"ct-old"},
		UpsertContainers: []agentmgr.DockerContainerInfo{
			{
				ID:     "ct-new",
				Name:   "new",
				Image:  "nginx:1.27",
				State:  "running",
				Status: "Up",
				Labels: map[string]string{"com.docker.compose.project": "stack-b"},
			},
		},
		ReplaceComposeStacks: true,
		ComposeStacks: []agentmgr.DockerComposeStack{
			{Name: "stack-b", Status: "running(1)", Containers: []string{"new"}},
		},
	}))

	host, ok := coord.GetHost("agent-01")
	if !ok {
		t.Fatal("expected host to exist")
	}
	if len(host.Containers) != 2 {
		t.Fatalf("expected 2 containers after delta, got %d", len(host.Containers))
	}
	var haveNew, haveKeep bool
	for _, ct := range host.Containers {
		if ct.ID == "ct-new" {
			haveNew = true
		}
		if ct.ID == "ct-keep" {
			haveKeep = true
		}
		if ct.ID == "ct-old" {
			t.Fatalf("removed container still present after delta")
		}
	}
	if !haveNew || !haveKeep {
		t.Fatalf("expected both ct-new and ct-keep to exist after delta")
	}
	if _, ok := host.Stats["ct-old"]; ok {
		t.Fatalf("expected stats for removed container to be pruned")
	}
	if len(host.ComposeStacks) != 1 || host.ComposeStacks[0].Name != "stack-b" {
		t.Fatalf("compose stacks not replaced by delta: %+v", host.ComposeStacks)
	}
}

func TestCoordinatorHandleDiscoveryDeltaCreatesHostWhenMissing(t *testing.T) {
	coord := NewCoordinator(nil)
	coord.HandleDiscoveryDelta("agent-99", makeDiscoveryDeltaMsg(agentmgr.DockerDiscoveryDeltaData{
		HostID: "agent-99",
		Engine: &agentmgr.DockerEngineInfo{
			Version:    "26.1.0",
			APIVersion: "1.45",
			OS:         "linux",
			Arch:       "amd64",
		},
		UpsertContainers: []agentmgr.DockerContainerInfo{
			{ID: "ct-1", Name: "standalone", State: "running"},
		},
	}))

	host, ok := coord.GetHost("agent-99")
	if !ok {
		t.Fatal("expected host to be created")
	}
	if host.Engine.Version != "26.1.0" {
		t.Fatalf("engine version = %q, want 26.1.0", host.Engine.Version)
	}
	if len(host.Containers) != 1 || host.Containers[0].ID != "ct-1" {
		t.Fatalf("unexpected containers after bootstrap delta: %+v", host.Containers)
	}
}

func TestCoordinatorListHostsReturnsDetachedCopies(t *testing.T) {
	coord := NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{
			{ID: "abc123456789ab", Name: "nginx", Labels: map[string]string{"tier": "web"}},
		},
	}))
	coord.HandleStats("agent-01", makeStatsMsg(agentmgr.DockerStatsData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerStats{
			{ID: "abc123456789ab", CPUPercent: 22.2},
		},
	}))

	first := coord.ListHosts()
	if len(first) != 1 {
		t.Fatalf("expected one host snapshot, got %d", len(first))
	}
	first[0].Containers[0].Name = "mutated"
	first[0].Containers[0].Labels["tier"] = "mutated"
	first[0].Stats["abc123456789ab"] = ContainerStats{CPUPercent: 0}

	second := coord.ListHosts()
	if len(second) != 1 {
		t.Fatalf("expected one host snapshot, got %d", len(second))
	}
	if second[0].Containers[0].Name != "nginx" {
		t.Fatalf("host snapshot should be detached copy, got container name %q", second[0].Containers[0].Name)
	}
	if second[0].Containers[0].Labels["tier"] != "web" {
		t.Fatalf("label mutation leaked into coordinator state")
	}
	stats := second[0].Stats["abc123456789ab"]
	if stats.CPUPercent != 22.2 {
		t.Fatalf("stats mutation leaked into coordinator state: %f", stats.CPUPercent)
	}
}

func TestCoordinatorMultipleHosts(t *testing.T) {
	coord := NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID:     "agent-01",
		Containers: []agentmgr.DockerContainerInfo{{ID: "ct1", Name: "nginx"}},
	}))
	coord.HandleDiscovery("agent-02", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID:     "agent-02",
		Containers: []agentmgr.DockerContainerInfo{{ID: "ct2", Name: "redis"}, {ID: "ct3", Name: "postgres"}},
	}))

	assets, _ := coord.Discover(context.Background())
	// 2 hosts + 3 containers = 5 assets
	if len(assets) != 5 {
		t.Errorf("expected 5 assets, got %d", len(assets))
	}
}

func TestCoordinatorRemoveHost(t *testing.T) {
	coord := NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID:     "agent-01",
		Containers: []agentmgr.DockerContainerInfo{{ID: "ct1", Name: "nginx"}},
	}))

	coord.RemoveHost("agent-01")

	assets, _ := coord.Discover(context.Background())
	if len(assets) != 0 {
		t.Errorf("expected 0 assets after remove, got %d", len(assets))
	}
}

func TestCoordinatorExecuteActionRoutes(t *testing.T) {
	sent := make(chan agentmgr.Message, 1)
	mock := &mockAgentCommander{sendFn: func(id string, msg agentmgr.Message) error {
		sent <- msg
		return nil
	}}

	coord := NewCoordinator(mock)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID:     "agent-01",
		Containers: []agentmgr.DockerContainerInfo{{ID: "abc123456789ab", Name: "nginx", State: "running"}},
	}))

	// Execute restart in background and simulate result
	go func() {
		msg := <-sent
		if msg.Type != agentmgr.MsgDockerAction {
			t.Errorf("expected docker.action, got %s", msg.Type)
		}
		var req agentmgr.DockerActionData
		json.Unmarshal(msg.Data, &req)

		// Send result back
		resultData := agentmgr.DockerActionResultData{
			RequestID: req.RequestID,
			Success:   true,
		}
		raw, _ := json.Marshal(resultData)
		coord.HandleActionResult("agent-01", agentmgr.Message{Type: agentmgr.MsgDockerActionResult, Data: raw})
	}()

	result, err := coord.ExecuteAction(context.Background(), "container.restart", connectorsdk.ActionRequest{
		TargetID: "docker-ct-agent-01-abc123456789",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" {
		t.Errorf("status = %q, want succeeded", result.Status)
	}
}

func TestCoordinatorTestConnection(t *testing.T) {
	coord := NewCoordinator(nil)
	health, err := coord.TestConnection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != "ok" {
		t.Errorf("status = %q, want ok", health.Status)
	}
}

func TestNormalizeID(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"agent-01", "agent-01"},
		{"Agent.01", "agent-01"},
		{"my host", "my-host"},
	}
	for _, tt := range tests {
		got := normalizeID(tt.input)
		if got != tt.want {
			t.Errorf("normalizeID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCoordinatorExecuteActionContainerCreateUsesHostTarget(t *testing.T) {
	sent := make(chan agentmgr.Message, 1)
	mock := &mockAgentCommander{sendFn: func(id string, msg agentmgr.Message) error {
		sent <- msg
		return nil
	}}

	coord := NewCoordinator(mock)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
	}))

	go func() {
		msg := <-sent
		if msg.Type != agentmgr.MsgDockerAction {
			t.Errorf("expected docker.action, got %s", msg.Type)
		}
		var req agentmgr.DockerActionData
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			t.Errorf("failed to unmarshal action data: %v", err)
			return
		}
		if req.Action != "container.create" {
			t.Errorf("action = %q, want container.create", req.Action)
		}
		if req.ContainerID != "" {
			t.Errorf("container_id = %q, want empty for host-target action", req.ContainerID)
		}
		if req.Params["image"] != "nginx:latest" {
			t.Errorf("image param = %q, want nginx:latest", req.Params["image"])
		}

		resultData := agentmgr.DockerActionResultData{
			RequestID: req.RequestID,
			Success:   true,
			Data:      "new-container-id",
		}
		raw, _ := json.Marshal(resultData)
		coord.HandleActionResult("agent-01", agentmgr.Message{Type: agentmgr.MsgDockerActionResult, Data: raw})
	}()

	result, err := coord.ExecuteAction(context.Background(), "container.create", connectorsdk.ActionRequest{
		TargetID: "docker-host-agent-01",
		Params: map[string]string{
			"image": "nginx:latest",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", result.Status)
	}
	if result.Output != "new-container-id" {
		t.Fatalf("output = %q, want new-container-id", result.Output)
	}
}

func TestCoordinatorExecuteActionStackDeployUsesHostTarget(t *testing.T) {
	sent := make(chan agentmgr.Message, 1)
	mock := &mockAgentCommander{sendFn: func(id string, msg agentmgr.Message) error {
		sent <- msg
		return nil
	}}

	coord := NewCoordinator(mock)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
	}))

	go func() {
		msg := <-sent
		if msg.Type != agentmgr.MsgDockerComposeAction {
			t.Errorf("expected docker.compose.action, got %s", msg.Type)
		}
		var req agentmgr.DockerComposeActionData
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			t.Errorf("failed to unmarshal compose action data: %v", err)
			return
		}
		if req.Action != "deploy" {
			t.Errorf("action = %q, want deploy", req.Action)
		}
		if req.StackName != "demo" {
			t.Errorf("stack_name = %q, want demo", req.StackName)
		}
		if !strings.Contains(req.ComposeYAML, "services:") {
			t.Errorf("compose yaml missing services stanza: %q", req.ComposeYAML)
		}

		resultData := agentmgr.DockerComposeResultData{
			RequestID: req.RequestID,
			Success:   true,
			Output:    "deployed",
		}
		raw, _ := json.Marshal(resultData)
		coord.HandleComposeResult("agent-01", agentmgr.Message{Type: agentmgr.MsgDockerComposeResult, Data: raw})
	}()

	result, err := coord.ExecuteAction(context.Background(), "stack.deploy", connectorsdk.ActionRequest{
		TargetID: "agent-01",
		Params: map[string]string{
			"stack_name":   "demo",
			"compose_yaml": "services:\n  web:\n    image: nginx:latest\n",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", result.Status)
	}
}

func TestCoordinatorExecuteActionStackDeployMissingStackName(t *testing.T) {
	coord := NewCoordinator(&mockAgentCommander{})
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
	}))

	result, err := coord.ExecuteAction(context.Background(), "stack.deploy", connectorsdk.ActionRequest{
		TargetID: "agent-01",
		Params: map[string]string{
			"compose_yaml": "services: {}",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if !strings.Contains(result.Message, "stack_name is required") {
		t.Fatalf("message = %q, want stack_name validation", result.Message)
	}
}
