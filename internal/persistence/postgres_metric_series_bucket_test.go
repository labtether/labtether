package persistence

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/telemetry"
)

// Series averages into step buckets inside Postgres. The charts depend on it
// giving the same buckets as telemetry.BucketAveragePoints, which the memory
// store and the other query paths use: whole seconds, floored to the step,
// with per-mount and per-interface samples left out.
func TestPostgresSeriesBucketsMatchGoBucketing(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping metric series bucket integration test")
	}
	store := mustOpenPostgresStoreForMigrationRecovery(t, dbURL)
	t.Cleanup(store.Close)

	ctx := context.Background()
	assetID := "series-bucket-" + time.Now().UTC().Format("20060102150405.000000000")
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Name: assetID, Type: "host", Source: "manual", Status: "online",
	}); err != nil {
		t.Fatalf("create device: %v", err)
	}
	t.Cleanup(func() { _, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID) })

	step := 10 * time.Minute
	base := time.Now().UTC().Add(-3 * time.Hour).Truncate(step)
	offsets := []time.Duration{
		0, 10 * time.Second, 9*time.Minute + 59*time.Second + 900*time.Millisecond, // one bucket, last sample just inside it
		10 * time.Minute, 10*time.Minute + 500*time.Millisecond, // next bucket
		47 * time.Minute, // a later bucket after a gap
	}
	raw := make([]telemetry.Point, 0, len(offsets))
	for i, offset := range offsets {
		at := base.Add(offset)
		value := float64(10 + i*7)
		if _, err := store.pool.Exec(ctx,
			`INSERT INTO metric_samples (asset_id, metric, unit, value, collected_at) VALUES ($1, $2, 'percent', $3, $4)`,
			assetID, telemetry.MetricCPUUsedPercent, value, at); err != nil {
			t.Fatalf("insert sample: %v", err)
		}
		raw = append(raw, telemetry.Point{TS: at.Unix(), Value: value})
	}
	// A per-mount sample of the same metric must not move the device average.
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO metric_samples (asset_id, metric, unit, value, collected_at, labels) VALUES ($1, $2, 'percent', 99, $3, '{"mount_point":"/data"}')`,
		assetID, telemetry.MetricCPUUsedPercent, base.Add(time.Minute)); err != nil {
		t.Fatalf("insert labeled sample: %v", err)
	}

	series, err := store.Series(assetID, base.Add(-time.Minute), base.Add(time.Hour), step)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	var got []telemetry.Point
	for _, entry := range series {
		if entry.Metric == telemetry.MetricCPUUsedPercent {
			got = entry.Points
		}
	}
	want := telemetry.BucketAveragePoints(raw, step)
	if len(want) != 3 {
		t.Fatalf("fixture should span 3 buckets, spans %d", len(want))
	}
	if len(got) != len(want) {
		t.Fatalf("got %d buckets %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i].TS != want[i].TS || math.Abs(got[i].Value-want[i].Value) > 1e-9 {
			t.Fatalf("bucket %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
