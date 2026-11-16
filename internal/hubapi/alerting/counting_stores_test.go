package alerting_test

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/shared"
	statusaggpkg "github.com/labtether/labtether/internal/hubapi/statusagg"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
	"time"
)

// countingTelemetryStore and countingLogStore are copies of cmd's, with every
// method (performance_hotspots_test.go, performance_log_aggregation_test.go,
// performance_alert_context_test.go), which cmd's performance tests still
// use. They count the reads of the stores they wrap.

type countingTelemetryStore struct {
	persistence.TelemetryStore
	snapshotCalls          int
	snapshotManyCalls      int
	seriesCalls            int
	seriesMetricCalls      int
	metricSeriesBatchCalls int
	hasSamplesCalls        int
}

func (c *countingTelemetryStore) Snapshot(assetID string, at time.Time) (telemetry.Snapshot, error) {
	c.snapshotCalls++
	return c.TelemetryStore.Snapshot(assetID, at)
}

func (c *countingTelemetryStore) SnapshotMany(assetIDs []string, at time.Time) (map[string]telemetry.Snapshot, error) {
	c.snapshotManyCalls++
	if store, ok := c.TelemetryStore.(persistence.TelemetrySnapshotBatchStore); ok {
		return store.SnapshotMany(assetIDs, at)
	}

	out := make(map[string]telemetry.Snapshot, len(assetIDs))
	for _, assetID := range assetIDs {
		snapshot, err := c.TelemetryStore.Snapshot(assetID, at)
		if err != nil {
			return nil, err
		}
		out[assetID] = snapshot
	}
	return out, nil
}

func (c *countingTelemetryStore) Series(assetID string, start, end time.Time, step time.Duration) ([]telemetry.Series, error) {
	c.seriesCalls++
	return c.TelemetryStore.Series(assetID, start, end, step)
}

func (c *countingTelemetryStore) SeriesMetric(assetID, metric string, start, end time.Time, step time.Duration) (telemetry.Series, error) {
	c.seriesMetricCalls++
	if store, ok := c.TelemetryStore.(interface {
		SeriesMetric(assetID, metric string, start, end time.Time, step time.Duration) (telemetry.Series, error)
	}); ok {
		return store.SeriesMetric(assetID, metric, start, end, step)
	}
	return telemetry.Series{}, nil
}

func (c *countingTelemetryStore) MetricSeriesBatch(assetIDs []string, metric string, start, end time.Time, step time.Duration) (map[string]telemetry.Series, error) {
	c.metricSeriesBatchCalls++
	if store, ok := c.TelemetryStore.(persistence.TelemetryAlertBatchStore); ok {
		return store.MetricSeriesBatch(assetIDs, metric, start, end, step)
	}
	return map[string]telemetry.Series{}, nil
}

func (c *countingTelemetryStore) HasTelemetrySamples(assetIDs []string, start, end time.Time) (map[string]bool, error) {
	c.hasSamplesCalls++
	if store, ok := c.TelemetryStore.(interface {
		HasTelemetrySamples(assetIDs []string, start, end time.Time) (map[string]bool, error)
	}); ok {
		return store.HasTelemetrySamples(assetIDs, start, end)
	}
	return map[string]bool{}, nil
}

func (c *countingTelemetryStore) AssetsWithSamples(assetIDs []string, start, end time.Time) (map[string]bool, error) {
	c.hasSamplesCalls++
	if store, ok := c.TelemetryStore.(persistence.TelemetryAlertBatchStore); ok {
		return store.AssetsWithSamples(assetIDs, start, end)
	}
	return map[string]bool{}, nil
}

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

type countingLogStore struct {
	persistence.LogStore
	queryEventsCalls     int
	lastQueryEventsReq   logs.QueryRequest
	querySourcesCalls    int
	lastSourceSummaryReq logs.SourceSummaryRequest
	groupSeverityCalls   int
	queryDeadLetterCalls int
	countDeadLetterCalls int
	listSourcesSinceCall int
	listSourcesCalls     int
	logWatermarkCalls    int
}

func (c *countingLogStore) QueryEvents(req logs.QueryRequest) ([]logs.Event, error) {
	c.queryEventsCalls++
	captured := req
	captured.FieldKeys = append([]string(nil), req.FieldKeys...)
	captured.GroupAssetIDs = append([]string(nil), req.GroupAssetIDs...)
	c.lastQueryEventsReq = captured
	return c.LogStore.QueryEvents(req)
}

