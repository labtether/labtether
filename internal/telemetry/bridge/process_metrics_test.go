package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockProcessMetricsSource implements ProcessMetricsSource for testing.
type mockProcessMetricsSource struct {
	entries []ProcessMetricEntry
}

func (m *mockProcessMetricsSource) AllProcessMetrics() []ProcessMetricEntry {
	return m.entries
}

func TestProcessMetricsBridgeCollect(t *testing.T) {
	source := &mockProcessMetricsSource{
		entries: []ProcessMetricEntry{
			{
				AssetID:    "asset-server-01",
				CPUPercent: 45.2,
				MemPercent: 12.8,
				MemRSS:     134217728, // 128 MiB
				Labels: map[string]string{
					"process_name": "nginx",
					"process_pid":  "1234",
				},
			},
			{
				AssetID:    "asset-server-01",
				CPUPercent: 22.1,
				MemPercent: 8.5,
				MemRSS:     67108864, // 64 MiB
				Labels: map[string]string{
					"process_name": "postgres",
					"process_pid":  "5678",
				},
			},
			{
				AssetID:    "asset-server-02",
				CPUPercent: 5.0,
				MemPercent: 3.3,
				MemRSS:     16777216, // 16 MiB
				Labels: map[string]string{
					"process_name": "node",
					"process_pid":  "9012",
				},
			},
		},
	}

	b := NewProcessMetricsBridge(source)

	if b.Name() != "process-metrics" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 9 {
		t.Fatalf("expected 9 samples (3 per process), got %d", len(samples))
	}

	// Build lookup: "assetID:metric:pid" -> sample using process_pid label to disambiguate
	// same-asset processes. Key by "assetID:metric:process_pid".
	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		pid := ""
		if s.Labels != nil {
			pid = s.Labels["process_pid"]
		}
		byKey[s.AssetID+":"+s.Metric+":"+pid] = s
	}

	// --- Process 1: nginx (pid 1234) on asset-server-01 ---
	assertSampleByPID(t, byKey, "asset-server-01", telemetry.MetricProcessCPUPercent, "percent", 45.2, "1234")
	assertSampleByPID(t, byKey, "asset-server-01", telemetry.MetricProcessMemoryPercent, "percent", 12.8, "1234")
	assertSampleByPID(t, byKey, "asset-server-01", telemetry.MetricProcessMemoryRSS, "bytes", 134217728, "1234")

	// --- Process 2: postgres (pid 5678) on asset-server-01 ---
	assertSampleByPID(t, byKey, "asset-server-01", telemetry.MetricProcessCPUPercent, "percent", 22.1, "5678")
	assertSampleByPID(t, byKey, "asset-server-01", telemetry.MetricProcessMemoryPercent, "percent", 8.5, "5678")
	assertSampleByPID(t, byKey, "asset-server-01", telemetry.MetricProcessMemoryRSS, "bytes", 67108864, "5678")

	// --- Process 3: node (pid 9012) on asset-server-02 ---
	assertSampleByPID(t, byKey, "asset-server-02", telemetry.MetricProcessCPUPercent, "percent", 5.0, "9012")
	assertSampleByPID(t, byKey, "asset-server-02", telemetry.MetricProcessMemoryPercent, "percent", 3.3, "9012")
	assertSampleByPID(t, byKey, "asset-server-02", telemetry.MetricProcessMemoryRSS, "bytes", 16777216, "9012")
}

func TestProcessMetricsBridgeLabels(t *testing.T) {
	wantLabels := map[string]string{
		"process_name": "redis-server",
		"process_pid":  "4321",
	}

	source := &mockProcessMetricsSource{
		entries: []ProcessMetricEntry{
			{
				AssetID:    "asset-cache-01",
				CPUPercent: 1.5,
				MemPercent: 2.0,
				MemRSS:     8388608,
				Labels:     wantLabels,
			},
		},
	}

	b := NewProcessMetricsBridge(source)
	samples := b.Collect()

	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
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

func TestProcessMetricsBridgeEmpty(t *testing.T) {
	source := &mockProcessMetricsSource{entries: nil}
	b := NewProcessMetricsBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}

// assertSampleByPID looks up a sample by "assetID:metric:pid" and checks unit and value.
func assertSampleByPID(
	t *testing.T,
	byKey map[string]telemetry.MetricSample,
	assetID, metric, wantUnit string,
	wantValue float64,
	pid string,
) {
	t.Helper()
	key := assetID + ":" + metric + ":" + pid
	s, ok := byKey[key]
	if !ok {
		t.Errorf("missing sample assetID=%q metric=%q pid=%q", assetID, metric, pid)
		return
	}
	if s.Unit != wantUnit {
		t.Errorf("sample %q/%q/%q: unit=%q, want %q", assetID, metric, pid, s.Unit, wantUnit)
	}
	if s.Value != wantValue {
		t.Errorf("sample %q/%q/%q: value=%v, want %v", assetID, metric, pid, s.Value, wantValue)
	}
	if s.CollectedAt.IsZero() {
		t.Errorf("sample %q/%q/%q: CollectedAt is zero", assetID, metric, pid)
	}
}
