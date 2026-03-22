package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// AlertStateSource provides alert firing and rules count metrics.
type AlertStateSource interface {
	// AllAlertStateMetrics returns hub-level alert state entries.
	AllAlertStateMetrics() []AlertStateEntry
}

// AlertRuleEvalSource is an optional interface that sources can implement
// to provide per-rule alert evaluation duration metrics.
type AlertRuleEvalSource interface {
	// AllAlertRuleEvalMetrics returns the latest evaluation duration per active rule.
	AllAlertRuleEvalMetrics() []AlertRuleEvalEntry
}

// AlertStateEntry holds hub-level alert firing and rules counts.
// This is a hub-level metric (not per-asset); AssetID is fixed to "hub-alerts".
type AlertStateEntry struct {
	FiringCount float64
	RulesCount  float64
	Labels      map[string]string // empty for hub-level
}

// AlertRuleEvalEntry holds per-rule evaluation timing.
type AlertRuleEvalEntry struct {
	RuleName   string
	DurationMS float64
}

// AlertStateBridge is a MetricsBridge that reads alert state from an
// AlertStateSource and converts it to MetricSample objects.
type AlertStateBridge struct {
	source AlertStateSource
}

// NewAlertStateBridge creates an AlertStateBridge backed by the given source.
func NewAlertStateBridge(source AlertStateSource) *AlertStateBridge {
	return &AlertStateBridge{source: source}
}

// Name returns the bridge identifier.
func (b *AlertStateBridge) Name() string { return "alert-state" }

// Interval returns how often this bridge should be collected.
func (b *AlertStateBridge) Interval() time.Duration { return 60 * time.Second }

// Collect iterates all alert state entries from the source and produces 2
// MetricSamples per entry: alerts_firing and alerts_rules, keyed to the
// synthetic asset ID "hub-alerts". If the source also implements
// AlertRuleEvalSource, per-rule alert_evaluation_duration_ms samples are
// appended.
func (b *AlertStateBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllAlertStateMetrics()
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	out := make([]telemetry.MetricSample, 0, len(entries)*2)

	for _, e := range entries {
		labels := e.Labels

		out = append(out,
			telemetry.MetricSample{
				AssetID:     "hub-alerts",
				Metric:      telemetry.MetricAlertsFiring,
				Unit:        "count",
				Value:       e.FiringCount,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     "hub-alerts",
				Metric:      telemetry.MetricAlertsRules,
				Unit:        "count",
				Value:       e.RulesCount,
				CollectedAt: now,
				Labels:      labels,
			},
		)
	}

	// Per-rule evaluation duration (optional source interface).
	if evalSource, ok := b.source.(AlertRuleEvalSource); ok {
		for _, re := range evalSource.AllAlertRuleEvalMetrics() {
			out = append(out, telemetry.MetricSample{
				AssetID:     "hub-alerts",
				Metric:      telemetry.MetricAlertEvaluationDurationMs,
				Unit:        "ms",
				Value:       re.DurationMS,
				CollectedAt: now,
				Labels:      map[string]string{"rule_name": re.RuleName},
			})
		}
	}

	return out
}
