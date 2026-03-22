package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestMemoryTelemetryStoreSnapshotManyReturnsLatestValues(t *testing.T) {
	store := NewMemoryTelemetryStore()
	now := time.Now().UTC()

	err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{
			AssetID:     "asset-1",
			Metric:      telemetry.MetricCPUUsedPercent,
			Unit:        "percent",
			Value:       25,
			CollectedAt: now.Add(-2 * time.Minute),
		},
		{
			AssetID:     "asset-1",
			Metric:      telemetry.MetricCPUUsedPercent,
			Unit:        "percent",
			Value:       55,
			CollectedAt: now.Add(-time.Minute),
		},
		{
			AssetID:     "asset-2",
			Metric:      telemetry.MetricMemoryUsedPercent,
			Unit:        "percent",
			Value:       70,
			CollectedAt: now.Add(-30 * time.Second),
		},
	})
	if err != nil {
		t.Fatalf("append samples failed: %v", err)
	}

	snapshots, err := store.SnapshotMany([]string{"asset-1", "asset-2", "asset-1"}, now)
	if err != nil {
		t.Fatalf("snapshot many failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected snapshots for 2 assets, got %d", len(snapshots))
	}

	assetOne := snapshots["asset-1"]
	if assetOne.CPUUsedPercent == nil || *assetOne.CPUUsedPercent != 55 {
		t.Fatalf("expected asset-1 latest cpu=55, got %+v", assetOne.CPUUsedPercent)
	}

	assetTwo := snapshots["asset-2"]
	if assetTwo.MemoryUsedPercent == nil || *assetTwo.MemoryUsedPercent != 70 {
		t.Fatalf("expected asset-2 memory=70, got %+v", assetTwo.MemoryUsedPercent)
	}
}

func TestMemoryTelemetryStoreEviction(t *testing.T) {
	store := NewMemoryTelemetryStore()
	now := time.Now().UTC()

	// Insert 1200 samples for a single asset.
	samples := make([]telemetry.MetricSample, 1200)
	for i := range samples {
		samples[i] = telemetry.MetricSample{
			AssetID:     "asset-evict",
			Metric:      telemetry.MetricCPUUsedPercent,
			Unit:        "percent",
			Value:       float64(i),
			CollectedAt: now.Add(time.Duration(i) * time.Second),
		}
	}
	if err := store.AppendSamples(context.Background(), samples); err != nil {
		t.Fatalf("append samples failed: %v", err)
	}

	// The store should have evicted down to at most maxSamplesPerAsset.
	store.mu.RLock()
	count := len(store.samples["asset-evict"])
	store.mu.RUnlock()

	if count > maxSamplesPerAsset {
		t.Fatalf("expected at most %d samples after eviction, got %d", maxSamplesPerAsset, count)
	}
	t.Logf("sample count after eviction: %d (cap %d)", count, maxSamplesPerAsset)

	// The most recent sample (value 1199) must be preserved.
	snap, err := store.Snapshot("asset-evict", now.Add(2000*time.Second))
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if snap.CPUUsedPercent == nil {
		t.Fatal("expected CPUUsedPercent in snapshot, got nil")
	}
	if *snap.CPUUsedPercent != 1199 {
		t.Fatalf("expected latest value 1199, got %v", *snap.CPUUsedPercent)
	}

	// With 1200 inserted, dropCount = 1200 - 800 = 400, so first surviving = 400.
	store.mu.RLock()
	firstValue := store.samples["asset-evict"][0].Value
	store.mu.RUnlock()
	if firstValue != 400 {
		t.Fatalf("expected oldest surviving value == 400, got %v", firstValue)
	}
	t.Logf("eviction test: oldest surviving value = %v", firstValue)
}

