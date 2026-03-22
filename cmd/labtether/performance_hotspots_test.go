package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/updates"
)

type canonicalStoreWithWatermarkCounter struct {
	persistence.CanonicalModelStore
	watermarkCalls int
	providerCalls  int
}

func (c *canonicalStoreWithWatermarkCounter) CanonicalStatusWatermark() (time.Time, error) {
	c.watermarkCalls++
	return time.Unix(1, 0).UTC(), nil
}

func (c *canonicalStoreWithWatermarkCounter) ListProviderInstances(limit int) ([]model.ProviderInstance, error) {
	c.providerCalls++
	return c.CanonicalModelStore.ListProviderInstances(limit)
}

type countingAlertInstanceStore struct {
	persistence.AlertInstanceStore
	listSilenceCalls int
}

func (c *countingAlertInstanceStore) ListAlertSilences(limit int, activeOnly bool) ([]alerts.AlertSilence, error) {
	c.listSilenceCalls++
	return c.AlertInstanceStore.ListAlertSilences(limit, activeOnly)
}

type countingRuntimeSettingsStore struct {
	persistence.RuntimeSettingsStore
	listOverridesCalls int
}

func (c *countingRuntimeSettingsStore) ListRuntimeSettingOverrides() (map[string]string, error) {
	c.listOverridesCalls++
	return c.RuntimeSettingsStore.ListRuntimeSettingOverrides()
}

type countingActionStore struct {
	persistence.ActionStore
	listActionRunsCalls int
}

func (c *countingActionStore) ListActionRuns(limit, offset int, runType, status string) ([]actions.Run, error) {
	c.listActionRunsCalls++
	return c.ActionStore.ListActionRuns(limit, offset, runType, status)
}

type countingUpdateStore struct {
	persistence.UpdateStore
	listUpdateRunsCalls int
}

func (c *countingUpdateStore) ListUpdateRuns(limit int, status string) ([]updates.Run, error) {
	c.listUpdateRunsCalls++
	return c.UpdateStore.ListUpdateRuns(limit, status)
}

type countingIncidentStore struct {
	persistence.IncidentStore
	listIncidentAlertLinksCalls int
	hasAutoIncidentCalls        int
}

func (c *countingIncidentStore) ListIncidentAlertLinks(incidentID string, limit int) ([]incidents.AlertLink, error) {
	c.listIncidentAlertLinksCalls++
	return c.IncidentStore.ListIncidentAlertLinks(incidentID, limit)
}

func (c *countingIncidentStore) HasAutoIncidentForAlertInstance(alertInstanceID string) (bool, error) {
	c.hasAutoIncidentCalls++
	if store, ok := c.IncidentStore.(interface {
		HasAutoIncidentForAlertInstance(alertInstanceID string) (bool, error)
	}); ok {
		return store.HasAutoIncidentForAlertInstance(alertInstanceID)
	}
	return false, nil
}

