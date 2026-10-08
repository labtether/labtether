package main

import (
	"context"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/updates"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func (c *countingUpdateStore) ListUpdateRunsPage(limit, offset int, status string) ([]updates.Run, error) {
	c.listUpdateRunsCalls++
	if store, ok := c.UpdateStore.(persistence.UpdateRunPageStore); ok {
		return store.ListUpdateRunsPage(limit, offset, status)
	}
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
