package bridge

import (
	"testing"
	"time"

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
				LatencyMs:   42.5,
				HasLatency:  true,
				Status:      1,
				CollectedAt: time.Now().UTC(),
				Labels: map[string]string{
					"check_id":   "check-http",
					"check_name": "Example HTTP",
					"check_type": "http",
				},
			},
			{
				LatencyMs:   5.0,
				HasLatency:  true,
				Status:      0,
				CollectedAt: time.Now().UTC(),
				Labels: map[string]string{
					"check_id":   "check-dns",
					"check_name": "Example DNS",
					"check_type": "dns",
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

	// Build lookup: "scope:metric:check_id" -> sample.
	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		byKey[s.Scope+":"+s.Metric+":"+s.Labels["check_id"]] = s
	}

	// --- Check 1: HTTP ---
	assertSyntheticSample(t, byKey, "check-http", telemetry.MetricSyntheticLatencyMs, "ms", 42.5)
	assertSyntheticSample(t, byKey, "check-http", telemetry.MetricSyntheticStatus, "status", 1)

	// --- Check 2: DNS ---
	assertSyntheticSample(t, byKey, "check-dns", telemetry.MetricSyntheticLatencyMs, "ms", 5.0)
	assertSyntheticSample(t, byKey, "check-dns", telemetry.MetricSyntheticStatus, "status", 0)
}

func TestSyntheticChecksBridgeLabels(t *testing.T) {
	wantLabels := map[string]string{
		"check_id":   "check-tcp",
		"check_name": "SSH",
		"check_type": "tcp",
	}

	source := &mockSyntheticChecksSource{
		entries: []SyntheticCheckEntry{
			{
				LatencyMs: 10.0, HasLatency: true, Status: 1,
				CollectedAt: time.Now().UTC(), Labels: wantLabels,
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

func assertSyntheticSample(t *testing.T, byKey map[string]telemetry.MetricSample, checkID, metric, unit string, value float64) {
	t.Helper()
	key := telemetry.MetricScopeHubSynthetic + ":" + metric + ":" + checkID
	sample, ok := byKey[key]
	if !ok {
		t.Fatalf("missing synthetic sample %s", key)
	}
	if sample.AssetID != "" || sample.Scope != telemetry.MetricScopeHubSynthetic || sample.Unit != unit || sample.Value != value || sample.CollectedAt.IsZero() {
		t.Fatalf("synthetic sample %s = %+v", key, sample)
	}
	if _, err := telemetry.NormalizeHubMetricSample(sample); err != nil {
		t.Fatalf("synthetic sample %s is not a valid hub metric: %v", key, err)
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
