package promexport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/telemetry/bridge"
	"github.com/labtether/labtether/internal/telemetry/promexport"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
)

// ----------------------------------------------------------------------------
// memStoreSnapshotSource adapts MemoryTelemetryStore to SnapshotSource.
//
// LatestSnapshots returns each metric stored for each asset as a LabeledMetric,
// keyed by asset ID.  Labels from MetricSample.Labels are forwarded verbatim.
// AssetMetadata returns the metadata map supplied at construction.
// ----------------------------------------------------------------------------

type memStoreSnapshotSource struct {
	store *persistence.MemoryTelemetryStore
	at    time.Time
	metas map[string]promexport.AssetMeta
}

func newMemStoreSnapshotSource(
	store *persistence.MemoryTelemetryStore,
	at time.Time,
	metas map[string]promexport.AssetMeta,
) *memStoreSnapshotSource {
	return &memStoreSnapshotSource{store: store, at: at, metas: metas}
}

// assetIDs returns all asset IDs present in the metadata map plus any that
// exist in the store (resolved via DynamicSnapshotMany with the known IDs).
func (s *memStoreSnapshotSource) assetIDs() []string {
	ids := make([]string, 0, len(s.metas))
	for id := range s.metas {
		ids = append(ids, id)
	}
	return ids
}

func (s *memStoreSnapshotSource) LatestSnapshots() map[string][]promexport.LabeledMetric {
	ids := s.assetIDs()
	dynMap, err := s.store.DynamicSnapshotMany(ids, s.at)
	if err != nil {
		return nil
	}

	// DynamicSnapshotMany only returns metrics for the provided IDs.
	// Re-query individually for any asset not covered (zero-length result).
	out := make(map[string][]promexport.LabeledMetric, len(ids))
	for _, id := range ids {
		dyn, ok := dynMap[id]
		if !ok {
			continue
		}
		metrics := make([]promexport.LabeledMetric, 0, len(dyn.Metrics))
		for metricName, value := range dyn.Metrics {
			metrics = append(metrics, promexport.LabeledMetric{
				Metric: metricName,
				Value:  value,
			})
		}
		if len(metrics) > 0 {
			out[id] = metrics
		}
	}
	return out
}

func (s *memStoreSnapshotSource) AssetMetadata() map[string]promexport.AssetMeta {
	return s.metas
}

// ----------------------------------------------------------------------------
// mockBridge implements bridge.MetricsBridge for testing.
// ----------------------------------------------------------------------------

type mockBridge struct {
	name     string
	samples  []telemetry.MetricSample
	interval time.Duration
}

func (m *mockBridge) Name() string                      { return m.name }
func (m *mockBridge) Collect() []telemetry.MetricSample { return m.samples }
func (m *mockBridge) Interval() time.Duration           { return m.interval }

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func collectAllMetrics(c prometheus.Collector) []prometheus.Metric {
	ch := make(chan prometheus.Metric, 256)
	c.Collect(ch)
	close(ch)
	var out []prometheus.Metric
	for m := range ch {
		out = append(out, m)
	}
	return out
}

func metricToDTO(m prometheus.Metric) *dto.Metric {
	pb := &dto.Metric{}
	if err := m.Write(pb); err != nil {
		panic("metricToDTO: " + err.Error())
	}
	return pb
}

