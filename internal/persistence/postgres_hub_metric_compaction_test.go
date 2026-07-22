package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestPostgresHubMetricCompactionBoundsCrossBatchSeriesAndHistory(t *testing.T) {
	store := newTestPostgresStore(t)
	now := time.Now().UTC()
	prefix := fmt.Sprintf("ltqa-compact-%d", now.UnixNano())
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(),
			`DELETE FROM hub_metric_samples WHERE labels->>'site_id' LIKE $1`, prefix+"%",
		)
	})

	makeBatch := func(kind string, collectedAt time.Time) []telemetry.MetricSample {
		batch := make([]telemetry.MetricSample, telemetry.MaxHubMetricSeriesPerScope)
		for i := range batch {
			id := fmt.Sprintf("%s-%s-%04d", prefix, kind, i)
			batch[i] = telemetry.MetricSample{
				Scope: telemetry.MetricScopeHubReliability, Metric: telemetry.MetricSiteReliabilityScore,
				Unit: "score", Value: float64(i), CollectedAt: collectedAt.Add(time.Duration(i) * time.Nanosecond),
				Labels: map[string]string{"site_id": id, "site_name": kind},
			}
		}
		return batch
	}
	if err := store.AppendSamples(context.Background(), makeBatch("old", now.Add(-5*time.Minute))); err != nil {
		t.Fatalf("append old hub series: %v", err)
	}
	if err := store.AppendSamples(context.Background(), makeBatch("new", now)); err != nil {
		t.Fatalf("append new hub series: %v", err)
	}

	snapshots, err := store.HubMetricSnapshots(context.Background(), now.Add(time.Minute), telemetry.MaxHubMetricSnapshotSeries)
	if err != nil {
		t.Fatalf("load compacted hub snapshot: %v", err)
	}
	newCount := 0
	oldCount := 0
	for _, sample := range snapshots[telemetry.MetricScopeHubReliability] {
		siteID := sample.Labels["site_id"]
		switch {
		case len(siteID) >= len(prefix+"-new-") && siteID[:len(prefix+"-new-")] == prefix+"-new-":
			newCount++
		case len(siteID) >= len(prefix+"-old-") && siteID[:len(prefix+"-old-")] == prefix+"-old-":
			oldCount++
		}
	}
	if newCount != telemetry.MaxHubMetricSeriesPerScope || oldCount != 0 {
		t.Fatalf("cross-batch compacted series: new=%d old=%d", newCount, oldCount)
	}

	seriesID := fmt.Sprintf("%s-new-%04d", prefix, 0)
	history := make([]telemetry.MetricSample, telemetry.MaxHubMetricHistoryPerSeries+1)
	for i := range history {
		history[i] = telemetry.MetricSample{
			Scope: telemetry.MetricScopeHubReliability, Metric: telemetry.MetricSiteReliabilityScore,
			Unit: "score", Value: float64(1000 + i), CollectedAt: now.Add(time.Duration(i+1) * time.Second),
			Labels: map[string]string{"site_id": seriesID, "site_name": "new"},
		}
	}
	if err := store.AppendSamples(context.Background(), history); err != nil {
		t.Fatalf("append hub history: %v", err)
	}
	var stored int
	if err := store.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM hub_metric_samples
		 WHERE scope = $1 AND labels->>'site_id' = $2`,
		telemetry.MetricScopeHubReliability, seriesID,
	).Scan(&stored); err != nil {
		t.Fatalf("count compacted hub history: %v", err)
	}
	if stored != telemetry.MaxHubMetricHistoryPerSeries {
		t.Fatalf("hub history rows = %d, want %d", stored, telemetry.MaxHubMetricHistoryPerSeries)
	}
}
