package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockDiskMountsSource implements DiskMountsSource for testing.
type mockDiskMountsSource struct {
	entries []DiskMountEntry
}

func (m *mockDiskMountsSource) AllDiskMounts() []DiskMountEntry {
	return m.entries
}

func TestDiskMountsBridgeCollect(t *testing.T) {
	source := &mockDiskMountsSource{
		entries: []DiskMountEntry{
			{
				AssetID:   "linux-asset-server01",
				Total:     100 * 1024 * 1024 * 1024,
				Used:      40 * 1024 * 1024 * 1024,
				Available: 60 * 1024 * 1024 * 1024,
				UsePct:    40.0,
				Labels: map[string]string{
					"mount_point": "/",
				},
			},
			{
				AssetID:   "linux-asset-server01",
				Total:     500 * 1024 * 1024 * 1024,
				Used:      200 * 1024 * 1024 * 1024,
				Available: 300 * 1024 * 1024 * 1024,
				UsePct:    40.0,
				Labels: map[string]string{
					"mount_point": "/data",
				},
			},
		},
	}

	b := NewDiskMountsBridge(source)

	if b.Name() != "disk-mounts" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 8 {
		t.Fatalf("expected 8 samples (4 per mount), got %d", len(samples))
	}

	// Build lookup: "assetID:metric:mount_point" -> sample to distinguish two mounts on same asset.
	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		mp := s.Labels["mount_point"]
		byKey[s.AssetID+":"+s.Metric+":"+mp] = s
	}

	assetID := "linux-asset-server01"

	// Root mount
	assertSampleByKey(t, byKey, assetID, telemetry.MetricDiskTotalBytes, "bytes", 100*1024*1024*1024, "/")
	assertSampleByKey(t, byKey, assetID, telemetry.MetricDiskUsedBytes, "bytes", 40*1024*1024*1024, "/")
	assertSampleByKey(t, byKey, assetID, telemetry.MetricDiskAvailableBytes, "bytes", 60*1024*1024*1024, "/")
	assertSampleByKey(t, byKey, assetID, telemetry.MetricDiskUsedPercent, "percent", 40.0, "/")

	// /data mount
	assertSampleByKey(t, byKey, assetID, telemetry.MetricDiskTotalBytes, "bytes", 500*1024*1024*1024, "/data")
	assertSampleByKey(t, byKey, assetID, telemetry.MetricDiskUsedBytes, "bytes", 200*1024*1024*1024, "/data")
	assertSampleByKey(t, byKey, assetID, telemetry.MetricDiskAvailableBytes, "bytes", 300*1024*1024*1024, "/data")
	assertSampleByKey(t, byKey, assetID, telemetry.MetricDiskUsedPercent, "percent", 40.0, "/data")
}

func TestDiskMountsBridgeEmpty(t *testing.T) {
	source := &mockDiskMountsSource{entries: nil}
	b := NewDiskMountsBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}

func TestDiskMountsBridgeLabelsHaveMountPoint(t *testing.T) {
	wantMount := "/var/log"

	source := &mockDiskMountsSource{
		entries: []DiskMountEntry{
			{
				AssetID: "linux-asset-server02",
				Total:   50 * 1024 * 1024 * 1024,
				UsePct:  10.0,
				Labels: map[string]string{
					"mount_point": wantMount,
				},
			},
		},
	}

	b := NewDiskMountsBridge(source)
	samples := b.Collect()

	if len(samples) != 4 {
		t.Fatalf("expected 4 samples, got %d", len(samples))
	}

	for _, s := range samples {
		got, ok := s.Labels["mount_point"]
		if !ok {
			t.Errorf("metric %q: missing mount_point label", s.Metric)
			continue
		}
		if got != wantMount {
			t.Errorf("metric %q: mount_point=%q, want %q", s.Metric, got, wantMount)
		}
	}
}

// assertSampleByKey looks up a sample by "assetID:metric:mountPoint" key.
func assertSampleByKey(
	t *testing.T,
	byKey map[string]telemetry.MetricSample,
	assetID, metric, wantUnit string,
	wantValue float64,
	mountPoint string,
) {
	t.Helper()
	key := assetID + ":" + metric + ":" + mountPoint
	s, ok := byKey[key]
	if !ok {
		t.Errorf("missing sample assetID=%q metric=%q mount_point=%q", assetID, metric, mountPoint)
		return
	}
	if s.Unit != wantUnit {
		t.Errorf("sample %q/%q/%q: unit=%q, want %q", assetID, metric, mountPoint, s.Unit, wantUnit)
	}
	if s.Value != wantValue {
		t.Errorf("sample %q/%q/%q: value=%v, want %v", assetID, metric, mountPoint, s.Value, wantValue)
	}
	if s.CollectedAt.IsZero() {
		t.Errorf("sample %q/%q/%q: CollectedAt is zero", assetID, metric, mountPoint)
	}
}
