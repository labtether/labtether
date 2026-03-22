package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockDockerStatsSource implements DockerStatsSource for testing.
type mockDockerStatsSource struct {
	entries []ContainerMetricEntry
}

func (m *mockDockerStatsSource) AllContainerMetrics() []ContainerMetricEntry {
	return m.entries
}

func TestDockerStatsBridgeCollect(t *testing.T) {
	source := &mockDockerStatsSource{
		entries: []ContainerMetricEntry{
			{
				AssetID:    "docker-ct-myhost-abc123456789",
				CPU:        12.5,
				Memory:     44.0,
				NetRX:      1024.0,
				NetTX:      512.0,
				BlockRead:  2048.0,
				BlockWrite: 4096.0,
				PIDs:       8,
				Labels: map[string]string{
					"docker_host":  "myhost",
					"docker_image": "nginx:latest",
					"docker_stack": "web",
				},
			},
			{
				AssetID:    "docker-ct-myhost-def098765432",
				CPU:        3.2,
				Memory:     15.7,
				NetRX:      256.0,
				NetTX:      128.0,
				BlockRead:  0,
				BlockWrite: 512.0,
				PIDs:       2,
				Labels: map[string]string{
					"docker_host":  "myhost",
					"docker_image": "redis:7",
					"docker_stack": "",
				},
			},
		},
	}

	b := NewDockerStatsBridge(source)

	if b.Name() != "docker-stats" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 14 {
		t.Fatalf("expected 14 samples (7 per container), got %d", len(samples))
	}

	// Build a lookup: "assetID:metric" -> sample for easy assertion.
	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		byKey[s.AssetID+":"+s.Metric] = s
	}

	// --- Container 1 assertions ---
	ct1 := "docker-ct-myhost-abc123456789"

	assertSample(t, byKey, ct1, telemetry.MetricCPUUsedPercent, "percent", 12.5)
	assertSample(t, byKey, ct1, telemetry.MetricMemoryUsedPercent, "percent", 44.0)
	assertSample(t, byKey, ct1, telemetry.MetricNetworkRXBytesPerSec, "bytes_per_sec", 1024.0)
	assertSample(t, byKey, ct1, telemetry.MetricNetworkTXBytesPerSec, "bytes_per_sec", 512.0)
	assertSample(t, byKey, ct1, telemetry.MetricBlockReadBytesPerSec, "bytes_per_sec", 2048.0)
	assertSample(t, byKey, ct1, telemetry.MetricBlockWriteBytesPerSec, "bytes_per_sec", 4096.0)
	assertSample(t, byKey, ct1, telemetry.MetricPIDs, "count", 8)

	// --- Container 2 assertions ---
	ct2 := "docker-ct-myhost-def098765432"

	assertSample(t, byKey, ct2, telemetry.MetricCPUUsedPercent, "percent", 3.2)
	assertSample(t, byKey, ct2, telemetry.MetricMemoryUsedPercent, "percent", 15.7)
	assertSample(t, byKey, ct2, telemetry.MetricNetworkRXBytesPerSec, "bytes_per_sec", 256.0)
	assertSample(t, byKey, ct2, telemetry.MetricNetworkTXBytesPerSec, "bytes_per_sec", 128.0)
	assertSample(t, byKey, ct2, telemetry.MetricBlockReadBytesPerSec, "bytes_per_sec", 0)
	assertSample(t, byKey, ct2, telemetry.MetricBlockWriteBytesPerSec, "bytes_per_sec", 512.0)
	assertSample(t, byKey, ct2, telemetry.MetricPIDs, "count", 2)
}

func TestDockerStatsBridgeEmpty(t *testing.T) {
	source := &mockDockerStatsSource{entries: nil}
	b := NewDockerStatsBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}

func TestDockerStatsBridgeLabels(t *testing.T) {
	wantLabels := map[string]string{
		"docker_host":  "prodhost",
		"docker_image": "postgres:15",
		"docker_stack": "db-stack",
	}

	source := &mockDockerStatsSource{
		entries: []ContainerMetricEntry{
			{
				AssetID: "docker-ct-prodhost-aabbcc112233",
				CPU:     5.0,
				Labels:  wantLabels,
			},
		},
	}

	b := NewDockerStatsBridge(source)
	samples := b.Collect()

	if len(samples) != 7 {
		t.Fatalf("expected 7 samples, got %d", len(samples))
	}

	for _, s := range samples {
		if s.Labels == nil {
			t.Errorf("metric %q: expected labels, got nil", s.Metric)
			continue
		}
		for k, want := range wantLabels {
			got, ok := s.Labels[k]
			if !ok {
				t.Errorf("metric %q: missing label %q", s.Metric, k)
				continue
			}
			if got != want {
				t.Errorf("metric %q label %q: got %q, want %q", s.Metric, k, got, want)
			}
		}
	}
}

// assertSample is a helper that looks up a sample by "assetID:metric" and checks unit and value.
func assertSample(
	t *testing.T,
	byKey map[string]telemetry.MetricSample,
	assetID, metric, wantUnit string,
	wantValue float64,
) {
	t.Helper()
	s, ok := byKey[assetID+":"+metric]
	if !ok {
		t.Errorf("missing sample assetID=%q metric=%q", assetID, metric)
		return
	}
	if s.Unit != wantUnit {
		t.Errorf("sample %q/%q: unit=%q, want %q", assetID, metric, s.Unit, wantUnit)
	}
	if s.Value != wantValue {
		t.Errorf("sample %q/%q: value=%v, want %v", assetID, metric, s.Value, wantValue)
	}
	if s.CollectedAt.IsZero() {
		t.Errorf("sample %q/%q: CollectedAt is zero", assetID, metric)
	}
}