type countingLogStore struct {
	persistence.LogStore
	queryEventsCalls     int
	lastQueryEventsReq   logs.QueryRequest
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
		mapped := mapLogEventToDeadLetter(event)
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
	if store, ok := c.LogStore.(statusRecentSourceLister); ok {
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
	return statusAggregateLogSources(events, limit), nil
}

func (c *countingLogStore) ListSources(limit int) ([]logs.SourceSummary, error) {
	c.listSourcesCalls++
	return c.LogStore.ListSources(limit)
}

func (c *countingLogStore) LogEventsWatermark() (time.Time, error) {
	c.logWatermarkCalls++
	if store, ok := c.LogStore.(statusAggregateLogWatermarkReader); ok {
		return store.LogEventsWatermark()
	}
	return time.Unix(0, 0).UTC(), nil
}

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

func TestStatusAggregateSkipsFingerprintPrecomputeWithoutConditionalRequest(t *testing.T) {
	sut := newTestAPIServer(t)

	canonicalCounter := &canonicalStoreWithWatermarkCounter{CanonicalModelStore: sut.canonicalStore}
	sut.canonicalStore = canonicalCounter

	handler := sut.handleStatusAggregate(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/status/aggregate", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if canonicalCounter.watermarkCalls != 1 {
		t.Fatalf("expected one canonical watermark call for canonical cache key derivation, got %d", canonicalCounter.watermarkCalls)
	}
	if canonicalCounter.providerCalls != 1 {
		t.Fatalf("expected one canonical provider listing for initial aggregate build, got %d", canonicalCounter.providerCalls)
	}
}

func TestStatusAggregateConditionalRequestUsesPrecomputeAfterCacheWarmup(t *testing.T) {
	sut := newTestAPIServer(t)

	canonicalCounter := &canonicalStoreWithWatermarkCounter{CanonicalModelStore: sut.canonicalStore}
	sut.canonicalStore = canonicalCounter

	handler := sut.handleStatusAggregate(nil, nil)

	firstReq := httptest.NewRequest(http.MethodGet, "/status/aggregate", nil)
	firstRec := httptest.NewRecorder()
	handler(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected warmup 200, got %d", firstRec.Code)
	}
	etag := firstRec.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("expected warmup response to include ETag")
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/status/aggregate", nil)
	secondReq.Header.Set("If-None-Match", etag)
	secondRec := httptest.NewRecorder()
	handler(secondRec, secondReq)

	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for first conditional warmup, got %d", secondRec.Code)
	}
	secondETag := secondRec.Header().Get("ETag")
	if secondETag == "" {
		t.Fatalf("expected first conditional response to include ETag")
	}

	thirdReq := httptest.NewRequest(http.MethodGet, "/status/aggregate", nil)
	thirdReq.Header.Set("If-None-Match", secondETag)
	thirdRec := httptest.NewRecorder()
	handler(thirdRec, thirdReq)

	if thirdRec.Code != http.StatusNotModified {
		t.Fatalf("expected 304 for cached conditional request, got %d", thirdRec.Code)
	}
	if canonicalCounter.watermarkCalls != 2 {
		t.Fatalf("expected 2 canonical watermark calls (payload cache key only; fingerprint uses generation counter), got %d", canonicalCounter.watermarkCalls)
	}
	if canonicalCounter.providerCalls != 1 {
		t.Fatalf("expected canonical payload cache reuse across conditional requests, provider calls=%d", canonicalCounter.providerCalls)
	}
}

func TestStatusAggregateCanonicalPayloadCacheReusesCanonicalQueries(t *testing.T) {
	sut := newTestAPIServer(t)

	canonicalCounter := &canonicalStoreWithWatermarkCounter{CanonicalModelStore: sut.canonicalStore}
	sut.canonicalStore = canonicalCounter

	_ = sut.buildStatusAggregateResponse(context.Background(), "")
	_ = sut.buildStatusAggregateResponse(context.Background(), "")

	if canonicalCounter.watermarkCalls != 2 {
		t.Fatalf("expected one canonical watermark call per response build, got %d", canonicalCounter.watermarkCalls)
	}
	if canonicalCounter.providerCalls != 1 {
		t.Fatalf("expected canonical payload cache hit on second build, provider calls=%d", canonicalCounter.providerCalls)
	}
}

func TestStatusAggregateCanonicalPayloadCacheInvalidatesOnAssetSetChange(t *testing.T) {
	sut := newTestAPIServer(t)

	canonicalCounter := &canonicalStoreWithWatermarkCounter{CanonicalModelStore: sut.canonicalStore}
	sut.canonicalStore = canonicalCounter

	_ = sut.buildStatusAggregateResponse(context.Background(), "")
	seedAssetViaHeartbeat(t, sut, "canonical-cache-asset-1", "CANON")
	_ = sut.buildStatusAggregateResponse(context.Background(), "")

	if canonicalCounter.providerCalls != 2 {
		t.Fatalf("expected canonical payload to rebuild after asset-set change, provider calls=%d", canonicalCounter.providerCalls)
	}
}

