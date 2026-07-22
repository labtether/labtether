package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/retention"
	"github.com/labtether/labtether/internal/telemetry"
)

func TestPostgresTelemetryScopesAndUnknownAssetPartialWrite(t *testing.T) {
	store := newTestPostgresStore(t)
	asset := createTestAsset(t, store, "telemetry-scope")
	now := time.Now().UTC()
	suffix := fmt.Sprintf("%d", now.UnixNano())
	assetMetric := "ltqa_asset_" + suffix
	hubRuleID := "ltqa-hub-" + suffix
	oldHubRuleID := "ltqa-hub-old-" + suffix
	unknownAssetID := "ltqa-missing-" + suffix
	invalidBatchMetric := "ltqa_invalid_batch_" + suffix

	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(),
			`DELETE FROM hub_metric_samples WHERE labels->>'rule_id' = ANY($1::text[])`,
			[]string{hubRuleID, oldHubRuleID},
		)
	})

	assetsBefore, err := store.ListAssets()
	if err != nil {
		t.Fatalf("list assets before mixed append: %v", err)
	}
	err = store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{AssetID: asset.ID, Metric: assetMetric, Unit: "count", Value: 1, CollectedAt: now},
		{AssetID: unknownAssetID, Metric: assetMetric, Unit: "count", Value: 2, CollectedAt: now},
		{
			Scope: telemetry.MetricScopeHubAlerts, Metric: telemetry.MetricAlertEvaluationDurationMs,
			Unit: "ms", Value: 3, CollectedAt: now,
			Labels: map[string]string{"rule_id": hubRuleID, "rule_name": "ltqa"},
		},
	})
	var unknownErr *UnknownMetricAssetsError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("append error = %v, want UnknownMetricAssetsError", err)
	}
	if unknownErr.SkippedSamples != 1 || len(unknownErr.AssetIDs) != 1 || unknownErr.AssetIDs[0] != unknownAssetID {
		t.Fatalf("unexpected partial-write error: %+v", unknownErr)
	}

	var stored int
	if err := store.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM metric_samples WHERE asset_id = $1 AND metric = $2`,
		asset.ID, assetMetric,
	).Scan(&stored); err != nil {
		t.Fatalf("count known asset sample: %v", err)
	}
	if stored != 1 {
		t.Fatalf("known asset samples stored = %d, want 1", stored)
	}

	assetsAfter, err := store.ListAssets()
	if err != nil {
		t.Fatalf("list assets after hub sample: %v", err)
	}
	if len(assetsAfter) != len(assetsBefore) {
		t.Fatalf("hub sample changed user-visible asset count: before=%d after=%d", len(assetsBefore), len(assetsAfter))
	}

	hubSnapshots, err := store.HubMetricSnapshots(
		context.Background(),
		now.Add(time.Second),
		telemetry.MaxHubMetricSnapshotSeries,
	)
	if err != nil {
		t.Fatalf("load hub snapshots: %v", err)
	}
	var found bool
	for _, sample := range hubSnapshots[telemetry.MetricScopeHubAlerts] {
		if sample.Metric != telemetry.MetricAlertEvaluationDurationMs || sample.Labels["rule_id"] != hubRuleID {
			continue
		}
		found = true
		if sample.AssetID != "" || sample.Value != 3 || sample.Labels["rule_name"] != "ltqa" {
			t.Fatalf("unexpected persisted hub sample: %+v", sample)
		}
	}
	if !found {
		t.Fatalf("persisted hub rule metric %q missing from snapshot", hubRuleID)
	}

	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{AssetID: asset.ID, Metric: invalidBatchMetric, Unit: "count", Value: 1, CollectedAt: now},
		{Scope: telemetry.MetricScopeHubAlerts, Metric: telemetry.MetricAlertsFiring, Unit: "count", Value: 1, CollectedAt: now, Labels: map[string]string{"unexpected": "label"}},
	}); err == nil {
		t.Fatal("invalid hub sample was accepted")
	}
	if err := store.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM metric_samples WHERE asset_id = $1 AND metric = $2`, asset.ID, invalidBatchMetric,
	).Scan(&stored); err != nil {
		t.Fatalf("count prevalidated asset sample: %v", err)
	}
	if stored != 0 {
		t.Fatalf("invalid mixed batch partially wrote %d asset samples", stored)
	}

	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{
			Scope:       telemetry.MetricScopeHubAlerts,
			Metric:      telemetry.MetricAlertEvaluationDurationMs,
			Unit:        "ms",
			Value:       1,
			CollectedAt: time.Unix(0, 0).UTC(),
			Labels:      map[string]string{"rule_id": oldHubRuleID, "rule_name": "old"},
		},
	}); err != nil {
		t.Fatalf("append old hub sample: %v", err)
	}
	settings := retention.DefaultSettings()
	settings.MetricsWindow = time.Hour
	pruneResult, err := store.PruneExpiredData(time.Unix(2*int64(time.Hour/time.Second), 0).UTC(), settings)
	if err != nil {
		t.Fatalf("prune old hub sample: %v", err)
	}
	if pruneResult.MetricsDeleted < 1 {
		t.Fatalf("metrics deleted = %d, want at least the old hub sample", pruneResult.MetricsDeleted)
	}
	if err := store.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM hub_metric_samples WHERE labels->>'rule_id' = $1`, oldHubRuleID,
	).Scan(&stored); err != nil {
		t.Fatalf("count old hub sample after prune: %v", err)
	}
	if stored != 0 {
		t.Fatalf("old hub samples remaining after prune = %d, want 0", stored)
	}
}
