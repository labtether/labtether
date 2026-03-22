package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockSiteReliabilitySource implements SiteReliabilitySource for testing.
type mockSiteReliabilitySource struct {
	entries []SiteReliabilityEntry
}

func (m *mockSiteReliabilitySource) AllSiteReliabilityMetrics() []SiteReliabilityEntry {
	return m.entries
}

func TestSiteReliabilityBridgeCollect(t *testing.T) {
	source := &mockSiteReliabilitySource{
		entries: []SiteReliabilityEntry{
			{
				Score: 99.5,
				Labels: map[string]string{
					"site_id":   "site-abc",
					"site_name": "Primary DC",
				},
			},
		},
	}

	b := NewSiteReliabilityBridge(source)

	if b.Name() != "site-reliability" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}

	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		byKey[s.AssetID+":"+s.Metric] = s
	}

	assertSample(t, byKey, "hub-reliability", telemetry.MetricSiteReliabilityScore, "score", 99.5)
}

func TestSiteReliabilityBridgeLabels(t *testing.T) {
	wantLabels := map[string]string{
		"site_id":   "site-xyz",
		"site_name": "DR Site",
	}

	source := &mockSiteReliabilitySource{
		entries: []SiteReliabilityEntry{
			{
				Score:  85.0,
				Labels: wantLabels,
			},
		},
	}

	b := NewSiteReliabilityBridge(source)
	samples := b.Collect()

	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}

	s := samples[0]
	if s.Labels == nil {
		t.Fatal("expected labels, got nil")
	}
	for k, want := range wantLabels {
		got, ok := s.Labels[k]
		if !ok {
			t.Errorf("missing label %q", k)
			continue
		}
		if got != want {
			t.Errorf("label %q: got %q, want %q", k, got, want)
		}
	}
}

func TestSiteReliabilityBridgeEmpty(t *testing.T) {
	source := &mockSiteReliabilitySource{entries: nil}
	b := NewSiteReliabilityBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}
