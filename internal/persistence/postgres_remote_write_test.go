package persistence

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/telemetry/remotewrite"
)

func TestPostgresRemoteWriteCursorAndEqualTimestampPagination(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	asset := createTestAsset(t, store, "remote-write-page")
	var baselineAssetID, baselineHubID int64
	if err := store.pool.QueryRow(ctx, `SELECT COALESCE(MAX(id), 0) FROM metric_samples`).Scan(&baselineAssetID); err != nil {
		t.Fatalf("load asset baseline: %v", err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT COALESCE(MAX(id), 0) FROM hub_metric_samples`).Scan(&baselineHubID); err != nil {
		t.Fatalf("load hub baseline: %v", err)
	}
	now := time.Now().UTC()
	samples := make([]telemetry.MetricSample, 260)
	for index := range samples {
		samples[index] = telemetry.MetricSample{
			AssetID: asset.ID, Metric: telemetry.MetricCPUUsedPercent, Unit: "percent",
			Value: float64(index), CollectedAt: now,
			Labels: map[string]string{"process_pid": string(rune('a'+index%26)) + string(rune('a'+(index/26)%26))},
		}
	}
	if err := store.AppendSamples(ctx, samples); err != nil {
		t.Fatalf("append equal-time samples: %v", err)
	}
	if err := store.AppendSamples(ctx, []telemetry.MetricSample{{
		Scope: telemetry.MetricScopeHubAlerts, Metric: telemetry.MetricAlertsFiring,
		Unit: "count", Value: 1, CollectedAt: now,
	}}); err != nil {
		t.Fatalf("append hub sample: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM prometheus_remote_write_state`)
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM hub_metric_samples WHERE id > $1`, baselineHubID)
	})

	first, err := store.SamplesAfter(ctx, remotewrite.Cursor{AssetSampleID: baselineAssetID, HubSampleID: baselineHubID}, remotewrite.MaxSamplesPerRequest)
	if err != nil {
		t.Fatalf("SamplesAfter first: %v", err)
	}
	if first.Next.AssetSampleID <= baselineAssetID || first.Next.HubSampleID <= baselineHubID || len(first.Samples) != 261 {
		t.Fatalf("first page mismatch: next=%+v samples=%d", first.Next, len(first.Samples))
	}
	second, err := store.SamplesAfter(ctx, first.Next, remotewrite.MaxSamplesPerRequest)
	if err != nil {
		t.Fatalf("SamplesAfter second: %v", err)
	}
	if got := len(first.Samples) + len(second.Samples); got != 261 {
		t.Fatalf("equal-time pagination exported %d samples, want 261", got)
	}
	if len(second.Samples) != 0 || second.Next != first.Next {
		t.Fatalf("second page should be empty and stable: first=%+v second=%+v samples=%d", first.Next, second.Next, len(second.Samples))
	}

	fingerprintA := strings.Repeat("a", 64)
	fingerprintB := strings.Repeat("b", 64)
	initial, err := store.LoadRemoteWriteCursor(ctx, fingerprintA)
	if err != nil || initial != (remotewrite.Cursor{}) {
		t.Fatalf("initial cursor=%+v error=%v", initial, err)
	}
	if err := store.SaveRemoteWriteCursor(ctx, fingerprintA, second.Next, now); err != nil {
		t.Fatalf("SaveRemoteWriteCursor: %v", err)
	}
	reloaded, err := store.LoadRemoteWriteCursor(ctx, fingerprintA)
	if err != nil || reloaded != second.Next {
		t.Fatalf("reloaded cursor=%+v want=%+v error=%v", reloaded, second.Next, err)
	}
	reset, err := store.LoadRemoteWriteCursor(ctx, fingerprintB)
	if err != nil || reset != (remotewrite.Cursor{}) {
		t.Fatalf("replacement endpoint cursor=%+v error=%v", reset, err)
	}
	if err := store.SaveRemoteWriteCursor(ctx, fingerprintA, second.Next, now); err == nil {
		t.Fatal("stale endpoint advanced the replacement cursor")
	}
}
