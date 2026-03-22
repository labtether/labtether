package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockProxmoxMetricsSource implements ProxmoxMetricsSource for testing.
type mockProxmoxMetricsSource struct {
	entries []ProxmoxMetricEntry
}

func (m *mockProxmoxMetricsSource) AllProxmoxMetrics() []ProxmoxMetricEntry {
	return m.entries
}

func TestProxmoxMetricsBridgeCollect(t *testing.T) {
	source := &mockProxmoxMetricsSource{
		entries: []ProxmoxMetricEntry{
			{
				AssetID:   "proxmox-node-pve01",
				CPU:       0.25, // 25%
				MemUsed:   4 * 1024 * 1024 * 1024,
				MemTotal:  16 * 1024 * 1024 * 1024,
				NetIn:     1048576,
				NetOut:    524288,
				DiskRead:  2097152,
				DiskWrite: 1048576,
				Labels: map[string]string{
					"proxmox_node": "pve01",
				},
			},
			{
				AssetID:   "proxmox-vm-pve01-100",
				CPU:       0.05, // 5%
				MemUsed:   1 * 1024 * 1024 * 1024,
				MemTotal:  2 * 1024 * 1024 * 1024,
				NetIn:     102400,
				NetOut:    51200,
				DiskRead:  0,
				DiskWrite: 4096,
				Labels: map[string]string{
					"proxmox_node": "pve01",
				},
			},
		},
	}

	b := NewProxmoxMetricsBridge(source)

	if b.Name() != "proxmox-metrics" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 14 {
		t.Fatalf("expected 14 samples (7 per entry), got %d", len(samples))
	}

	// Build lookup: "assetID:metric" -> sample.
	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		byKey[s.AssetID+":"+s.Metric] = s
	}

	// --- Node 1 assertions ---
	n1 := "proxmox-node-pve01"

	assertSample(t, byKey, n1, telemetry.MetricCPUUsedPercent, "percent", 25.0)
	assertSample(t, byKey, n1, telemetry.MetricMemoryUsedBytes, "bytes", 4*1024*1024*1024)
	assertSample(t, byKey, n1, telemetry.MetricMemoryTotalBytes, "bytes", 16*1024*1024*1024)
	assertSample(t, byKey, n1, telemetry.MetricNetworkRXBytesPerSec, "bytes_per_sec", 1048576)
	assertSample(t, byKey, n1, telemetry.MetricNetworkTXBytesPerSec, "bytes_per_sec", 524288)
	assertSample(t, byKey, n1, telemetry.MetricDiskReadBytesPerSec, "bytes_per_sec", 2097152)
	assertSample(t, byKey, n1, telemetry.MetricDiskWriteBytesPerSec, "bytes_per_sec", 1048576)

	// --- VM assertions ---
	vm1 := "proxmox-vm-pve01-100"

	assertSample(t, byKey, vm1, telemetry.MetricCPUUsedPercent, "percent", 5.0)
	assertSample(t, byKey, vm1, telemetry.MetricMemoryUsedBytes, "bytes", 1*1024*1024*1024)
	assertSample(t, byKey, vm1, telemetry.MetricMemoryTotalBytes, "bytes", 2*1024*1024*1024)
	assertSample(t, byKey, vm1, telemetry.MetricNetworkRXBytesPerSec, "bytes_per_sec", 102400)
	assertSample(t, byKey, vm1, telemetry.MetricNetworkTXBytesPerSec, "bytes_per_sec", 51200)
	assertSample(t, byKey, vm1, telemetry.MetricDiskReadBytesPerSec, "bytes_per_sec", 0)
	assertSample(t, byKey, vm1, telemetry.MetricDiskWriteBytesPerSec, "bytes_per_sec", 4096)
}

func TestProxmoxMetricsBridgeEmpty(t *testing.T) {
	source := &mockProxmoxMetricsSource{entries: nil}
	b := NewProxmoxMetricsBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}

func TestProxmoxMetricsBridgeLabels(t *testing.T) {
	wantLabels := map[string]string{
		"proxmox_node": "pve02",
	}

	source := &mockProxmoxMetricsSource{
		entries: []ProxmoxMetricEntry{
			{
				AssetID: "proxmox-node-pve02",
				CPU:     0.1,
				Labels:  wantLabels,
			},
		},
	}

	b := NewProxmoxMetricsBridge(source)
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

func TestProxmoxMetricsBridgeCPUConversion(t *testing.T) {
	// CPU ratio 0.75 should produce 75.0 percent.
	source := &mockProxmoxMetricsSource{
		entries: []ProxmoxMetricEntry{
			{
				AssetID: "proxmox-node-pve03",
				CPU:     0.75,
				Labels:  map[string]string{"proxmox_node": "pve03"},
			},
		},
	}

	b := NewProxmoxMetricsBridge(source)
	samples := b.Collect()

	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		byKey[s.AssetID+":"+s.Metric] = s
	}

	assertSample(t, byKey, "proxmox-node-pve03", telemetry.MetricCPUUsedPercent, "percent", 75.0)
}
