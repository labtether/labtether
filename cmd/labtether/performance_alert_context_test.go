package main

import (
	"context"
	"time"

	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

// Keep the existing query-count regressions attached to the evaluator's real
// context entry points. The underlying Memory/PG owner still performs the read.
func (c *countingTelemetryStore) MetricSeriesBatchContext(ctx context.Context, ids []string, metric string, start, end time.Time, step time.Duration, maxRawPoints int) (map[string]telemetry.Series, error) {
	c.metricSeriesBatchCalls++
	return c.TelemetryStore.(persistence.TelemetryQueryBatchStore).MetricSeriesBatchContext(ctx, ids, metric, start, end, step, maxRawPoints)
}
func (c *countingTelemetryStore) HasTelemetrySamplesContext(ctx context.Context, ids []string, start, end time.Time) (map[string]bool, error) {
	c.hasSamplesCalls++
	return c.TelemetryStore.(persistence.AlertTelemetryActivityContextStore).HasTelemetrySamplesContext(ctx, ids, start, end)
}
func (c *countingTelemetryStore) AssetsWithMetricSamplesContext(ctx context.Context, ids []string, metric string, start, end time.Time) (map[string]bool, error) {
	c.hasSamplesCalls++
	return c.TelemetryStore.(persistence.AlertTelemetryActivityContextStore).AssetsWithMetricSamplesContext(ctx, ids, metric, start, end)
}
func (c *countingLogStore) QueryEventsContext(ctx context.Context, req logs.QueryRequest) ([]logs.Event, error) {
	c.queryEventsCalls++
	captured := req
	captured.FieldKeys = append([]string(nil), req.FieldKeys...)
	captured.GroupAssetIDs = append([]string(nil), req.GroupAssetIDs...)
	c.lastQueryEventsReq = captured
	return c.LogStore.(persistence.AlertLogContextStore).QueryEventsContext(ctx, req)
}
