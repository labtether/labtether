package docker

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDockerHostJSONRoundTrip(t *testing.T) {
	host := DockerHost{
		AgentID: "agent-01",
		Engine:  EngineInfo{Version: "24.0.7", APIVersion: "1.43", OS: "linux", Arch: "amd64"},
		Containers: []ContainerState{
			{ID: "abc123", Name: "nginx", Image: "nginx:1.25", State: "running", Status: "Up 3 days"},
		},
		Images: []ImageState{
			{ID: "sha256:abc", Tags: []string{"nginx:1.25"}, Size: 187654321, Created: "2026-02-01T00:00:00Z"},
		},
		Networks: []NetworkState{
			{ID: "net123", Name: "bridge", Driver: "bridge", Scope: "local"},
		},
		Volumes: []VolumeState{
			{Name: "pgdata", Driver: "local", Mountpoint: "/var/lib/docker/volumes/pgdata/_data"},
		},
		ComposeStacks: []ComposeStackState{
			{Name: "webstack", Status: "running(2)", ConfigFile: "/opt/stacks/web/docker-compose.yml", ContainerIDs: []string{"abc123", "def456"}},
		},
		Stats: map[string]ContainerStats{
			"abc123": {CPUPercent: 2.5, MemoryBytes: 134217728, MemoryLimit: 536870912, MemoryPercent: 25.0},
		},
		LastSeen: time.Now(),
	}

	data, err := json.Marshal(host)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded DockerHost
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.AgentID != host.AgentID {
		t.Errorf("AgentID = %q, want %q", decoded.AgentID, host.AgentID)
	}
	if decoded.Engine.Version != "24.0.7" {
		t.Errorf("Engine.Version = %q, want %q", decoded.Engine.Version, "24.0.7")
	}
	if len(decoded.Containers) != 1 || decoded.Containers[0].Name != "nginx" {
		t.Errorf("Containers round-trip failed: %+v", decoded.Containers)
	}
	if len(decoded.Images) != 1 || decoded.Images[0].Tags[0] != "nginx:1.25" {
		t.Errorf("Images round-trip failed")
	}
	if len(decoded.Networks) != 1 || decoded.Networks[0].Name != "bridge" {
		t.Errorf("Networks round-trip failed")
	}
	if len(decoded.Volumes) != 1 || decoded.Volumes[0].Name != "pgdata" {
		t.Errorf("Volumes round-trip failed")
	}
	if len(decoded.ComposeStacks) != 1 || decoded.ComposeStacks[0].Name != "webstack" {
		t.Errorf("ComposeStacks round-trip failed")
	}
	if stats, ok := decoded.Stats["abc123"]; !ok || stats.CPUPercent != 2.5 {
		t.Errorf("Stats round-trip failed")
	}
}

func TestContainerStatsFieldTypes(t *testing.T) {
	raw := `{"cpu_percent":45.2,"memory_bytes":268435456,"memory_limit":536870912,"memory_percent":50.0,"net_rx_bytes":1048576,"net_tx_bytes":524288,"block_read_bytes":2097152,"block_write_bytes":1048576,"pids":12}`
	var stats ContainerStats
	if err := json.Unmarshal([]byte(raw), &stats); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if stats.CPUPercent != 45.2 {
		t.Errorf("CPUPercent = %f, want 45.2", stats.CPUPercent)
	}
	if stats.PIDs != 12 {
		t.Errorf("PIDs = %d, want 12", stats.PIDs)
	}
}
