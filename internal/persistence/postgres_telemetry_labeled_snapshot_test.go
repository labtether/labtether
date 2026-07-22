package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestPostgresLatestLabeledMetricSnapshotsParityAndBounds(t *testing.T) {
	store := newTestPostgresStore(t)
	asset := createTestAsset(t, store, "labeled-snapshot")
	now := time.Now().UTC()
	hubRuleID := fmt.Sprintf("ltqa-labeled-hub-%d", now.UnixNano())
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM hub_metric_samples WHERE labels->>'rule_id' = $1`, hubRuleID)
	})

	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{AssetID: asset.ID, Metric: telemetry.MetricDiskUsedBytes, Unit: "bytes", Value: 10, CollectedAt: now, Labels: map[string]string{"mount_point": "/"}},
		{AssetID: asset.ID, Metric: telemetry.MetricDiskUsedBytes, Unit: "bytes", Value: 20, CollectedAt: now, Labels: map[string]string{"mount_point": "/data"}},
		{AssetID: asset.ID, Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 1, CollectedAt: now},
		{AssetID: asset.ID, Metric: telemetry.MetricMemoryUsedPercent, Unit: "percent", Value: 90, CollectedAt: now.Add(-telemetry.PrometheusAssetSnapshotMaxAge - time.Second)},
		{AssetID: asset.ID, Metric: telemetry.MetricTemperatureCelsius, Unit: "celsius", Value: 30, CollectedAt: now.Add(time.Hour)},
		{Scope: telemetry.MetricScopeHubAlerts, Metric: telemetry.MetricAlertEvaluationDurationMs, Unit: "ms", Value: 11, CollectedAt: now, Labels: map[string]string{"rule_id": hubRuleID, "rule_name": "duplicate"}},
	}); err != nil {
		t.Fatalf("append labeled samples: %v", err)
	}
	// Insert equal-time replacements in a later call so their BIGSERIAL IDs are
	// deterministically higher. Row order within a single INSERT ... SELECT is
	// not an ordering contract and must not make this tie-break fixture flaky.
	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{AssetID: asset.ID, Metric: telemetry.MetricDiskUsedBytes, Unit: "bytes", Value: 25, CollectedAt: now, Labels: map[string]string{"mount_point": "/data"}},
		{AssetID: asset.ID, Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 2, CollectedAt: now, Labels: map[string]string{}},
		{Scope: telemetry.MetricScopeHubAlerts, Metric: telemetry.MetricAlertEvaluationDurationMs, Unit: "ms", Value: 22, CollectedAt: now, Labels: map[string]string{"rule_id": hubRuleID, "rule_name": "duplicate"}},
	}); err != nil {
		t.Fatalf("append equal-time replacement samples: %v", err)
	}

	snapshots, err := store.LatestLabeledMetricSnapshots(
		context.Background(),
		[]string{" " + asset.ID + " ", asset.ID},
		now.Add(time.Second),
		telemetry.MaxPrometheusAssetMetricSeries,
	)
	if err != nil {
		t.Fatalf("load labeled snapshots: %v", err)
	}
	samples := snapshots[asset.ID]
	if len(samples) != 3 {
		t.Fatalf("labeled series count = %d, want 3: %+v", len(samples), samples)
	}
	mountValues := make(map[string]float64)
	var cpuCount int
	for _, sample := range samples {
		if sample.Scope != "" || sample.AssetID != asset.ID {
			t.Fatalf("invalid asset snapshot identity: %+v", sample)
		}
		switch sample.Metric {
		case telemetry.MetricDiskUsedBytes:
			mountValues[sample.Labels["mount_point"]] = sample.Value
		case telemetry.MetricCPUUsedPercent:
			cpuCount++
			if sample.Value != 2 || sample.Labels != nil {
				t.Fatalf("nil/empty label canonicalization or ID tie-break mismatch: %+v", sample)
			}
		case telemetry.MetricMemoryUsedPercent, telemetry.MetricTemperatureCelsius:
			t.Fatalf("stale/future sample leaked into snapshot: %+v", sample)
		case telemetry.MetricAlertEvaluationDurationMs:
			t.Fatalf("hub sample leaked into asset snapshot: %+v", sample)
		}
	}
	if cpuCount != 1 || len(mountValues) != 2 || mountValues["/"] != 10 || mountValues["/data"] != 25 {
		t.Fatalf("labeled snapshot mismatch: cpu=%d mounts=%v", cpuCount, mountValues)
	}

	if partial, err := store.LatestLabeledMetricSnapshots(context.Background(), []string{asset.ID}, now.Add(time.Second), 2); !errors.Is(err, ErrTelemetrySnapshotSeriesLimitExceeded) || partial != nil {
		t.Fatalf("series overflow result=%+v error=%v, want nil typed failure", partial, err)
	}
	if partial, err := store.latestLabeledMetricSnapshots(context.Background(), []string{asset.ID}, now.Add(time.Second), telemetry.MaxPrometheusAssetMetricSeries, 2); !errors.Is(err, ErrTelemetrySnapshotRowLimitExceeded) || partial != nil {
		t.Fatalf("raw-row overflow result=%+v error=%v, want nil typed failure", partial, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.LatestLabeledMetricSnapshots(ctx, []string{asset.ID}, now, telemetry.MaxPrometheusAssetMetricSeries); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled snapshot error = %v, want context.Canceled", err)
	}
	tooManyAssets := make([]string, telemetry.MaxPrometheusSnapshotAssets+1)
	for i := range tooManyAssets {
		tooManyAssets[i] = asset.ID
	}
	if _, err := store.LatestLabeledMetricSnapshots(context.Background(), tooManyAssets, now, telemetry.MaxPrometheusAssetMetricSeries); !errors.Is(err, ErrTelemetrySnapshotAssetLimitExceeded) {
		t.Fatalf("raw asset input overflow error = %v", err)
	}

	hubSnapshots, err := store.HubMetricSnapshots(context.Background(), now.Add(time.Second), telemetry.MaxHubMetricSnapshotSeries)
	if err != nil {
		t.Fatalf("load hub snapshots: %v", err)
	}
	for _, sample := range hubSnapshots[telemetry.MetricScopeHubAlerts] {
		if sample.Labels["rule_id"] == hubRuleID {
			if sample.Value != 22 {
				t.Fatalf("hub equal-time ID tie-break value = %v, want 22", sample.Value)
			}
			return
		}
	}
	t.Fatalf("hub tie-break fixture %q missing", hubRuleID)
}
