package alerting

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
	"math"
	"strconv"
	"strings"
	"time"
)

type alertMetricSeriesStore interface {
	SeriesMetric(assetID, metric string, start, end time.Time, step time.Duration) (telemetry.Series, error)
}

type alertTelemetryActivityStore interface {
	HasTelemetrySamples(assetIDs []string, start, end time.Time) (map[string]bool, error)
}

func (d *Deps) EvaluateHeartbeatStaleWithPrefetch(
	rule alerts.Rule,
	prefetchedAssets []assets.Asset,
	prefetchedCapabilities map[string][]string,
) (bool, error) {
	windowSec := rule.WindowSeconds
	if windowSec <= 0 {
		windowSec = alerts.DefaultWindowSeconds
	}
	threshold := time.Now().UTC().Add(-alerts.DurationFromSeconds(windowSec, time.Duration(alerts.DefaultWindowSeconds)*time.Second))

	targetAssets, err := d.resolveRuleTargetAssetsWithCapabilities(rule, prefetchedAssets, prefetchedCapabilities, true)
	if err != nil {
		return false, err
	}

	for _, targetAsset := range targetAssets {
		if targetAsset.LastSeenAt.Before(threshold) && targetAsset.Status != "offline" {
			return true, nil
		}
	}
	return false, nil
}

func (d *Deps) EvaluateMetricThreshold(_ context.Context, rule alerts.Rule) (bool, error) {
	return d.EvaluateMetricThresholdWithPrefetch(rule, nil, nil)
}

func (d *Deps) EvaluateMetricThresholdWithPrefetch(
	rule alerts.Rule,
	prefetchedAssets []assets.Asset,
	prefetchedCapabilities map[string][]string,
) (bool, error) {
	if d.TelemetryStore == nil {
		return false, nil
	}

	windowSec := rule.WindowSeconds
	if windowSec <= 0 {
		windowSec = alerts.DefaultWindowSeconds
	}
	now := time.Now().UTC()
	windowStart := now.Add(-alerts.DurationFromSeconds(windowSec, time.Duration(alerts.DefaultWindowSeconds)*time.Second))

	threshold, _ := toFloat64(rule.Condition["threshold"])
	operator, _ := rule.Condition["operator"].(string)
	metric, _ := rule.Condition["metric"].(string)
	if operator == "" {
		operator = ">"
	}

	targetAssets, err := d.resolveRuleTargetAssetsWithCapabilities(rule, prefetchedAssets, prefetchedCapabilities, true)
	if err != nil {
		return false, err
	}
	assetIDs := uniqueAlertAssetIDs(targetAssets)
	if metric != "" {
		if batchStore, ok := d.TelemetryStore.(persistence.TelemetryAlertBatchStore); ok && len(assetIDs) > 0 {
			batchedSeries, err := batchStore.MetricSeriesBatch(assetIDs, metric, windowStart, now, 0)
			if err != nil {
				return false, fmt.Errorf("series batch query for %s: %w", metric, err)
			}
			agg, _ := rule.Condition["aggregate"].(string)
			for _, assetID := range assetIDs {
				series := batchedSeries[assetID]
				if len(series.Points) == 0 {
					continue
				}
				value := aggregatePoints(series.Points, agg)
				if compareThreshold(value, operator, threshold) {
					return true, nil
				}
			}
			return false, nil
		}
	}
	for _, targetAsset := range targetAssets {
		assetID := strings.TrimSpace(targetAsset.ID)
		if assetID == "" {
			continue
		}
		if metric != "" {
			if metricStore, ok := d.TelemetryStore.(alertMetricSeriesStore); ok {
				sr, err := metricStore.SeriesMetric(assetID, metric, windowStart, now, 0)
				if err != nil {
					return false, fmt.Errorf("series query for %s/%s: %w", assetID, metric, err)
				}
				if len(sr.Points) == 0 {
					continue
				}
				agg, _ := rule.Condition["aggregate"].(string)
				value := aggregatePoints(sr.Points, agg)
				if compareThreshold(value, operator, threshold) {
					return true, nil
				}
				continue
			}
		}

		series, err := d.TelemetryStore.Series(assetID, windowStart, now, 0)
		if err != nil {
			return false, fmt.Errorf("series query for %s: %w", assetID, err)
		}
		for _, sr := range series {
			if metric != "" && sr.Metric != metric {
				continue
			}
			if len(sr.Points) == 0 {
				continue
			}
			agg, _ := rule.Condition["aggregate"].(string)
			value := aggregatePoints(sr.Points, agg)
			if compareThreshold(value, operator, threshold) {
				return true, nil
			}
		}
	}

	return false, nil
}

