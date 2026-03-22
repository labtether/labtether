package docker

import (
	"strconv"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestInferComposeStacks(t *testing.T) {
	containers := []DockerContainer{
		{
			Names: []string{"/nginx"},
			State: "running",
			Labels: map[string]string{
				"com.docker.compose.project":             "webstack",
				"com.docker.compose.project.working_dir": "/opt/stacks/web",
			},
		},
		{
			Names: []string{"/redis"},
			State: "running",
			Labels: map[string]string{
				"com.docker.compose.project":             "webstack",
				"com.docker.compose.project.working_dir": "/opt/stacks/web",
			},
		},
		{
			Names:  []string{"/standalone"},
			State:  "running",
			Labels: map[string]string{},
		},
	}

	stacks := inferComposeStacks(containers)
	if len(stacks) != 1 {
		t.Fatalf("expected 1 stack, got %d", len(stacks))
	}
	if stacks[0].Name != "webstack" {
		t.Errorf("stack name = %q, want %q", stacks[0].Name, "webstack")
	}
	if len(stacks[0].Containers) != 2 {
		t.Errorf("expected 2 containers in webstack, got %d", len(stacks[0].Containers))
	}
	if stacks[0].ConfigFile != "/opt/stacks/web/docker-compose.yml" {
		t.Errorf("config file = %q, want %q", stacks[0].ConfigFile, "/opt/stacks/web/docker-compose.yml")
	}
	if stacks[0].Status != "running(2)" {
		t.Errorf("status = %q, want %q", stacks[0].Status, "running(2)")
	}
}

func TestInferComposeStacksEmpty(t *testing.T) {
	stacks := inferComposeStacks(nil)
	if len(stacks) != 0 {
		t.Errorf("expected 0 stacks for nil input, got %d", len(stacks))
	}
}

func TestInferComposeStacksNoLabels(t *testing.T) {
	containers := []DockerContainer{
		{Names: []string{"/app"}, State: "running", Labels: map[string]string{}},
	}
	stacks := inferComposeStacks(containers)
	if len(stacks) != 0 {
		t.Errorf("expected 0 stacks for containers without compose labels, got %d", len(stacks))
	}
}

func TestDockerCollectorIsAvailable(t *testing.T) {
	collector := NewDockerCollector("/nonexistent/docker.sock", nil, "test-agent", 30*time.Second)
	if collector.IsAvailable() {
		t.Error("expected collector to be unavailable for nonexistent socket")
	}
}

func TestContainerName(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{[]string{"/nginx"}, "nginx"},
		{[]string{"/my-app"}, "my-app"},
		{[]string{}, ""},
		{nil, ""},
	}
	for _, tt := range tests {
		got := ContainerName(tt.names)
		if got != tt.want {
			t.Errorf("ContainerName(%v) = %q, want %q", tt.names, got, tt.want)
		}
	}
}

