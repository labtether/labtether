package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockPBSMetricsSource implements PBSMetricsSource for testing.
type mockPBSMetricsSource struct {
	entries []PBSMetricEntry
}

func (m *mockPBSMetricsSource) AllPBSMetrics() []PBSMetricEntry {
	return m.entries
}

func TestPBSMetricsBridgeCollect(t *testing.T) {
	source := &mockPBSMetricsSource{
		entries: []PBSMetricEntry{
			{
				AssetID:     "pbs-asset-backup01",
				Total:       2 * 1024 * 1024 * 1024 * 1024, // 2 TiB
				Used:        500 * 1024 * 1024 * 1024,      // 500 GiB
				Available:   1524 * 1024 * 1024 * 1024,     // remainder
				BackupCount: 42,
				BackupAge:   3600, // 1 hour
				GCPending:   8192,
				Labels: map[string]string{
					"datastore": "backup-store",
				},
			},
		},
	}

	b := NewPBSMetricsBridge(source)

	if b.Name() != "pbs-metrics" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 6 {
		t.Fatalf("expected 6 samples (6 per entry), got %d", len(samples))
	}

	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		byKey[s.AssetID+":"+s.Metric] = s
	}

	assetID := "pbs-asset-backup01"

	assertSample(t, byKey, assetID, telemetry.MetricStorageTotalBytes, "bytes", 2*1024*1024*1024*1024)
	assertSample(t, byKey, assetID, telemetry.MetricStorageUsedBytes, "bytes", 500*1024*1024*1024)
	assertSample(t, byKey, assetID, telemetry.MetricStorageAvailableBytes, "bytes", 1524*1024*1024*1024)
	assertSample(t, byKey, assetID, telemetry.MetricBackupCount, "count", 42)
	assertSample(t, byKey, assetID, telemetry.MetricBackupAgeSeconds, "seconds", 3600)
	assertSample(t, byKey, assetID, telemetry.MetricGCPendingBytes, "bytes", 8192)
}

func TestPBSMetricsBridgeEmpty(t *testing.T) {
	source := &mockPBSMetricsSource{entries: nil}
	b := NewPBSMetricsBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}

func TestPBSMetricsBridgeLabels(t *testing.T) {
	wantLabels := map[string]string{
		"datastore": "offsite-store",
	}

	source := &mockPBSMetricsSource{
		entries: []PBSMetricEntry{
			{
				AssetID: "pbs-asset-backup02",
				Total:   1024,
				Labels:  wantLabels,
			},
		},
	}

	b := NewPBSMetricsBridge(source)
	samples := b.Collect()

	if len(samples) != 6 {
		t.Fatalf("expected 6 samples, got %d", len(samples))
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
