package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestPostgresAppendSamplesChunksPastBindLimitInOneTransaction(t *testing.T) {
	store := newTestPostgresStore(t)
	asset := createTestAsset(t, store, "telemetry-chunk")
	now := time.Now().UTC()
	suffix := fmt.Sprintf("%d", now.UnixNano())
	metric := "ltqa_chunk_" + suffix
	hubRuleID := "ltqa-chunk-hub-" + suffix
	unknownA := "ltqa-chunk-missing-a-" + suffix
	unknownB := "ltqa-chunk-missing-b-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM hub_metric_samples WHERE labels->>'rule_id' = $1`, hubRuleID)
	})

	const assetSampleCount = 10923 // one more than floor(65,535 / 6)
	samples := make([]telemetry.MetricSample, 0, assetSampleCount+1)
	for i := 0; i < assetSampleCount; i++ {
		assetID := asset.ID
		switch i {
		case 0:
			assetID = unknownA
		case maxMetricSamplesPerInsert:
			assetID = unknownB
		}
		samples = append(samples, telemetry.MetricSample{
			AssetID: assetID, Metric: metric, Unit: "count", Value: float64(i), CollectedAt: now,
		})
	}
	samples = append(samples, telemetry.MetricSample{
		Scope: telemetry.MetricScopeHubAlerts, Metric: telemetry.MetricAlertEvaluationDurationMs,
		Unit: "ms", Value: 7, CollectedAt: now,
		Labels: map[string]string{"rule_id": hubRuleID, "rule_name": "chunk"},
	})

	err := store.AppendSamples(context.Background(), samples)
	var unknownErr *UnknownMetricAssetsError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("append error = %v, want UnknownMetricAssetsError", err)
	}
	if unknownErr.SkippedSamples != 2 || len(unknownErr.AssetIDs) != 2 || unknownErr.AssetIDs[0] != unknownA || unknownErr.AssetIDs[1] != unknownB {
		t.Fatalf("unknown assets were not aggregated across chunks: %+v", unknownErr)
	}
	var stored int
	if err := store.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM metric_samples WHERE asset_id = $1 AND metric = $2`, asset.ID, metric,
	).Scan(&stored); err != nil {
		t.Fatalf("count chunked asset samples: %v", err)
	}
	if stored != assetSampleCount-2 {
		t.Fatalf("chunked known samples = %d, want %d", stored, assetSampleCount-2)
	}
	if err := store.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM hub_metric_samples WHERE labels->>'rule_id' = $1`, hubRuleID,
	).Scan(&stored); err != nil {
		t.Fatalf("count transactional hub sample: %v", err)
	}
	if stored != 1 {
		t.Fatalf("hub samples committed with chunked partial write = %d, want 1", stored)
	}
}
