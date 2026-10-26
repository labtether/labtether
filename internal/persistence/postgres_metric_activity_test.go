package persistence

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/telemetry"
)

// The memory alert test cannot prove that SQL isolates the selected metric,
// requested assets and inclusive date bounds. Never run this on a shared DB.
func TestPostgresMetricActivitySelection(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var existingAssets, existingSamples int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM assets),
	 (SELECT COUNT(*) FROM metric_samples)`).Scan(&existingAssets, &existingSamples); err != nil || existingAssets != 0 || existingSamples != 0 {
		t.Fatalf("metric query fixture requires empty owned tables: assets=%d samples=%d err=%v", existingAssets, existingSamples, err)
	}
	prefix := "metric-activity-" + strings.ToLower(os.Getenv("LABTETHER_JOB_ID")) + "-"
	var ownedIDs []string
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if len(ownedIDs) == 0 {
			return
		}
		if _, err := s.pool.Exec(cleanupCtx, `DELETE FROM assets WHERE id=ANY($1::text[])`, ownedIDs); err != nil {
			t.Errorf("remove owned metric fixture assets: %v", err)
		}
		var remains int
		if err := s.pool.QueryRow(cleanupCtx, `SELECT (SELECT COUNT(*) FROM assets WHERE id=ANY($1::text[]))+
		 (SELECT COUNT(*) FROM metric_samples WHERE asset_id=ANY($1::text[]))+
		 (SELECT COUNT(*) FROM asset_heartbeats WHERE asset_id=ANY($1::text[]))`, ownedIDs).Scan(&remains); err != nil || remains != 0 {
			t.Errorf("owned metric fixture cleanup incomplete: %d/%v", remains, err)
		}
	})
	for _, suffix := range []string{"start", "end", "other-metric", "old", "future", "foreign"} {
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
		id := prefix + suffix
		if _, found, err := s.GetAsset(id); err != nil || found {
			t.Fatalf("fixture asset collision: %s/%v", id, err)
		}
		ownedIDs = append(ownedIDs, id)
		row, err := s.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: "host", Source: "test", Status: "online"})
		if err != nil || row.ID != id {
			t.Fatalf("seed metric fixture asset: %s/%v", id, err)
		}
	}
	end := time.Now().UTC().Truncate(time.Microsecond)
	start := end.Add(-time.Minute)
	metric := "probe.custom_count"
	samples := []telemetry.MetricSample{
		{AssetID: prefix + "start", Metric: metric, Unit: "count", Value: 1, CollectedAt: start},
		{AssetID: prefix + "end", Metric: metric, Unit: "count", Value: 1, CollectedAt: end},
		{AssetID: prefix + "other-metric", Metric: "probe.other_count", Unit: "count", Value: 1, CollectedAt: end},
		{AssetID: prefix + "old", Metric: metric, Unit: "count", Value: 1, CollectedAt: start.Add(-time.Microsecond)},
		{AssetID: prefix + "future", Metric: metric, Unit: "count", Value: 1, CollectedAt: end.Add(time.Microsecond)},
		{AssetID: prefix + "foreign", Metric: metric, Unit: "count", Value: 1, CollectedAt: end},
	}
	if err := s.AppendSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	memory := NewMemoryTelemetryStore()
	if err := memory.AppendSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	requested := []string{prefix + "start", " " + prefix + "end ", prefix + "start", "", prefix + "other-metric", prefix + "old", prefix + "future", prefix + "absent"}
	for name, store := range map[string]TelemetryMetricActivityStore{"postgres": s, "memory": memory} {
		for _, tc := range []struct {
			metric string
			want   map[string]bool
		}{
			{metric, map[string]bool{prefix + "start": true, prefix + "end": true}},
			{"probe.other_count", map[string]bool{prefix + "other-metric": true}},
		} {
			got, err := store.AssetsWithMetricSamples(requested, tc.metric, start, end)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s metric/date/asset presence: got=%v want=%v err=%v", name, got, tc.want, err)
			}
		}
	}
}