func TestMetricThresholdAlertUsesBatchMetricSeriesQuery(t *testing.T) {
	sut := newTestAPIServer(t)

	telemetryCounter := &countingTelemetryStore{TelemetryStore: sut.telemetryStore}
	sut.telemetryStore = telemetryCounter

	seedAssetViaHeartbeat(t, sut, "batch-threshold-asset-1", "Batch Threshold One")
	seedAssetViaHeartbeat(t, sut, "batch-threshold-asset-2", "Batch Threshold Two")
	seedMetricSamples(t, sut, "batch-threshold-asset-1", telemetry.MetricCPUUsedPercent, 55)
	seedMetricSamples(t, sut, "batch-threshold-asset-2", telemetry.MetricCPUUsedPercent, 97)

	rule := alerts.Rule{
		ID:          "rule-batch-threshold",
		Name:        "Batch Threshold",
		Kind:        alerts.RuleKindMetricThreshold,
		Severity:    alerts.SeverityCritical,
		TargetScope: alerts.TargetScopeAsset,
		Condition: map[string]any{
			"metric":    telemetry.MetricCPUUsedPercent,
			"operator":  ">",
			"threshold": float64(90),
		},
		Targets: []alerts.RuleTarget{
			{AssetID: "batch-threshold-asset-1"},
			{AssetID: "batch-threshold-asset-2"},
		},
	}

	triggered, err := sut.evaluateMetricThresholdWithPrefetch(rule, nil, nil)
	if err != nil {
		t.Fatalf("evaluateMetricThresholdWithPrefetch() error = %v", err)
	}
	if !triggered {
		t.Fatalf("expected threshold alert to trigger")
	}
	if telemetryCounter.metricSeriesBatchCalls != 1 {
		t.Fatalf("expected one batch metric query, got %d", telemetryCounter.metricSeriesBatchCalls)
	}
	if telemetryCounter.seriesMetricCalls != 0 {
		t.Fatalf("expected per-asset metric queries to be skipped, got %d", telemetryCounter.seriesMetricCalls)
	}
	if telemetryCounter.seriesCalls != 0 {
		t.Fatalf("expected full series queries to be skipped, got %d", telemetryCounter.seriesCalls)
	}
}

func TestMetricDeadmanAlertUsesBatchActivityQuery(t *testing.T) {
	sut := newTestAPIServer(t)

	telemetryCounter := &countingTelemetryStore{TelemetryStore: sut.telemetryStore}
	sut.telemetryStore = telemetryCounter

	seedAssetViaHeartbeat(t, sut, "batch-deadman-asset-1", "Batch Deadman One")
	seedAssetViaHeartbeat(t, sut, "batch-deadman-asset-2", "Batch Deadman Two")
	seedMetricSamples(t, sut, "batch-deadman-asset-1", telemetry.MetricCPUUsedPercent, 40)

	rule := alerts.Rule{
		ID:          "rule-batch-deadman",
		Name:        "Batch Deadman",
		Kind:        alerts.RuleKindMetricDeadman,
		Severity:    alerts.SeverityHigh,
		TargetScope: alerts.TargetScopeAsset,
		Targets: []alerts.RuleTarget{
			{AssetID: "batch-deadman-asset-1"},
			{AssetID: "batch-deadman-asset-2"},
		},
	}

	triggered, err := sut.evaluateMetricDeadmanWithPrefetch(rule, nil, nil)
	if err != nil {
		t.Fatalf("evaluateMetricDeadmanWithPrefetch() error = %v", err)
	}
	if !triggered {
		t.Fatalf("expected deadman alert to trigger for missing asset telemetry")
	}
	if telemetryCounter.hasSamplesCalls != 1 {
		t.Fatalf("expected one batch activity query, got %d", telemetryCounter.hasSamplesCalls)
	}
	if telemetryCounter.seriesCalls != 0 {
		t.Fatalf("expected full series deadman queries to be skipped, got %d", telemetryCounter.seriesCalls)
	}
}

