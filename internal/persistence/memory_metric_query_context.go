package persistence

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// Only matching points are copied, under one cancellable read lock. Detect the
// first excess row before appending it; never return an incomplete aggregate.
func (m *MemoryTelemetryStore) MetricSeriesBatchContext(ctx context.Context, assetIDs []string, metric string, start, end time.Time, step time.Duration, maxRawPoints int) (map[string]telemetry.Series, error) {
	if ctx == nil || maxRawPoints <= 0 {
		return nil, fmt.Errorf("telemetry query needs context and positive row limit")
	}
	return m.metricSeriesBatchContext(ctx, assetIDs, metric, start, end, step, maxRawPoints)
}

func (m *MemoryTelemetryStore) metricSeriesBatchContext(ctx context.Context, assetIDs []string, metric string, start, end time.Time, step time.Duration, maxRawPoints int) (map[string]telemetry.Series, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ids := normalizeLogAssetIDs(assetIDs)
	metric = strings.TrimSpace(metric)
	pointsByID := make(map[string][]telemetry.Point, len(ids))
	if err := lockAlertEvaluation(ctx, &m.mu, true); err != nil {
		return nil, err
	}
	err := func() error {
		defer m.mu.RUnlock()
		count := 0
		for _, id := range ids {
			for _, sample := range m.samples[id] {
				if err := ctx.Err(); err != nil {
					return err
				}
				if sample.Metric != metric || sample.CollectedAt.Before(start) || sample.CollectedAt.After(end) {
					continue
				}
				if maxRawPoints > 0 && count == maxRawPoints {
					return ErrTelemetryQueryLimitExceeded
				}
				count++
				pointsByID[id] = append(pointsByID[id], telemetry.Point{TS: sample.CollectedAt.Unix(), Value: sample.Value})
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	definition, _ := telemetryCanonicalMetricDefinition(metric)
	out := make(map[string]telemetry.Series, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		points := pointsByID[id]
		sort.Slice(points, func(i, j int) bool { return points[i].TS < points[j].TS })
		points = telemetry.BucketAveragePoints(points, step)
		out[id] = telemetry.Series{Metric: metric, Unit: definition.Unit, Points: points, Current: telemetry.LastPointValue(points)}
	}
	return out, ctx.Err()
}
