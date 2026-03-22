package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestDockerEventStreamParsing(t *testing.T) {
	eventJSON := `{"Type":"container","Action":"start","Actor":{"ID":"abc123","Attributes":{"name":"nginx","image":"nginx:1.25"}},"time":1708700000}` + "\n"
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			w.Write([]byte(eventJSON))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	events := make(chan DockerEvent, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go client.streamEvents(ctx, events)

	select {
	case ev := <-events:
		if ev.Type != "container" {
			t.Errorf("type = %q, want %q", ev.Type, "container")
		}
		if ev.Action != "start" {
			t.Errorf("action = %q, want %q", ev.Action, "start")
		}
		if ev.Actor.ID != "abc123" {
			t.Errorf("actor.ID = %q, want %q", ev.Actor.ID, "abc123")
		}
		if ev.Actor.Attributes["name"] != "nginx" {
			t.Errorf("actor.name = %q, want %q", ev.Actor.Attributes["name"], "nginx")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
}

func TestDockerEventStreamMultipleEvents(t *testing.T) {
	events := `{"Type":"container","Action":"start","Actor":{"ID":"a1"},"time":1}
{"Type":"container","Action":"stop","Actor":{"ID":"a2"},"time":2}
`
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			w.Write([]byte(events))
			return
		}
	}))
	defer srv.Close()

	ch := make(chan DockerEvent, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go client.streamEvents(ctx, ch)

	var received []DockerEvent
	timeout := time.After(2 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case ev := <-ch:
			received = append(received, ev)
		case <-timeout:
			t.Fatalf("timeout after receiving %d events", len(received))
		}
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Action != "start" || received[1].Action != "stop" {
		t.Errorf("unexpected actions: %q, %q", received[0].Action, received[1].Action)
	}
}

func TestCalculateStats(t *testing.T) {
	raw := DockerStatsResponse{}
	raw.CPUStats.CPUUsage.TotalUsage = 200
	raw.CPUStats.SystemCPUUsage = 1000
	raw.CPUStats.OnlineCPUs = 4
	raw.PreCPUStats.CPUUsage.TotalUsage = 100
	raw.PreCPUStats.SystemCPUUsage = 500
	raw.MemoryStats.Usage = 268435456 // 256MB
	raw.MemoryStats.Limit = 536870912 // 512MB
	raw.PidsStats.Current = 12

	stats := calculateStats("test-container", raw)

	// CPU: (200-100) / (1000-500) * 4 * 100 = 80%
	if stats.CPUPercent < 79.9 || stats.CPUPercent > 80.1 {
		t.Errorf("CPUPercent = %f, want ~80.0", stats.CPUPercent)
	}
	if stats.MemoryBytes != 268435456 {
		t.Errorf("MemoryBytes = %d, want 268435456", stats.MemoryBytes)
	}
	// Memory: 256/512 * 100 = 50%
	if stats.MemoryPercent < 49.9 || stats.MemoryPercent > 50.1 {
		t.Errorf("MemoryPercent = %f, want ~50.0", stats.MemoryPercent)
	}
	if stats.PIDs != 12 {
		t.Errorf("PIDs = %d, want 12", stats.PIDs)
	}
}

func TestCalculateStatsZeroDelta(t *testing.T) {
	// When system delta is 0, CPU should be 0 (no division by zero)
	raw := DockerStatsResponse{}
	raw.CPUStats.CPUUsage.TotalUsage = 100
	raw.CPUStats.SystemCPUUsage = 100
	raw.PreCPUStats.CPUUsage.TotalUsage = 100
	raw.PreCPUStats.SystemCPUUsage = 100

	stats := calculateStats("test", raw)
	if stats.CPUPercent != 0 {
		t.Errorf("CPUPercent = %f, want 0 for zero delta", stats.CPUPercent)
	}
}

func TestContainerStatsEndpoint(t *testing.T) {
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/abc123/stats" && r.URL.Query().Get("stream") == "false" {
			json.NewEncoder(w).Encode(map[string]any{
				"cpu_stats": map[string]any{
					"cpu_usage":        map[string]any{"total_usage": 200},
					"system_cpu_usage": 1000,
					"online_cpus":      2,
				},
				"precpu_stats": map[string]any{
					"cpu_usage":        map[string]any{"total_usage": 100},
					"system_cpu_usage": 500,
				},
				"memory_stats": map[string]any{"usage": 100000, "limit": 200000},
				"pids_stats":   map[string]any{"current": 5},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	stats, err := client.containerStats(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if stats.MemoryStats.Usage != 100000 {
		t.Errorf("memory usage = %d, want 100000", stats.MemoryStats.Usage)
	}
}

func TestQueueDiscoveryForEventContainerImmediate(t *testing.T) {
	collector := NewDockerCollector("/tmp/docker.sock", nil, "asset-1", 30*time.Second)
	collector.queueDiscoveryForEvent(DockerEvent{Type: "container", Action: "start"})

	select {
	case trigger := <-collector.discoveryTriggerCh:
		if trigger.full {
			t.Fatalf("container events should request container refresh, got full=true")
		}
		if !trigger.immediate {
			t.Fatalf("container start should be immediate")
		}
	default:
		t.Fatalf("expected discovery trigger to be queued")
	}
}

func TestQueueDiscoveryForEventImageUsesFullRefresh(t *testing.T) {
	collector := NewDockerCollector("/tmp/docker.sock", nil, "asset-1", 30*time.Second)
	collector.queueDiscoveryForEvent(DockerEvent{Type: "image", Action: "pull"})

	select {
	case trigger := <-collector.discoveryTriggerCh:
		if !trigger.full {
			t.Fatalf("image events should request full refresh")
		}
	default:
		t.Fatalf("expected discovery trigger to be queued")
	}
}