func TestLogPatternAlertUsesSingleTargetedLogQuery(t *testing.T) {
	sut := newTestAPIServer(t)

	logCounter := &countingLogStore{LogStore: sut.logStore}
	sut.logStore = logCounter

	seedAssetViaHeartbeat(t, sut, "batch-log-asset-1", "Batch Log One")
	seedAssetViaHeartbeat(t, sut, "batch-log-asset-2", "Batch Log Two")
	now := time.Now().UTC()
	if err := sut.logStore.AppendEvent(logs.Event{
		ID:        "batch-log-event-1",
		AssetID:   "batch-log-asset-2",
		Source:    "agent",
		Level:     "error",
		Message:   "kernel panic happened",
		Timestamp: now,
	}); err != nil {
		t.Fatalf("append event failed: %v", err)
	}

	rule := alerts.Rule{
		ID:          "rule-batch-log",
		Name:        "Batch Log Pattern",
		Kind:        alerts.RuleKindLogPattern,
		Severity:    alerts.SeverityHigh,
		TargetScope: alerts.TargetScopeAsset,
		Condition: map[string]any{
			"pattern": "panic",
			"level":   "error",
		},
		Targets: []alerts.RuleTarget{
			{AssetID: "batch-log-asset-1"},
			{AssetID: "batch-log-asset-2"},
		},
	}

	triggered, err := sut.evaluateLogPatternWithPrefetch(rule, nil, nil)
	if err != nil {
		t.Fatalf("evaluateLogPatternWithPrefetch() error = %v", err)
	}
	if !triggered {
		t.Fatalf("expected log pattern alert to trigger")
	}
	if logCounter.queryEventsCalls != 1 {
		t.Fatalf("expected one targeted log query, got %d", logCounter.queryEventsCalls)
	}
	if len(logCounter.lastQueryEventsReq.GroupAssetIDs) != 2 {
		t.Fatalf("expected targeted asset filter to include both assets, got %#v", logCounter.lastQueryEventsReq.GroupAssetIDs)
	}
}

