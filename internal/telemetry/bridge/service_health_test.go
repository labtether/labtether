package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockServiceHealthSource implements ServiceHealthSource for testing.
type mockServiceHealthSource struct {
	entries []ServiceHealthEntry
}

func (m *mockServiceHealthSource) AllServiceHealth() []ServiceHealthEntry {
	return m.entries
}

func TestServiceHealthBridgeCollect(t *testing.T) {
	source := &mockServiceHealthSource{
		entries: []ServiceHealthEntry{
			{
				AssetID:       "web-asset-monitor01",
				ResponseMs:    142.5,
				UptimePercent: 99.9,
				Status:        1,
				Labels: map[string]string{
					"service_name": "homepage",
					"service_url":  "https://example.com",
				},
			},
			{
				AssetID:       "web-asset-monitor01",
				ResponseMs:    0,
				UptimePercent: 72.3,
				Status:        0,
				Labels: map[string]string{
					"service_name": "api",
					"service_url":  "https://api.example.com/health",
				},
			},
		},
	}

	b := NewServiceHealthBridge(source)

	if b.Name() != "service-health" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 6 {
		t.Fatalf("expected 6 samples (3 per service), got %d", len(samples))
	}

	// Build lookup: "assetID:metric:service_name" -> sample.
	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		svcName := s.Labels["service_name"]
		byKey[s.AssetID+":"+s.Metric+":"+svcName] = s
	}

	assetID := "web-asset-monitor01"

	// homepage (up)
	assertSampleBySvc(t, byKey, assetID, telemetry.MetricServiceResponseMs, "milliseconds", 142.5, "homepage")
	assertSampleBySvc(t, byKey, assetID, telemetry.MetricServiceUptimePercent, "percent", 99.9, "homepage")
	assertSampleBySvc(t, byKey, assetID, telemetry.MetricServiceStatus, "status", 1, "homepage")

	// api (down)
	assertSampleBySvc(t, byKey, assetID, telemetry.MetricServiceResponseMs, "milliseconds", 0, "api")
	assertSampleBySvc(t, byKey, assetID, telemetry.MetricServiceUptimePercent, "percent", 72.3, "api")
	assertSampleBySvc(t, byKey, assetID, telemetry.MetricServiceStatus, "status", 0, "api")
}

func TestServiceHealthBridgeEmpty(t *testing.T) {
	source := &mockServiceHealthSource{entries: nil}
	b := NewServiceHealthBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}

func TestServiceHealthBridgeLabelsHaveServiceNameAndURL(t *testing.T) {
	wantLabels := map[string]string{
		"service_name": "grafana",
		"service_url":  "http://grafana.local:3000",
	}

	source := &mockServiceHealthSource{
		entries: []ServiceHealthEntry{
			{
				AssetID:       "web-asset-monitor02",
				ResponseMs:    80.0,
				UptimePercent: 100.0,
				Status:        1,
				Labels:        wantLabels,
			},
		},
	}

	b := NewServiceHealthBridge(source)
	samples := b.Collect()

	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}

	for _, s := range samples {
		if s.Labels == nil {
			t.Errorf("metric %q: expected labels, got nil", s.Metric)
			continue
		}
		for k, want := range wantLabels {
			got, ok := s.Labels[k]
			if !ok {
				t.Errorf("metric %q: missing label %q", s.Metric, k)
				continue
			}
			if got != want {
				t.Errorf("metric %q label %q: got %q, want %q", s.Metric, k, got, want)
			}
		}
	}
}

// assertSampleBySvc looks up a sample by "assetID:metric:service_name" key.
func assertSampleBySvc(
	t *testing.T,
	byKey map[string]telemetry.MetricSample,
	assetID, metric, wantUnit string,
	wantValue float64,
	svcName string,
) {
	t.Helper()
	key := assetID + ":" + metric + ":" + svcName
	s, ok := byKey[key]
	if !ok {
		t.Errorf("missing sample assetID=%q metric=%q service_name=%q", assetID, metric, svcName)
		return
	}
	if s.Unit != wantUnit {
		t.Errorf("sample %q/%q/%q: unit=%q, want %q", assetID, metric, svcName, s.Unit, wantUnit)
	}
	if s.Value != wantValue {
		t.Errorf("sample %q/%q/%q: value=%v, want %v", assetID, metric, svcName, s.Value, wantValue)
	}
	if s.CollectedAt.IsZero() {
		t.Errorf("sample %q/%q/%q: CollectedAt is zero", assetID, metric, svcName)
	}
}