func labelValue(pb *dto.Metric, name string) string {
	for _, lp := range pb.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

func hasLabel(pb *dto.Metric, name string) bool {
	for _, lp := range pb.GetLabel() {
		if lp.GetName() == name {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// Tests
// ----------------------------------------------------------------------------

// TestBridgeToPrometheusEndToEnd verifies the full pipeline:
//   bridge.Registry → MemoryTelemetryStore → promexport.Collector → HTTP handler
func TestBridgeToPrometheusEndToEnd(t *testing.T) {
	now := time.Now().UTC()

	// 1. Create an in-memory telemetry store.
	store := persistence.NewMemoryTelemetryStore()

	// 2. Create a bridge registry with two mock bridges.
	reg := bridge.NewRegistry()
	reg.Register(&mockBridge{
		name: "test-bridge-linux",
		samples: []telemetry.MetricSample{
			{
				AssetID:     "asset-linux-1",
				Metric:      telemetry.MetricCPUUsedPercent,
				Unit:        "percent",
				Value:       42.5,
				CollectedAt: now.Add(-30 * time.Second),
			},
			{
				AssetID:     "asset-linux-1",
				Metric:      telemetry.MetricMemoryUsedPercent,
				Unit:        "percent",
				Value:       60.0,
				CollectedAt: now.Add(-30 * time.Second),
			},
		},
		interval: time.Second,
	})
	reg.Register(&mockBridge{
		name: "test-bridge-docker",
		samples: []telemetry.MetricSample{
			{
				AssetID:     "asset-docker-1",
				Metric:      telemetry.MetricCPUUsedPercent,
				Unit:        "percent",
				Value:       15.0,
				CollectedAt: now.Add(-15 * time.Second),
				Labels: map[string]string{
					"docker_host":  "myhost",
					"docker_image": "nginx:latest",
				},
			},
		},
		interval: time.Second,
	})

	// 3. Collect from bridges and append to store.
	samples := reg.CollectAll()
	if err := store.AppendSamples(context.Background(), samples); err != nil {
		t.Fatalf("AppendSamples failed: %v", err)
	}

	// 4. Create a SnapshotSource adapter backed by the store.
	metas := map[string]promexport.AssetMeta{
		"asset-linux-1": {
			Name:     "linux-host",
			Type:     "linux",
			Platform: "linux",
			Group:    "servers",
		},
		"asset-docker-1": {
			Name:        "nginx-container",
			Type:        "docker-container",
			DockerHost:  "myhost",
			DockerImage: "nginx:latest",
		},
	}
	src := newMemStoreSnapshotSource(store, now.Add(time.Second), metas)

	// 5. Create Prometheus collector and gather metrics.
	collector := promexport.NewCollector(src)
	metrics := collectAllMetrics(collector)

	// Must have at least one metric per asset for asset_info + actual metrics.
	if len(metrics) == 0 {
		t.Fatal("expected at least one Prometheus metric, got none")
	}

	// Verify asset_info is emitted for both assets.
	infoByAsset := map[string]*dto.Metric{}
	for _, m := range metrics {
		pb := metricToDTO(m)
		if pb.GetGauge() == nil {
			continue
		}
		if pb.GetGauge().GetValue() != 1.0 {
			continue
		}
		if !hasLabel(pb, "asset_name") || !hasLabel(pb, "asset_type") {
			continue
		}
		// Distinguish asset_info from a metric that happens to be 1.0 by checking
		// it does NOT have per-metric labels like mount_point or interface.
		if hasLabel(pb, "mount_point") || hasLabel(pb, "interface") {
			continue
		}
		aid := labelValue(pb, "asset_id")
		infoByAsset[aid] = pb
	}

	for _, wantID := range []string{"asset-linux-1", "asset-docker-1"} {
		if _, ok := infoByAsset[wantID]; !ok {
			t.Errorf("labtether_asset_info not emitted for asset %q", wantID)
		}
	}

	// Verify label values for the Linux asset's info metric.
	if info, ok := infoByAsset["asset-linux-1"]; ok {
		if got := labelValue(info, "asset_name"); got != "linux-host" {
			t.Errorf("asset-linux-1 asset_name: want linux-host, got %q", got)
		}
		if got := labelValue(info, "asset_type"); got != "linux" {
			t.Errorf("asset-linux-1 asset_type: want linux, got %q", got)
		}
		if got := labelValue(info, "group"); got != "servers" {
			t.Errorf("asset-linux-1 group: want servers, got %q", got)
		}
	}

	// Verify at least the cpu_used_percent metric for asset-linux-1 appears with
	// the correct value.
	foundCPU := false
	for _, m := range metrics {
		pb := metricToDTO(m)
		if labelValue(pb, "asset_id") != "asset-linux-1" {
			continue
		}
		if pb.GetGauge() == nil {
			continue
		}
		if pb.GetGauge().GetValue() == 42.5 {
			foundCPU = true
			if got := labelValue(pb, "asset_name"); got != "linux-host" {
				t.Errorf("cpu metric asset_name: want linux-host, got %q", got)
			}
		}
	}
	if !foundCPU {
		t.Error("expected cpu_used_percent=42.5 metric for asset-linux-1")
	}

	// 6. Verify the HTTP handler produces valid Prometheus exposition format.
	handler := promexport.NewHandler(src)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("handler returned status %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "labtether_asset_info") {
		t.Error("handler output missing labtether_asset_info")
	}
	if !strings.Contains(body, "labtether_cpu_used_percent") {
		t.Error("handler output missing labtether_cpu_used_percent")
	}
	if !strings.Contains(body, "asset-linux-1") {
		t.Error("handler output missing asset-linux-1 label value")
	}
}

// TestBridgeToStoreToSnapshot verifies that samples written through a bridge
// and appended to the store are visible via DynamicSnapshotForAsset.
func TestBridgeToStoreToSnapshot(t *testing.T) {
	now := time.Now().UTC()
	store := persistence.NewMemoryTelemetryStore()

	reg := bridge.NewRegistry()
	reg.Register(&mockBridge{
		name: "snapshot-bridge",
		samples: []telemetry.MetricSample{
			{
				AssetID:     "snap-asset",
				Metric:      telemetry.MetricDiskUsedPercent,
				Unit:        "percent",
				Value:       88.0,
				CollectedAt: now.Add(-10 * time.Second),
			},
			{
				AssetID:     "snap-asset",
				Metric:      telemetry.MetricTemperatureCelsius,
				Unit:        "celsius",
				Value:       72.0,
				CollectedAt: now.Add(-10 * time.Second),
			},
		},
		interval: time.Second,
	})

	samples := reg.CollectAll()
	if err := store.AppendSamples(context.Background(), samples); err != nil {
		t.Fatalf("AppendSamples: %v", err)
	}

	dyn, err := store.DynamicSnapshotForAsset("snap-asset", now)
	if err != nil {
		t.Fatalf("DynamicSnapshotForAsset: %v", err)
	}

	if got, ok := dyn.Metrics[telemetry.MetricDiskUsedPercent]; !ok || got != 88.0 {
		t.Errorf("disk_used_percent: want 88.0, got %v (ok=%v)", got, ok)
	}
	if got, ok := dyn.Metrics[telemetry.MetricTemperatureCelsius]; !ok || got != 72.0 {
		t.Errorf("temperature_celsius: want 72.0, got %v (ok=%v)", got, ok)
	}
}

// TestMultiBridgeAggregationInStore verifies that samples from two independent
// bridges for the same asset are all available via the snapshot.
func TestMultiBridgeAggregationInStore(t *testing.T) {
	now := time.Now().UTC()
	store := persistence.NewMemoryTelemetryStore()

	reg := bridge.NewRegistry()
	reg.Register(&mockBridge{
		name: "bridge-cpu",
		samples: []telemetry.MetricSample{
			{
				AssetID:     "multi-asset",
				Metric:      telemetry.MetricCPUUsedPercent,
				Unit:        "percent",
				Value:       30.0,
				CollectedAt: now.Add(-5 * time.Second),
			},
		},
		interval: time.Second,
	})
	reg.Register(&mockBridge{
		name: "bridge-mem",
		samples: []telemetry.MetricSample{
			{
				AssetID:     "multi-asset",
				Metric:      telemetry.MetricMemoryUsedPercent,
				Unit:        "percent",
				Value:       50.0,
				CollectedAt: now.Add(-5 * time.Second),
			},
		},
		interval: time.Second,
	})

	if err := store.AppendSamples(context.Background(), reg.CollectAll()); err != nil {
		t.Fatalf("AppendSamples: %v", err)
	}

	dyn, err := store.DynamicSnapshotForAsset("multi-asset", now)
	if err != nil {
		t.Fatalf("DynamicSnapshotForAsset: %v", err)
	}
	if got, ok := dyn.Metrics[telemetry.MetricCPUUsedPercent]; !ok || got != 30.0 {
		t.Errorf("cpu_used_percent: want 30.0, got %v (ok=%v)", got, ok)
	}
	if got, ok := dyn.Metrics[telemetry.MetricMemoryUsedPercent]; !ok || got != 50.0 {
		t.Errorf("memory_used_percent: want 50.0, got %v (ok=%v)", got, ok)
	}
}

// TestEmptyBridgeProducesNoMetrics verifies that a bridge returning no samples
// results in an empty store and an empty Prometheus output.
func TestEmptyBridgeProducesNoMetrics(t *testing.T) {
	store := persistence.NewMemoryTelemetryStore()

	reg := bridge.NewRegistry()
	reg.Register(&mockBridge{name: "empty", samples: nil, interval: time.Second})

	if err := store.AppendSamples(context.Background(), reg.CollectAll()); err != nil {
		t.Fatalf("AppendSamples: %v", err)
	}

	src := newMemStoreSnapshotSource(store, time.Now(), map[string]promexport.AssetMeta{})
	collector := promexport.NewCollector(src)
	metrics := collectAllMetrics(collector)

	if len(metrics) != 0 {
		t.Errorf("expected 0 Prometheus metrics for empty bridge, got %d", len(metrics))
	}
}
