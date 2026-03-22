package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockAlertStateSource implements AlertStateSource for testing.
type mockAlertStateSource struct {
	entries []AlertStateEntry
}

func (m *mockAlertStateSource) AllAlertStateMetrics() []AlertStateEntry {
	return m.entries
}

func TestAlertStateBridgeCollect(t *testing.T) {
	source := &mockAlertStateSource{
		entries: []AlertStateEntry{
			{
				FiringCount: 3,
				RulesCount:  12,
				Labels:      map[string]string{},
			},
		},
	}

	b := NewAlertStateBridge(source)

	if b.Name() != "alert-state" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}

	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		byKey[s.AssetID+":"+s.Metric] = s
	}

	assertSample(t, byKey, "hub-alerts", telemetry.MetricAlertsFiring, "count", 3)
	assertSample(t, byKey, "hub-alerts", telemetry.MetricAlertsRules, "count", 12)
}

func TestAlertStateBridgeZeroValues(t *testing.T) {
	source := &mockAlertStateSource{
		entries: []AlertStateEntry{
			{
				FiringCount: 0,
				RulesCount:  0,
				Labels:      nil,
			},
		},
	}

	b := NewAlertStateBridge(source)
	samples := b.Collect()

	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}

	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		byKey[s.AssetID+":"+s.Metric] = s
	}

	assertSample(t, byKey, "hub-alerts", telemetry.MetricAlertsFiring, "count", 0)
	assertSample(t, byKey, "hub-alerts", telemetry.MetricAlertsRules, "count", 0)
}

func TestAlertStateBridgeEmpty(t *testing.T) {
	source := &mockAlertStateSource{entries: nil}
	b := NewAlertStateBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}