func TestMemoryTelemetryStoreDynamicSnapshotForAsset(t *testing.T) {
	store := NewMemoryTelemetryStore()
	now := time.Now().UTC()

	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{
			AssetID:     "asset-dyn",
			Metric:      telemetry.MetricCPUUsedPercent,
			Unit:        "percent",
			Value:       42.0,
			CollectedAt: now.Add(-2 * time.Minute),
		},
		{
			AssetID:     "asset-dyn",
			Metric:      telemetry.MetricBlockReadBytesPerSec,
			Unit:        "bytes_per_sec",
			Value:       1024.0,
			CollectedAt: now.Add(-time.Minute),
		},
		{
			AssetID:     "asset-dyn",
			Metric:      telemetry.MetricCPUUsedPercent,
			Unit:        "percent",
			Value:       55.0,
			CollectedAt: now.Add(-30 * time.Second),
		},
	}); err != nil {
		t.Fatalf("append samples failed: %v", err)
	}

	dyn, err := store.DynamicSnapshotForAsset("asset-dyn", now)
	if err != nil {
		t.Fatalf("DynamicSnapshotForAsset failed: %v", err)
	}

	// Should return all metrics, not just canonical 6.
	if got, ok := dyn.Metrics[telemetry.MetricCPUUsedPercent]; !ok || got != 55.0 {
		t.Errorf("expected cpu=55.0, got %v (ok=%v)", got, ok)
	}
	if got, ok := dyn.Metrics[telemetry.MetricBlockReadBytesPerSec]; !ok || got != 1024.0 {
		t.Errorf("expected block_read=1024.0, got %v (ok=%v)", got, ok)
	}

	// Verify ToLegacySnapshot works via the dynamic path.
	legacy := dyn.ToLegacySnapshot()
	if legacy.CPUUsedPercent == nil || *legacy.CPUUsedPercent != 55.0 {
		t.Errorf("legacy snapshot CPUUsedPercent mismatch: %v", legacy.CPUUsedPercent)
	}
}

func TestMemoryTelemetryStoreDynamicSnapshotMany(t *testing.T) {
	store := NewMemoryTelemetryStore()
	now := time.Now().UTC()

	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{
			AssetID:     "dyn-a",
			Metric:      telemetry.MetricPIDs,
			Unit:        "count",
			Value:       12.0,
			CollectedAt: now.Add(-time.Minute),
		},
		{
			AssetID:     "dyn-b",
			Metric:      telemetry.MetricMemoryUsedBytes,
			Unit:        "bytes",
			Value:       512000.0,
			CollectedAt: now.Add(-30 * time.Second),
		},
	}); err != nil {
		t.Fatalf("append samples failed: %v", err)
	}

	// Duplicate ID in input must be deduplicated.
	dynMap, err := store.DynamicSnapshotMany([]string{"dyn-a", "dyn-b", "dyn-a"}, now)
	if err != nil {
		t.Fatalf("DynamicSnapshotMany failed: %v", err)
	}
	if len(dynMap) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(dynMap))
	}

	if got, ok := dynMap["dyn-a"].Metrics[telemetry.MetricPIDs]; !ok || got != 12.0 {
		t.Errorf("dyn-a pids mismatch: got %v ok=%v", got, ok)
	}
	if got, ok := dynMap["dyn-b"].Metrics[telemetry.MetricMemoryUsedBytes]; !ok || got != 512000.0 {
		t.Errorf("dyn-b memory_used_bytes mismatch: got %v ok=%v", got, ok)
	}
}