func TestStatusLoadDeadLettersCachesByWatermark(t *testing.T) {
	sut := newTestAPIServer(t)

	logCounter := &countingLogStore{LogStore: sut.logStore}
	sut.logStore = logCounter

	err := sut.logStore.AppendEvent(logs.Event{
		ID:      "perf-dead-letter-1",
		Source:  "dead_letter",
		Level:   "error",
		Message: "decode failure",
		Fields: map[string]string{
			"event_id":  "perf-dlq-1",
			"component": "worker.command.decode",
			"subject":   "terminal.commands.requested",
		},
		Timestamp: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to seed dead-letter event: %v", err)
	}

	snapshot := sut.statusLoadDeadLetters()
	second := sut.statusLoadDeadLetters()
	if logCounter.queryDeadLetterCalls != 1 {
		t.Fatalf("expected cached dead-letter fetch to reuse first projected query, got %d", logCounter.queryDeadLetterCalls)
	}
	if logCounter.countDeadLetterCalls != 1 {
		t.Fatalf("expected cached dead-letter fetch to reuse first count query, got %d", logCounter.countDeadLetterCalls)
	}
	if logCounter.queryEventsCalls != 0 {
		t.Fatalf("expected QueryEvents fallback to be skipped, got %d", logCounter.queryEventsCalls)
	}
	if len(snapshot.Events) != 1 {
		t.Fatalf("expected one listed dead-letter event, got %d", len(snapshot.Events))
	}
	if len(second.Events) != 1 {
		t.Fatalf("expected cached snapshot to preserve one listed dead-letter event, got %d", len(second.Events))
	}
	if snapshot.Total < 1 {
		t.Fatalf("expected total >= 1, got %d", snapshot.Total)
	}
}

func TestHandleDeadLettersUsesSingleQuery(t *testing.T) {
	sut := newTestAPIServer(t)

	logCounter := &countingLogStore{LogStore: sut.logStore}
	sut.logStore = logCounter

	err := sut.logStore.AppendEvent(logs.Event{
		ID:      "perf-dead-letter-api-1",
		Source:  "dead_letter",
		Level:   "error",
		Message: "network timeout",
		Fields: map[string]string{
			"event_id":  "perf-dlq-api-1",
			"component": "worker.command.result_publish",
			"subject":   "terminal.commands.completed",
		},
		Timestamp: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to seed dead-letter event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/queue/dead-letters?window=24h&limit=5", nil)
	rec := httptest.NewRecorder()
	sut.handleDeadLetters(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if logCounter.queryDeadLetterCalls != 1 {
		t.Fatalf("expected exactly one optimized dead-letter query for endpoint, got %d", logCounter.queryDeadLetterCalls)
	}
	if logCounter.countDeadLetterCalls != 1 {
		t.Fatalf("expected exactly one optimized dead-letter count query for endpoint, got %d", logCounter.countDeadLetterCalls)
	}
	if logCounter.queryEventsCalls != 0 {
		t.Fatalf("expected QueryEvents fallback to be skipped for endpoint, got %d", logCounter.queryEventsCalls)
	}
}

func TestStatusLogSourcesUsesWatermarkCache(t *testing.T) {
	sut := newTestAPIServer(t)

	logCounter := &countingLogStore{LogStore: sut.logStore}
	sut.logStore = logCounter

	if err := sut.logStore.AppendEvent(logs.Event{
		ID:        "perf-log-source-1",
		Source:    "agent",
		Level:     "info",
		Message:   "hello",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("failed to seed log event: %v", err)
	}

	assetSite := map[string]string{}
	first := sut.statusListLogSources("", assetSite, "test.status.aggregate")
	second := sut.statusListLogSources("", assetSite, "test.status.aggregate")

	if len(first) == 0 {
		t.Fatalf("expected first source listing to include data")
	}
	if len(second) == 0 {
		t.Fatalf("expected cached source listing to include data")
	}
	if logCounter.listSourcesSinceCall != 1 {
		t.Fatalf("expected one ListSourcesSince call across cached reads, got %d", logCounter.listSourcesSinceCall)
	}
}

func TestStatusLogSourcesSiteFilterUsesProjectedSiteField(t *testing.T) {
	sut := newTestAPIServer(t)

	logCounter := &countingLogStore{LogStore: sut.logStore}
	sut.logStore = logCounter

	now := time.Now().UTC()
	const assetID = "group-filter-asset-1"
	if err := sut.logStore.AppendEvent(logs.Event{
		ID:      "group-filter-log-1",
		AssetID: assetID,
		Source:  "agent",
		Level:   "info",
		Message: "group-scoped log",
		Fields: map[string]string{
			"group_id": "group-a",
		},
		Timestamp: now,
	}); err != nil {
		t.Fatalf("failed to seed log event: %v", err)
	}

	assetSite := map[string]string{
		assetID: "group-a",
	}
	_ = sut.statusListLogSources("group-a", assetSite, "test.status.aggregate.group-filter")

	if logCounter.queryEventsCalls != 1 {
		t.Fatalf("expected one QueryEvents call, got %d", logCounter.queryEventsCalls)
	}
	if logCounter.lastQueryEventsReq.ExcludeFields {
		t.Fatalf("expected group-filter source query to keep projected fields")
	}
	if len(logCounter.lastQueryEventsReq.FieldKeys) != 1 || logCounter.lastQueryEventsReq.FieldKeys[0] != "group_id" {
		t.Fatalf("expected group-filter source query to project only group_id, got %#v", logCounter.lastQueryEventsReq.FieldKeys)
	}
	if logCounter.lastQueryEventsReq.GroupID != "group-a" {
		t.Fatalf("expected group-filter source query to include group id, got %q", logCounter.lastQueryEventsReq.GroupID)
	}
	if len(logCounter.lastQueryEventsReq.GroupAssetIDs) != 1 || logCounter.lastQueryEventsReq.GroupAssetIDs[0] != assetID {
		t.Fatalf("expected group-filter source query to include fallback asset IDs, got %#v", logCounter.lastQueryEventsReq.GroupAssetIDs)
	}
}

func TestHandleLogSourcesSiteFilterUsesProjectedSiteField(t *testing.T) {
	sut := newTestAPIServer(t)

	logCounter := &countingLogStore{LogStore: sut.logStore}
	sut.logStore = logCounter

	groupEntry, err := sut.groupStore.CreateGroup(groups.CreateRequest{
		Name: "Projection Group",
		Slug: "prj",
	})
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	const assetID = "logs-group-projection-asset-1"
	seedAssetViaHeartbeatWithSite(t, sut, assetID, groupEntry.ID)
	if err := sut.logStore.AppendEvent(logs.Event{
		ID:        "logs-group-projection-event-1",
		AssetID:   assetID,
		Source:    "agent",
		Level:     "info",
		Message:   "group filtered source event",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("failed to seed log event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/sources?group_id="+groupEntry.ID+"&limit=10", nil)
	rec := httptest.NewRecorder()
	sut.handleLogSources(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if logCounter.queryEventsCalls != 1 {
		t.Fatalf("expected one QueryEvents call, got %d", logCounter.queryEventsCalls)
	}
	if len(logCounter.lastQueryEventsReq.FieldKeys) != 1 || logCounter.lastQueryEventsReq.FieldKeys[0] != "group_id" {
		t.Fatalf("expected group-filter log-sources query to project only group_id, got %#v", logCounter.lastQueryEventsReq.FieldKeys)
	}
	if logCounter.lastQueryEventsReq.GroupID != groupEntry.ID {
		t.Fatalf("expected group-filter log-sources query to include group id, got %q", logCounter.lastQueryEventsReq.GroupID)
	}
	if len(logCounter.lastQueryEventsReq.GroupAssetIDs) != 1 || logCounter.lastQueryEventsReq.GroupAssetIDs[0] != assetID {
		t.Fatalf("expected group-filter log-sources query to include fallback asset IDs, got %#v", logCounter.lastQueryEventsReq.GroupAssetIDs)
	}
}

func TestMaybeAutoCreateIncidentUsesDirectExistenceCheck(t *testing.T) {
	sut := newTestAPIServer(t)

	incidentCounter := &countingIncidentStore{IncidentStore: sut.incidentStore}
	sut.incidentStore = incidentCounter

	existing, err := sut.incidentStore.CreateIncident(incidents.CreateIncidentRequest{
		Title:    "Existing Auto Incident",
		Severity: incidents.SeverityCritical,
		Source:   incidents.SourceAlertAuto,
	})
	if err != nil {
		t.Fatalf("create auto incident: %v", err)
	}
	if _, err := sut.incidentStore.LinkIncidentAlert(existing.ID, incidents.LinkAlertRequest{
		AlertInstanceID: "inst-dup",
		LinkType:        incidents.LinkTypeTrigger,
	}); err != nil {
		t.Fatalf("link existing auto incident: %v", err)
	}

	sut.maybeAutoCreateIncident(alerts.Rule{
		ID:       "rule-dup",
		Name:     "Duplicate CPU Saturation",
		Severity: alerts.SeverityCritical,
	}, alerts.AlertInstance{
		ID:        "inst-dup",
		StartedAt: time.Now().Add(-10 * time.Minute),
	})

	autoIncidents, err := sut.incidentStore.ListIncidents(persistence.IncidentFilter{
		Limit:  10,
		Source: incidents.SourceAlertAuto,
	})
	if err != nil {
		t.Fatalf("list auto incidents: %v", err)
	}
	if len(autoIncidents) != 1 {
		t.Fatalf("expected duplicate auto incident creation to be skipped, incidents=%d", len(autoIncidents))
	}
	if incidentCounter.hasAutoIncidentCalls != 1 {
		t.Fatalf("expected direct existence helper to be used once, got %d", incidentCounter.hasAutoIncidentCalls)
	}
	if incidentCounter.listIncidentAlertLinksCalls != 0 {
		t.Fatalf("expected no per-incident link scans, got %d", incidentCounter.listIncidentAlertLinksCalls)
	}
}

func TestHandleLogSourcesPrefersRecentWindowAggregationByDefault(t *testing.T) {
	sut := newTestAPIServer(t)

	logCounter := &countingLogStore{LogStore: sut.logStore}
	sut.logStore = logCounter

	if err := sut.logStore.AppendEvent(logs.Event{
		ID:        "perf-log-source-default-window-1",
		Source:    "agent",
		Level:     "info",
		Message:   "new event",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("failed to seed log event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/sources?limit=10", nil)
	rec := httptest.NewRecorder()
	sut.handleLogSources(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	secondReq := httptest.NewRequest(http.MethodGet, "/logs/sources?limit=10", nil)
	secondRec := httptest.NewRecorder()
	sut.handleLogSources(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected second call 200, got %d", secondRec.Code)
	}
	if logCounter.listSourcesSinceCall != 1 {
		t.Fatalf("expected default path to reuse cached ListSourcesSince result across repeated calls, got %d", logCounter.listSourcesSinceCall)
	}
	if logCounter.listSourcesCalls != 0 {
		t.Fatalf("expected default path to skip ListSources all-time aggregation, got %d", logCounter.listSourcesCalls)
	}
}

func TestHandleLogSourcesAllModeUsesAllTimeAggregation(t *testing.T) {
	sut := newTestAPIServer(t)

	logCounter := &countingLogStore{LogStore: sut.logStore}
	sut.logStore = logCounter

	req := httptest.NewRequest(http.MethodGet, "/logs/sources?limit=10&all=1", nil)
	rec := httptest.NewRecorder()
	sut.handleLogSources(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if logCounter.listSourcesCalls != 1 {
		t.Fatalf("expected all-mode path to call ListSources once, got %d", logCounter.listSourcesCalls)
	}
}

func TestMetricsOverviewUsesBatchTelemetrySnapshots(t *testing.T) {
	sut := newTestAPIServer(t)

	telemetryCounter := &countingTelemetryStore{TelemetryStore: sut.telemetryStore}
	sut.telemetryStore = telemetryCounter

	seedAssetViaHeartbeat(t, sut, "batch-asset-1", "Batch One")
	seedAssetViaHeartbeat(t, sut, "batch-asset-2", "Batch Two")

	req := httptest.NewRequest(http.MethodGet, "/metrics/overview?window=15m", nil)
	rec := httptest.NewRecorder()
	sut.handleMetricsOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if telemetryCounter.snapshotManyCalls != 1 {
		t.Fatalf("expected one SnapshotMany call, got %d", telemetryCounter.snapshotManyCalls)
	}
	if telemetryCounter.snapshotCalls != 0 {
		t.Fatalf("expected Snapshot fallback path to be skipped, got %d", telemetryCounter.snapshotCalls)
	}
}

func TestStatusTelemetryOverviewUsesBatchTelemetrySnapshots(t *testing.T) {
	sut := newTestAPIServer(t)

	telemetryCounter := &countingTelemetryStore{TelemetryStore: sut.telemetryStore}
	sut.telemetryStore = telemetryCounter

	seedAssetViaHeartbeat(t, sut, "status-batch-asset-1", "Status Batch One")
	seedAssetViaHeartbeat(t, sut, "status-batch-asset-2", "Status Batch Two")

	_ = sut.buildStatusAggregateLiveResponse(context.Background(), "")
	if telemetryCounter.snapshotManyCalls != 1 {
		t.Fatalf("expected one SnapshotMany call for status telemetry overview, got %d", telemetryCounter.snapshotManyCalls)
	}
	if telemetryCounter.snapshotCalls != 0 {
		t.Fatalf("expected Snapshot fallback path to be skipped for status telemetry overview, got %d", telemetryCounter.snapshotCalls)
	}
}

func TestStatusTelemetryOverviewBatchCacheReusesSnapshots(t *testing.T) {
	sut := newTestAPIServer(t)

	telemetryCounter := &countingTelemetryStore{TelemetryStore: sut.telemetryStore}
	sut.telemetryStore = telemetryCounter

	seedAssetViaHeartbeat(t, sut, "status-cache-asset-1", "Status Cache One")
	seedAssetViaHeartbeat(t, sut, "status-cache-asset-2", "Status Cache Two")

	_ = sut.buildStatusAggregateLiveResponse(context.Background(), "")
	_ = sut.buildStatusAggregateLiveResponse(context.Background(), "")

	if telemetryCounter.snapshotManyCalls != 1 {
		t.Fatalf("expected telemetry overview cache to reuse SnapshotMany result, calls=%d", telemetryCounter.snapshotManyCalls)
	}
	if telemetryCounter.snapshotCalls != 0 {
		t.Fatalf("expected Snapshot fallback path to remain unused, got %d", telemetryCounter.snapshotCalls)
	}
}

func TestStatusTelemetryOverviewBatchCacheInvalidatesOnAssetSetChange(t *testing.T) {
	sut := newTestAPIServer(t)

	telemetryCounter := &countingTelemetryStore{TelemetryStore: sut.telemetryStore}
	sut.telemetryStore = telemetryCounter

	seedAssetViaHeartbeat(t, sut, "status-cache-invalidate-asset-1", "Status Cache Invalidate One")
	_ = sut.buildStatusAggregateLiveResponse(context.Background(), "")

	seedAssetViaHeartbeat(t, sut, "status-cache-invalidate-asset-2", "Status Cache Invalidate Two")
	_ = sut.buildStatusAggregateLiveResponse(context.Background(), "")

	if telemetryCounter.snapshotManyCalls != 2 {
		t.Fatalf("expected telemetry overview cache invalidation on asset-set change, calls=%d", telemetryCounter.snapshotManyCalls)
	}
}

func TestStatusRoutingBaseURLsUsesShortLivedCache(t *testing.T) {
	sut := newTestAPIServer(t)

	runtimeCounter := &countingRuntimeSettingsStore{
		RuntimeSettingsStore: sut.runtimeStore,
	}
	sut.runtimeStore = runtimeCounter

	_, _ = sut.statusRoutingBaseURLs()
	_, _ = sut.statusRoutingBaseURLs()

	if runtimeCounter.listOverridesCalls != 1 {
		t.Fatalf("expected runtime overrides to be loaded once within cache window, got %d", runtimeCounter.listOverridesCalls)
	}
}

func TestStatusEndpointProbeCacheReusesRecentResults(t *testing.T) {
	sut := newTestAPIServer(t)

	probeHits := 0
	previousProbe := statusProbeEndpointFunc
	statusProbeEndpointFunc = func(ctx context.Context, target statusEndpointTarget) statusEndpointResult {
		probeHits++
		return statusEndpointResult{
			Name:      target.Name,
			URL:       target.URL,
			OK:        true,
			Status:    "up",
			LatencyMs: 1,
		}
	}
	t.Cleanup(func() {
		statusProbeEndpointFunc = previousProbe
	})

	targets := []statusEndpointTarget{
		{Name: "LabTether", URL: "https://example.local/healthz"},
	}

	first := sut.statusProbeEndpointsCached(context.Background(), targets)
	second := sut.statusProbeEndpointsCached(context.Background(), targets)

	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("expected endpoint probe results on both calls")
	}
	if probeHits != 1 {
		t.Fatalf("expected endpoint probe cache to reuse recent response, hits=%d", probeHits)
	}
}
