package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockAlertStateSource implements AlertStateSource for testing.
type mockAlertStateSource struct {
	entries []AlertStateEntry
}

type mockAlertStateSnapshotSource struct {
	entries     []AlertStateEntry
	evaluations []AlertRuleEvalEntry
}

func (m *mockAlertStateSnapshotSource) AllAlertStateMetrics() []AlertStateEntry {
	panic("snapshot-capable source must not use the legacy aggregate path")
}

func (m *mockAlertStateSnapshotSource) AllAlertMetricsSnapshot() ([]AlertStateEntry, []AlertRuleEvalEntry) {
	return m.entries, m.evaluations
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
		if s.AssetID != "" {
			t.Fatalf("hub metric unexpectedly referenced asset %q", s.AssetID)
		}
		byKey[s.Scope+":"+s.Metric] = s
	}

	assertSample(t, byKey, telemetry.MetricScopeHubAlerts, telemetry.MetricAlertsFiring, "count", 3)
	assertSample(t, byKey, telemetry.MetricScopeHubAlerts, telemetry.MetricAlertsRules, "count", 12)
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
		if s.AssetID != "" {
			t.Fatalf("hub metric unexpectedly referenced asset %q", s.AssetID)
		}
		byKey[s.Scope+":"+s.Metric] = s
	}

	assertSample(t, byKey, telemetry.MetricScopeHubAlerts, telemetry.MetricAlertsFiring, "count", 0)
	assertSample(t, byKey, telemetry.MetricScopeHubAlerts, telemetry.MetricAlertsRules, "count", 0)
}

func TestAlertStateBridgeEmpty(t *testing.T) {
	source := &mockAlertStateSource{entries: nil}
	b := NewAlertStateBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}

func TestAlertStateBridgeSnapshotKeepsDuplicateRuleNamesDistinct(t *testing.T) {
	source := &mockAlertStateSnapshotSource{
		entries: []AlertStateEntry{{FiringCount: 501, RulesCount: 502}},
		evaluations: []AlertRuleEvalEntry{
			{RuleID: "rule-a", RuleName: "duplicate", DurationMS: 8},
			{RuleID: "rule-b", RuleName: "duplicate", DurationMS: 9},
		},
	}
	samples := NewAlertStateBridge(source).Collect()
	if len(samples) != 4 {
		t.Fatalf("sample count = %d, want 4", len(samples))
	}
	byRuleID := make(map[string]telemetry.MetricSample)
	for _, sample := range samples {
		if sample.Metric != telemetry.MetricAlertEvaluationDurationMs {
			continue
		}
		byRuleID[sample.Labels["rule_id"]] = sample
	}
	if len(byRuleID) != 2 || byRuleID["rule-a"].Value != 8 || byRuleID["rule-b"].Value != 9 {
		t.Fatalf("duplicate-name rule samples collapsed or changed: %+v", byRuleID)
	}
	for _, sample := range byRuleID {
		if sample.Labels["rule_name"] != "duplicate" {
			t.Fatalf("rule_name label changed: %+v", sample.Labels)
		}
		if _, err := telemetry.NormalizeHubMetricSample(sample); err != nil {
			t.Fatalf("bridge emitted invalid hub sample: %v", err)
		}
	}
}