func BenchmarkInferComposeStacksLarge(b *testing.B) {
	const (
		projectsPerHost   = 50
		containersPerProj = 20
	)
	containers := make([]DockerContainer, 0, projectsPerHost*containersPerProj)
	for project := 0; project < projectsPerHost; project++ {
		projectName := "stack-" + strconv.Itoa(project)
		for index := 0; index < containersPerProj; index++ {
			state := "exited"
			if index%3 != 0 {
				state = "running"
			}
			containers = append(containers, DockerContainer{
				Names: []string{"/" + projectName + "-" + strconv.Itoa(index)},
				State: state,
				Labels: map[string]string{
					"com.docker.compose.project":             projectName,
					"com.docker.compose.project.working_dir": "/opt/" + projectName,
				},
			})
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = inferComposeStacks(containers)
	}
}

func TestDiffContainerInfoMap(t *testing.T) {
	previous := map[string]agentmgr.DockerContainerInfo{
		"ct-1": {ID: "ct-1", Name: "alpha", State: "running"},
		"ct-2": {ID: "ct-2", Name: "beta", State: "running"},
	}
	next := map[string]agentmgr.DockerContainerInfo{
		"ct-1": {ID: "ct-1", Name: "alpha", State: "exited"},  // changed
		"ct-3": {ID: "ct-3", Name: "gamma", State: "running"}, // added
	}

	upserts, removals := diffContainerInfoMap(previous, next)
	if len(upserts) != 2 {
		t.Fatalf("expected 2 upserts, got %d", len(upserts))
	}
	if upserts[0].ID != "ct-1" || upserts[1].ID != "ct-3" {
		t.Fatalf("unexpected upsert ordering: %+v", upserts)
	}
	if len(removals) != 1 || removals[0] != "ct-2" {
		t.Fatalf("unexpected removals: %+v", removals)
	}
}

func TestShouldFallbackToFull(t *testing.T) {
	tests := []struct {
		name          string
		changeCount   int
		previousCount int
		compose       bool
		want          bool
	}{
		{name: "no previous", changeCount: 10, previousCount: 0, compose: false, want: false},
		{name: "huge churn", changeCount: 220, previousCount: 400, compose: false, want: true},
		{name: "majority changed", changeCount: 60, previousCount: 100, compose: false, want: true},
		{name: "small change", changeCount: 5, previousCount: 100, compose: false, want: false},
		{name: "compose-heavy", changeCount: 120, previousCount: 400, compose: true, want: true},
	}

	for _, tt := range tests {
		got := shouldFallbackToFull(tt.changeCount, tt.previousCount, tt.compose)
		if got != tt.want {
			t.Fatalf("%s: shouldFallbackToFull(...) = %t, want %t", tt.name, got, tt.want)
		}
	}
}

func TestNextStatsInterval(t *testing.T) {
	collector := NewDockerCollector("/tmp/docker.sock", nil, "asset-1", 30*time.Second)

	if got := collector.nextStatsInterval(30*time.Second, agentmgr.DockerContainerStats{CPUPercent: 55}, false); got != 15*time.Second {
		t.Fatalf("hot interval = %v, want 15s", got)
	}
	if got := collector.nextStatsInterval(30*time.Second, agentmgr.DockerContainerStats{CPUPercent: 1, MemoryPercent: 5, PIDs: 2}, false); got != 45*time.Second {
		t.Fatalf("cool interval = %v, want 45s", got)
	}
	if got := collector.nextStatsInterval(30*time.Second, agentmgr.DockerContainerStats{}, true); got != time.Minute {
		t.Fatalf("error interval = %v, want 1m", got)
	}
}

func TestBuildContainerInfoMap(t *testing.T) {
	raw := []DockerContainer{
		{
			ID:      "ct-1",
			Names:   []string{"/web"},
			Image:   "nginx:1.27",
			State:   "running",
			Status:  "Up",
			Created: 1700000000,
			Ports: []DockerPort{
				{PrivatePort: 443, PublicPort: 9443, Type: "tcp"},
				{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			NetworkSettings: struct {
				Networks map[string]interface{} `json:"Networks"`
			}{
				Networks: map[string]interface{}{"z-net": map[string]any{}, "a-net": map[string]any{}},
			},
		},
	}

	containers, running := buildContainerInfoMap(raw)
	info, ok := containers["ct-1"]
	if !ok {
		t.Fatal("expected ct-1 in map")
	}
	if len(running) != 1 {
		t.Fatalf("expected one running container, got %d", len(running))
	}
	if len(info.Ports) != 2 || info.Ports[0].Host != 8080 || info.Ports[1].Host != 9443 {
		t.Fatalf("ports not sorted by host: %+v", info.Ports)
	}
	if len(info.Networks) != 2 || info.Networks[0] != "a-net" || info.Networks[1] != "z-net" {
		t.Fatalf("networks not sorted: %+v", info.Networks)
	}
}
