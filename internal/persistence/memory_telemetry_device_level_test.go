package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// A Windows machine with C: and M: reported M:'s free space against C:'s
// total in its latest snapshot (available > total), and its disk chart
// jumped between C:, M: and the heartbeat value: device-level answers used
// per-mount samples, which share the metric names.
func TestDeviceSnapshotAndChartsDoNotMixDisks(t *testing.T) {
	store := NewMemoryTelemetryStore()
	now := time.Now().UTC().Truncate(time.Second)
	sample := func(metric string, value float64, at time.Time, mount string) telemetry.MetricSample {
		s := telemetry.MetricSample{AssetID: "win-1", Metric: metric, Unit: "x", Value: value, CollectedAt: at}
		if mount != "" {
			s.Labels = map[string]string{"mount_point": mount}
		}
		return s
	}
	mounts := now.Add(-30 * time.Second)
	samples := []telemetry.MetricSample{
		sample(telemetry.MetricDiskUsedPercent, 51.4, now.Add(-10*time.Second), ""),
		sample(telemetry.MetricDiskTotalBytes, 160e9, mounts, `C:\`),
		sample(telemetry.MetricDiskUsedBytes, 82e9, mounts, `C:\`),
		sample(telemetry.MetricDiskAvailableBytes, 78e9, mounts, `C:\`),
		sample(telemetry.MetricDiskUsedPercent, 51.4, mounts, `C:\`),
		sample(telemetry.MetricDiskTotalBytes, 537e9, mounts, `M:\`),
		sample(telemetry.MetricDiskUsedBytes, 335e9, mounts, `M:\`),
		sample(telemetry.MetricDiskAvailableBytes, 202e9, mounts, `M:\`),
		sample(telemetry.MetricDiskUsedPercent, 62.4, mounts, `M:\`),
	}
	if err := store.AppendSamples(context.Background(), samples); err != nil {
		t.Fatal(err)
	}
	snap, err := store.DynamicSnapshotForAsset("win-1", now)
	if err != nil {
		t.Fatal(err)
	}
	m := snap.Metrics
	if m[telemetry.MetricDiskTotalBytes] != 160e9 || m[telemetry.MetricDiskAvailableBytes] != 78e9 || m[telemetry.MetricDiskUsedBytes] != 82e9 || m[telemetry.MetricDiskUsedPercent] != 51.4 {
		t.Fatalf("snapshot mixes disks: %+v", m)
	}
	series, err := store.Series("win-1", now.Add(-time.Minute), now, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range series {
		if s.Metric != telemetry.MetricDiskUsedPercent {
			continue
		}
		for _, p := range s.Points {
			if p.Value != 51.4 {
				t.Fatalf("disk chart includes another disk's point %v: %+v", p.Value, s.Points)
			}
		}
	}
}
