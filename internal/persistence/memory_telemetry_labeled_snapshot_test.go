package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestMemoryLatestLabeledMetricSnapshotsPreservesSeriesAndCanonicalizesLabels(t *testing.T) {
	store := NewMemoryTelemetryStore()
	now := time.Now().UTC()
	rootLabels := map[string]string{"mount_point": "/"}
	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{AssetID: "asset-1", Metric: telemetry.MetricDiskUsedBytes, Unit: "bytes", Value: 10, CollectedAt: now, Labels: rootLabels},
		{AssetID: "asset-1", Metric: telemetry.MetricDiskUsedBytes, Unit: "bytes", Value: 20, CollectedAt: now, Labels: map[string]string{"mount_point": "/data"}},
		{AssetID: "asset-1", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 1, CollectedAt: now},
		{AssetID: "asset-1", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 2, Labels: map[string]string{}},
		{AssetID: "asset-1", Metric: telemetry.MetricMemoryUsedPercent, Unit: "percent", Value: 99, CollectedAt: now.Add(-telemetry.PrometheusAssetSnapshotMaxAge - time.Second)},
		{Scope: telemetry.MetricScopeHubAlerts, Metric: telemetry.MetricAlertsRules, Unit: "count", Value: 3, CollectedAt: now},
	}); err != nil {
		t.Fatalf("append samples: %v", err)
	}
	rootLabels["mount_point"] = "caller-mutated"
	snapshotAt := time.Now().UTC().Add(time.Second)

	snapshots, err := store.LatestLabeledMetricSnapshots(
		context.Background(),
		[]string{" asset-1 ", "asset-1"},
		snapshotAt,
		telemetry.MaxPrometheusAssetMetricSeries,
	)
	if err != nil {
		t.Fatalf("load labeled snapshots: %v", err)
	}
	samples := snapshots["asset-1"]
	if len(samples) != 3 {
		t.Fatalf("labeled series count = %d, want 3: %+v", len(samples), samples)
	}
	mountValues := make(map[string]float64)
	var cpuSamples int
	for _, sample := range samples {
		if sample.Scope != "" || sample.AssetID != "asset-1" {
			t.Fatalf("invalid asset identity in labeled snapshot: %+v", sample)
		}
		switch sample.Metric {
		case telemetry.MetricDiskUsedBytes:
			mountValues[sample.Labels["mount_point"]] = sample.Value
		case telemetry.MetricCPUUsedPercent:
			cpuSamples++
			if sample.Value != 2 || sample.Labels != nil {
				t.Fatalf("nil/empty labels did not canonicalize with last-append tie break: %+v", sample)
			}
		case telemetry.MetricMemoryUsedPercent:
			t.Fatalf("stale sample leaked into snapshot: %+v", sample)
		case telemetry.MetricAlertsRules:
			t.Fatalf("hub sample leaked into asset snapshot: %+v", sample)
		}
	}
	if cpuSamples != 1 || len(mountValues) != 2 || mountValues["/"] != 10 || mountValues["/data"] != 20 {
		t.Fatalf("labeled/unlabeled snapshot mismatch: cpu=%d mounts=%v", cpuSamples, mountValues)
	}

	for i := range samples {
		if samples[i].Labels != nil {
			samples[i].Labels["mount_point"] = "mutated"
		}
	}
	again, err := store.LatestLabeledMetricSnapshots(context.Background(), []string{"asset-1"}, snapshotAt, telemetry.MaxPrometheusAssetMetricSeries)
	if err != nil {
		t.Fatalf("reload labeled snapshots: %v", err)
	}
	for _, sample := range again["asset-1"] {
		if sample.Labels["mount_point"] == "mutated" {
			t.Fatal("snapshot labels were not deep-copied")
		}
	}
}

func TestMemoryLatestLabeledMetricSnapshotsEnforcesBoundsAndContext(t *testing.T) {
	store := NewMemoryTelemetryStore()
	now := time.Now().UTC()
	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{AssetID: "asset-1", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", CollectedAt: now},
		{AssetID: "asset-1", Metric: telemetry.MetricMemoryUsedPercent, Unit: "percent", CollectedAt: now},
	}); err != nil {
		t.Fatalf("append samples: %v", err)
	}
	if _, err := store.LatestLabeledMetricSnapshots(context.Background(), []string{"asset-1"}, now, 1); !errors.Is(err, ErrTelemetrySnapshotSeriesLimitExceeded) {
		t.Fatalf("series overflow error = %v", err)
	}
	if partial, err := store.latestLabeledMetricSnapshots(context.Background(), []string{"asset-1"}, now, telemetry.MaxPrometheusAssetMetricSeries, 1); !errors.Is(err, ErrTelemetrySnapshotRowLimitExceeded) || partial != nil {
		t.Fatalf("raw-row overflow result=%+v error=%v, want nil typed failure", partial, err)
	}
	assetIDs := make([]string, telemetry.MaxPrometheusSnapshotAssets+1)
	for i := range assetIDs {
		assetIDs[i] = fmt.Sprintf("asset-%05d", i)
	}
	if _, err := store.LatestLabeledMetricSnapshots(context.Background(), assetIDs, now, telemetry.MaxPrometheusAssetMetricSeries); !errors.Is(err, ErrTelemetrySnapshotAssetLimitExceeded) {
		t.Fatalf("asset overflow error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.LatestLabeledMetricSnapshots(ctx, []string{"asset-1"}, now, telemetry.MaxPrometheusAssetMetricSeries); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled snapshot error = %v, want context.Canceled", err)
	}
}

func TestMemoryAssetEvictionPreservesNewestEventTimeUnderLateDelivery(t *testing.T) {
	store := NewMemoryTelemetryStore()
	now := time.Now().UTC()
	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{{
		AssetID: "asset-late", Metric: telemetry.MetricDiskUsedBytes, Unit: "bytes", Value: 999,
		CollectedAt: now, Labels: map[string]string{"mount_point": "/fresh"},
	}}); err != nil {
		t.Fatalf("append fresh sample: %v", err)
	}
	late := make([]telemetry.MetricSample, 1200)
	for i := range late {
		late[i] = telemetry.MetricSample{
			AssetID: "asset-late", Metric: telemetry.MetricDiskUsedBytes, Unit: "bytes", Value: float64(i),
			CollectedAt: now.Add(-10*time.Minute + time.Duration(i)*time.Nanosecond),
			Labels:      map[string]string{"mount_point": fmt.Sprintf("/late-%04d", i)},
		}
	}
	if err := store.AppendSamples(context.Background(), late); err != nil {
		t.Fatalf("append delayed samples: %v", err)
	}
	snapshots, err := store.LatestLabeledMetricSnapshots(context.Background(), []string{"asset-late"}, now.Add(time.Second), telemetry.MaxPrometheusAssetMetricSeries)
	if err != nil {
		t.Fatalf("load labeled snapshot: %v", err)
	}
	for _, sample := range snapshots["asset-late"] {
		if sample.Labels["mount_point"] == "/fresh" && sample.Value == 999 {
			return
		}
	}
	t.Fatal("late-arriving old samples evicted the newest event-time series")
}