func (d *Deps) EvaluateMetricDeadmanWithPrefetch(
	rule alerts.Rule,
	prefetchedAssets []assets.Asset,
	prefetchedCapabilities map[string][]string,
) (bool, error) {
	if d.TelemetryStore == nil {
		return false, nil
	}

	windowSec := rule.WindowSeconds
	if windowSec <= 0 {
		windowSec = alerts.DefaultWindowSeconds
	}
	now := time.Now().UTC()
	windowStart := now.Add(-alerts.DurationFromSeconds(windowSec, time.Duration(alerts.DefaultWindowSeconds)*time.Second))

	targetAssets, err := d.resolveRuleTargetAssetsWithCapabilities(rule, prefetchedAssets, prefetchedCapabilities, true)
	if err != nil {
		return false, err
	}
	assetIDs := uniqueAlertAssetIDs(targetAssets)
	if activityStore, ok := d.TelemetryStore.(persistence.TelemetryAlertBatchStore); ok {
		hasSamples, err := activityStore.AssetsWithSamples(assetIDs, windowStart, now)
		if err != nil {
			return false, fmt.Errorf("deadman activity query: %w", err)
		}
		for _, assetID := range assetIDs {
			if !hasSamples[assetID] {
				return true, nil
			}
		}
		return false, nil
	} else if activityStore, ok := d.TelemetryStore.(alertTelemetryActivityStore); ok {
		hasSamples, err := activityStore.HasTelemetrySamples(assetIDs, windowStart, now)
		if err != nil {
			return false, fmt.Errorf("deadman activity query: %w", err)
		}
		for _, assetID := range assetIDs {
			if !hasSamples[assetID] {
				return true, nil
			}
		}
		return false, nil
	}
	for _, targetAsset := range targetAssets {
		assetID := strings.TrimSpace(targetAsset.ID)
		if assetID == "" {
			continue
		}
		series, err := d.TelemetryStore.Series(assetID, windowStart, now, 0)
		if err != nil {
			return false, fmt.Errorf("deadman series query for %s: %w", assetID, err)
		}

		// If no data points at all within window, fire
		hasData := false
		for _, sr := range series {
			if len(sr.Points) > 0 {
				hasData = true
				break
			}
		}
		if !hasData {
			return true, nil
		}
	}

	return false, nil
}

func (d *Deps) evaluateSyntheticCheck(_ context.Context, rule alerts.Rule) (bool, error) {
	if d.SyntheticStore == nil {
		return false, nil
	}

	checkID, _ := rule.Condition["check_id"].(string)
	if checkID == "" {
		return false, nil
	}

	consecutiveFailures := 3
	if cf, ok := rule.Condition["consecutive_failures"]; ok {
		if v, ok := toFloat64(cf); ok && v > 0 {
			consecutiveFailures = int(v)
		}
	}

	results, err := d.SyntheticStore.ListSyntheticResults(checkID, consecutiveFailures)
	if err != nil {
		return false, fmt.Errorf("synthetic results query for check %s: %w", checkID, err)
	}

	if len(results) < consecutiveFailures {
		return false, nil
	}

	// Check if the most recent N results are all failures
	for _, r := range results {
		if r.Status == "ok" {
			return false, nil
		}
	}

	return true, nil
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return finiteFloat64(val)
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return 0, false
		}
		return finiteFloat64(f)
	default:
		return 0, false
	}
}

func finiteFloat64(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

func conditionString(condition map[string]any, key string) string {
	if condition == nil {
		return ""
	}
	if value, ok := condition[key]; ok {
		if asString, ok := value.(string); ok {
			return asString
		}
	}
	return ""
}

func conditionStringMap(raw any) map[string]string {
	switch typed := raw.(type) {
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, value := range typed {
			normalizedKey := strings.TrimSpace(key)
			normalizedValue := strings.TrimSpace(value)
			if normalizedKey == "" || normalizedValue == "" {
				continue
			}
			out[normalizedKey] = normalizedValue
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(typed))
		for key, value := range typed {
			normalizedKey := strings.TrimSpace(key)
			if normalizedKey == "" {
				continue
			}
			switch converted := value.(type) {
			case string:
				normalizedValue := strings.TrimSpace(converted)
				if normalizedValue == "" {
					continue
				}
				out[normalizedKey] = normalizedValue
			case fmt.Stringer:
				normalizedValue := strings.TrimSpace(converted.String())
				if normalizedValue == "" {
					continue
				}
				out[normalizedKey] = normalizedValue
			default:
				continue
			}
		}
		return out
	default:
		return map[string]string{}
	}
}

func uniqueAlertAssetIDs(targetAssets []assets.Asset) []string {
	out := make([]string, 0, len(targetAssets))
	seen := make(map[string]struct{}, len(targetAssets))
	for _, targetAsset := range targetAssets {
		assetID := strings.TrimSpace(targetAsset.ID)
		if assetID == "" {
			continue
		}
		if _, ok := seen[assetID]; ok {
			continue
		}
		seen[assetID] = struct{}{}
		out = append(out, assetID)
	}
	return out
}

func aggregatePoints(points []telemetry.Point, agg string) float64 {
	if len(points) == 0 {
		return 0
	}
	switch agg {
	case "max":
		max := points[0].Value
		for _, p := range points[1:] {
			if p.Value > max {
				max = p.Value
			}
		}
		return max
	case "min":
		min := points[0].Value
		for _, p := range points[1:] {
			if p.Value < min {
				min = p.Value
			}
		}
		return min
	case "sum":
		sum := 0.0
		for _, p := range points {
			sum += p.Value
		}
		return sum
	default: // avg
		sum := 0.0
		for _, p := range points {
			sum += p.Value
		}
		return sum / float64(len(points))
	}
}

func compareThreshold(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	default:
		return value > threshold
	}
}