func (c *countingLogStore) QueryDeadLetterEvents(from, to time.Time, limit int) ([]logs.DeadLetterEvent, error) {
	c.queryDeadLetterCalls++
	if store, ok := c.LogStore.(persistence.DeadLetterLogStore); ok {
		return store.QueryDeadLetterEvents(from, to, limit)
	}

	events, err := c.LogStore.QueryEvents(logs.QueryRequest{
		Source: "dead_letter",
		Level:  "error",
		From:   from,
		To:     to,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	out := make([]logs.DeadLetterEvent, 0, len(events))
	for _, event := range events {
		mapped := shared.MapLogEventToDeadLetter(event)
		out = append(out, logs.DeadLetterEvent{
			ID:         mapped.ID,
			Component:  mapped.Component,
			Subject:    mapped.Subject,
			Deliveries: mapped.Deliveries,
			Error:      mapped.Error,
			PayloadB64: mapped.PayloadB64,
			CreatedAt:  mapped.CreatedAt,
		})
	}
	return out, nil
}

func (c *countingLogStore) CountDeadLetterEvents(from, to time.Time) (int, error) {
	c.countDeadLetterCalls++
	if store, ok := c.LogStore.(persistence.DeadLetterLogCountStore); ok {
		return store.CountDeadLetterEvents(from, to)
	}
	events, err := c.QueryDeadLetterEvents(from, to, 1000)
	if err != nil {
		return 0, err
	}
	return len(events), nil
}

func (c *countingLogStore) ListSourcesSince(limit int, from time.Time) ([]logs.SourceSummary, error) {
	c.listSourcesSinceCall++
	if store, ok := c.LogStore.(statusaggpkg.RecentSourceLister); ok {
		return store.ListSourcesSince(limit, from)
	}

	events, err := c.LogStore.QueryEvents(logs.QueryRequest{
		From:  from,
		To:    time.Now().UTC(),
		Limit: 1000,
	})
	if err != nil {
		return nil, err
	}
	return statusaggpkg.AggregateLogSources(events, limit), nil
}

func (c *countingLogStore) ListSources(limit int) ([]logs.SourceSummary, error) {
	c.listSourcesCalls++
	return c.LogStore.ListSources(limit)
}

func (c *countingLogStore) QuerySourceSummaries(req logs.SourceSummaryRequest) ([]logs.SourceSummary, error) {
	c.querySourcesCalls++
	captured := req
	captured.GroupAssetIDs = append([]string(nil), req.GroupAssetIDs...)
	c.lastSourceSummaryReq = captured
	if store, ok := c.LogStore.(persistence.LogSourceSummaryStore); ok {
		return store.QuerySourceSummaries(req)
	}

	events, err := c.LogStore.QueryEvents(logs.QueryRequest{
		From:          req.From,
		To:            req.To,
		Limit:         1000,
		GroupID:       req.GroupID,
		GroupAssetIDs: req.GroupAssetIDs,
		FieldKeys:     []string{"group_id"},
	})
	if err != nil {
		return nil, err
	}

	counts := make(map[string]logs.SourceSummary, len(events))
	for _, event := range events {
		entry := counts[event.Source]
		entry.Source = event.Source
		entry.Count++
		if event.Timestamp.After(entry.LastSeenAt) {
			entry.LastSeenAt = event.Timestamp
		}
		counts[event.Source] = entry
	}

	out := make([]logs.SourceSummary, 0, len(counts))
	for _, entry := range counts {
		out = append(out, entry)
	}
	return out, nil
}

func (c *countingLogStore) QueryGroupSeverityCounts(req logs.GroupSeverityCountRequest) ([]logs.GroupSeverityCount, error) {
	c.groupSeverityCalls++
	if store, ok := c.LogStore.(persistence.LogGroupSeverityCountStore); ok {
		return store.QueryGroupSeverityCounts(req)
	}
	return nil, nil
}

func (c *countingLogStore) LogEventsWatermark() (time.Time, error) {
	c.logWatermarkCalls++
	if store, ok := c.LogStore.(statusaggpkg.LogWatermarkReader); ok {
		return store.LogEventsWatermark()
	}
	return time.Unix(0, 0).UTC(), nil
}

func (c *countingLogStore) QueryEventsContext(ctx context.Context, req logs.QueryRequest) ([]logs.Event, error) {
	c.queryEventsCalls++
	captured := req
	captured.FieldKeys = append([]string(nil), req.FieldKeys...)
	captured.GroupAssetIDs = append([]string(nil), req.GroupAssetIDs...)
	c.lastQueryEventsReq = captured
	return c.LogStore.(persistence.AlertLogContextStore).QueryEventsContext(ctx, req)
}
