package alerting

import (
	"context"
	"fmt"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

// One condition may read at most this many raw points, across all targets and
// metrics. An excess is an evaluation error, never a partial false/recovery.
const alertMetricRawPointLimit = 100000

func (d *Deps) evaluateHeartbeatStaleOutcome(ctx context.Context, rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string) (alertConditionOutcome, error) {
	outcome := newAlertConditionOutcome()
	gap, _, err := alertStalenessCondition(rule)
	if err != nil {
		return outcome, err
	}
	threshold := time.Now().UTC().Add(-gap)
	targets, err := d.resolveRuleTargetAssetsContext(ctx, rule, prefetchedAssets, prefetchedCapabilities, true)
	if err != nil {
		return outcome, err
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return alertConditionOutcome{}, err
		}
		if target.LastSeenAt.Before(threshold) {
			outcome.witnessAsset(target)
		}
	}
	return outcome.result()
}

func (d *Deps) EvaluateMetricThreshold(ctx context.Context, rule alerts.Rule) (bool, error) {
	outcome, err := d.evaluateMetricThresholdOutcome(ctx, rule, nil, nil)
	return outcome.Triggered, err
}

func (d *Deps) evaluateMetricThresholdOutcome(ctx context.Context, rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string) (alertConditionOutcome, error) {
	outcome := newAlertConditionOutcome()
	threshold, err := metricThresholdValue(rule.Condition)
	if err != nil {
		return outcome, err
	}
	if d.TelemetryStore == nil {
		return outcome.result()
	}
	store, ok := d.TelemetryStore.(persistence.TelemetryQueryBatchStore)
	if !ok {
		return outcome, fmt.Errorf("bounded alert metric query unavailable")
	}
	now := time.Now().UTC()
	start := now.Add(-alerts.DurationFromSeconds(rule.WindowSeconds, time.Duration(alerts.DefaultWindowSeconds)*time.Second))
	operator, _ := rule.Condition["operator"].(string)
	if operator == "" {
		operator = ">"
	}
	metric, _ := rule.Condition["metric"].(string)
	metrics := []string{metric}
	// Preserve legacy empty-metric rules: they check every canonical metric.
	if metric == "" {
		metrics = nil
		for _, definition := range telemetry.CanonicalMetrics() {
			metrics = append(metrics, definition.Metric)
		}
	}
	targets, err := d.resolveRuleTargetAssetsContext(ctx, rule, prefetchedAssets, prefetchedCapabilities, true)
	if err != nil {
		return outcome, err
	}
	ids := uniqueAlertAssetIDs(targets)
	remaining := alertMetricRawPointLimit
	aggregate, _ := rule.Condition["aggregate"].(string)
	for _, name := range metrics {
		if err := ctx.Err(); err != nil {
			return alertConditionOutcome{}, err
		}
		// A zero remaining budget still permits an empty result. One requested row
		// distinguishes that from overflow without admitting a partial aggregate.
		limit := remaining
		if limit == 0 {
			limit = 1
		}
		series, err := store.MetricSeriesBatchContext(ctx, ids, name, start, now, 0, limit)
		if err != nil {
			return alertConditionOutcome{}, fmt.Errorf("series batch query for %s: %w", name, err)
		}
		for _, target := range targets {
			if err := ctx.Err(); err != nil {
				return alertConditionOutcome{}, err
			}
			points := series[target.ID].Points
			if len(points) > remaining {
				return alertConditionOutcome{}, persistence.ErrTelemetryQueryLimitExceeded
			}
			remaining -= len(points)
			if len(points) > 0 && compareThreshold(aggregatePoints(points, aggregate), operator, threshold) {
				outcome.witnessAsset(target)
			}
		}
	}
	return outcome.result()
}

func (d *Deps) evaluateMetricDeadmanOutcome(ctx context.Context, rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string) (alertConditionOutcome, error) {
	outcome := newAlertConditionOutcome()
	gap, metric, err := alertStalenessCondition(rule)
	if err != nil {
		return outcome, err
	}
	if d.TelemetryStore == nil {
		return outcome.result()
	}
	store, ok := d.TelemetryStore.(persistence.AlertTelemetryActivityContextStore)
	if !ok {
		return outcome, fmt.Errorf("context-aware deadman activity lookup unavailable")
	}
	targets, err := d.resolveRuleTargetAssetsContext(ctx, rule, prefetchedAssets, prefetchedCapabilities, true)
	if err != nil {
		return outcome, err
	}
	ids := uniqueAlertAssetIDs(targets)
	now := time.Now().UTC()
	var present map[string]bool
	if metric != "" {
		present, err = store.AssetsWithMetricSamplesContext(ctx, ids, metric, now.Add(-gap), now)
	} else {
		present, err = store.HasTelemetrySamplesContext(ctx, ids, now.Add(-gap), now)
	}
	if err != nil {
		return alertConditionOutcome{}, fmt.Errorf("deadman activity query: %w", err)
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return alertConditionOutcome{}, err
		}
		if !present[target.ID] {
			outcome.witnessAsset(target)
		}
	}
	return outcome.result()
}

func (d *Deps) EvaluateHeartbeatStaleWithPrefetch(rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outcome, err := d.evaluateHeartbeatStaleOutcome(ctx, rule, prefetchedAssets, prefetchedCapabilities)
	return outcome.Triggered, err
}
func (d *Deps) EvaluateMetricThresholdWithPrefetch(rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outcome, err := d.evaluateMetricThresholdOutcome(ctx, rule, prefetchedAssets, prefetchedCapabilities)
	return outcome.Triggered, err
}
func (d *Deps) EvaluateMetricDeadmanWithPrefetch(rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outcome, err := d.evaluateMetricDeadmanOutcome(ctx, rule, prefetchedAssets, prefetchedCapabilities)
	return outcome.Triggered, err
}
