package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockSyntheticChecksSource implements SyntheticChecksSource for testing.
type mockSyntheticChecksSource struct {
	entries []SyntheticCheckEntry
}

func (m *mockSyntheticChecksSource) AllSyntheticCheckMetrics() []SyntheticCheckEntry {
	return m.entries
}

func TestSyntheticChecksBridgeCollect(t *testing.T) {
	source := &mockSyntheticChecksSource{
		entries: []SyntheticCheckEntry{
			{
				AssetID:   "synthetic-check-http-example-com",
				LatencyMs: 42.5,
				Status:    1,
				Labels: map[string]string{
					"check_type": "http",
					"target":     "https://example.com",
				},
			},
			{
				AssetID:   "synthetic-check-dns-example-com",
				LatencyMs: 5.0,
				Status:    0,
				Labels: map[string]string{
					"check_type": "dns",
					"target":     "example.com",
				},
			},
		},
	}

	b := NewSyntheticChecksBridge(source)

	if b.Name() != "synthetic-checks" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 4 {
		t.Fatalf("expected 4 samples (2 per check), got %d", len(samples))
	}

	// Build lookup: "assetID:metric" -> sample.
	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		byKey[s.AssetID+":"+s.Metric] = s
	}

	// --- Check 1: HTTP ---
	ct1 := "synthetic-check-http-example-com"
	assertSample(t, byKey, ct1, telemetry.MetricSyntheticLatencyMs, "ms", 42.5)
	assertSample(t, byKey, ct1, telemetry.MetricSyntheticStatus, "status", 1)

	// --- Check 2: DNS ---
	ct2 := "synthetic-check-dns-example-com"
	assertSample(t, byKey, ct2, telemetry.MetricSyntheticLatencyMs, "ms", 5.0)
	assertSample(t, byKey, ct2, telemetry.MetricSyntheticStatus, "status", 0)
}

func TestSyntheticChecksBridgeLabels(t *testing.T) {
	wantLabels := map[string]string{
		"check_type": "tcp",
		"target":     "192.168.1.1:22",
	}

	source := &mockSyntheticChecksSource{
		entries: []SyntheticCheckEntry{
			{
				AssetID:   "synthetic-check-tcp-192-168-1-1",
				LatencyMs: 10.0,
				Status:    1,
				Labels:    wantLabels,
			},
		},
	}

	b := NewSyntheticChecksBridge(source)
	samples := b.Collect()

	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
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

func TestSyntheticChecksBridgeEmpty(t *testing.T) {
	source := &mockSyntheticChecksSource{entries: nil}
	b := NewSyntheticChecksBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}
