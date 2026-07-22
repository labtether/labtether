package persistence

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestPostgresSyntheticHubMetricRoundTrip(t *testing.T) {
	store := newTestPostgresStore(t)
	now := time.Now().UTC()
	checkID := "test-synthetic-" + strconv.FormatInt(now.UnixNano(), 10)
	labels := map[string]string{
		"check_id": checkID, "check_name": "Private endpoint", "check_type": "http",
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(),
			`DELETE FROM hub_metric_samples WHERE scope = $1 AND labels->>'check_id' = $2`,
			telemetry.MetricScopeHubSynthetic, checkID,
		)
	})
	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{Scope: telemetry.MetricScopeHubSynthetic, Metric: telemetry.MetricSyntheticLatencyMs, Unit: "ms", Value: 12, CollectedAt: now, Labels: labels},
		{Scope: telemetry.MetricScopeHubSynthetic, Metric: telemetry.MetricSyntheticStatus, Unit: "status", Value: 1, CollectedAt: now, Labels: labels},
	}); err != nil {
		t.Fatalf("append synthetic hub metrics: %v", err)
	}

	snapshots, err := store.HubMetricSnapshots(context.Background(), now.Add(time.Second), telemetry.MaxHubMetricSnapshotSeries)
	if err != nil {
		t.Fatalf("snapshot synthetic hub metrics: %v", err)
	}
	seen := make(map[string]bool)
	for _, sample := range snapshots[telemetry.MetricScopeHubSynthetic] {
		if sample.Labels["check_id"] == checkID {
			seen[sample.Metric] = true
			if _, leaked := sample.Labels["target"]; leaked {
				t.Fatalf("synthetic target label persisted: %+v", sample.Labels)
			}
		}
	}
	for _, metric := range []string{telemetry.MetricSyntheticLatencyMs, telemetry.MetricSyntheticStatus} {
		if !seen[metric] {
			t.Fatalf("synthetic hub metric %q missing from PostgreSQL snapshot", metric)
		}
	}
}