func TestDynamicSnapshotToLegacySnapshot(t *testing.T) {
	dyn := telemetry.DynamicSnapshot{
		Metrics: map[string]float64{
			telemetry.MetricCPUUsedPercent:       33.3,
			telemetry.MetricMemoryUsedPercent:    55.5,
			telemetry.MetricDiskUsedPercent:      77.7,
			telemetry.MetricTemperatureCelsius:   42.0,
			telemetry.MetricNetworkRXBytesPerSec: 1000.0,
			telemetry.MetricNetworkTXBytesPerSec: 2000.0,
			// Extra metric that should not appear in legacy snapshot.
			telemetry.MetricBlockReadBytesPerSec: 500.0,
		},
	}

	legacy := dyn.ToLegacySnapshot()
	if legacy.CPUUsedPercent == nil || *legacy.CPUUsedPercent != 33.3 {
		t.Errorf("CPUUsedPercent: got %v", legacy.CPUUsedPercent)
	}
	if legacy.MemoryUsedPercent == nil || *legacy.MemoryUsedPercent != 55.5 {
		t.Errorf("MemoryUsedPercent: got %v", legacy.MemoryUsedPercent)
	}
	if legacy.DiskUsedPercent == nil || *legacy.DiskUsedPercent != 77.7 {
		t.Errorf("DiskUsedPercent: got %v", legacy.DiskUsedPercent)
	}
	if legacy.TemperatureCelsius == nil || *legacy.TemperatureCelsius != 42.0 {
		t.Errorf("TemperatureCelsius: got %v", legacy.TemperatureCelsius)
	}
	if legacy.NetworkRXBytesPerSec == nil || *legacy.NetworkRXBytesPerSec != 1000.0 {
		t.Errorf("NetworkRXBytesPerSec: got %v", legacy.NetworkRXBytesPerSec)
	}
	if legacy.NetworkTXBytesPerSec == nil || *legacy.NetworkTXBytesPerSec != 2000.0 {
		t.Errorf("NetworkTXBytesPerSec: got %v", legacy.NetworkTXBytesPerSec)
	}
}

func TestDynamicSnapshotEmptyMetrics(t *testing.T) {
	store := NewMemoryTelemetryStore()
	now := time.Now().UTC()

	// Asset with no samples should return empty (non-nil) Metrics map via DynamicSnapshotForAsset.
	dyn, err := store.DynamicSnapshotForAsset("no-data", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dyn.Metrics == nil {
		t.Error("expected non-nil Metrics map for asset with no samples")
	}
	if len(dyn.Metrics) != 0 {
		t.Errorf("expected empty Metrics, got %v", dyn.Metrics)
	}

	// ToLegacySnapshot from empty dynamic snapshot should return zero Snapshot.
	legacy := dyn.ToLegacySnapshot()
	if legacy.CPUUsedPercent != nil {
		t.Errorf("expected nil CPUUsedPercent, got %v", *legacy.CPUUsedPercent)
	}
}

func TestMemoryTelemetryStoreMetricSeriesBatchAndSamplePresence(t *testing.T) {
	store := NewMemoryTelemetryStore()
	now := time.Now().UTC()

	if err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{
			AssetID:     "asset-a",
			Metric:      telemetry.MetricCPUUsedPercent,
			Unit:        "percent",
			Value:       61,
			CollectedAt: now.Add(-2 * time.Minute),
		},
		{
			AssetID:     "asset-b",
			Metric:      telemetry.MetricCPUUsedPercent,
			Unit:        "percent",
			Value:       82,
			CollectedAt: now.Add(-time.Minute),
		},
	}); err != nil {
		t.Fatalf("append samples failed: %v", err)
	}

	seriesByAsset, err := store.MetricSeriesBatch([]string{"asset-a", "asset-b"}, telemetry.MetricCPUUsedPercent, now.Add(-5*time.Minute), now, 0)
	if err != nil {
		t.Fatalf("metric series batch failed: %v", err)
	}
	if len(seriesByAsset["asset-a"].Points) != 1 || seriesByAsset["asset-a"].Points[0].Value != 61 {
		t.Fatalf("expected asset-a batch series to include one point, got %+v", seriesByAsset["asset-a"])
	}
	if len(seriesByAsset["asset-b"].Points) != 1 || seriesByAsset["asset-b"].Points[0].Value != 82 {
		t.Fatalf("expected asset-b batch series to include one point, got %+v", seriesByAsset["asset-b"])
	}

	hasSamples, err := store.AssetsWithSamples([]string{"asset-a", "asset-b", "asset-c"}, now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("assets with samples failed: %v", err)
	}
	if !hasSamples["asset-a"] || !hasSamples["asset-b"] {
		t.Fatalf("expected sample presence for seeded assets, got %+v", hasSamples)
	}
	if hasSamples["asset-c"] {
		t.Fatalf("expected unseeded asset-c to report no samples, got %+v", hasSamples)
	}
}
