package main

import (
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
	if store, ok := c.LogStore.(statusAggregateLogWatermarkReader); ok {
		return store.LogEventsWatermark()
	}
	return time.Unix(0, 0).UTC(), nil
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

func TestHandleLogSourcesGroupFilterUsesExactSourceAggregation(t *testing.T) {
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
	if logCounter.querySourcesCalls != 1 {
		t.Fatalf("expected one QuerySourceSummaries call, got %d", logCounter.querySourcesCalls)
	}
	if logCounter.queryEventsCalls != 0 {
		t.Fatalf("expected group-filter log-sources path to skip raw QueryEvents fallback, got %d calls", logCounter.queryEventsCalls)
	}
	if logCounter.lastSourceSummaryReq.GroupID != groupEntry.ID {
		t.Fatalf("expected grouped source aggregation to include group id, got %q", logCounter.lastSourceSummaryReq.GroupID)
	}
	if len(logCounter.lastSourceSummaryReq.GroupAssetIDs) != 1 || logCounter.lastSourceSummaryReq.GroupAssetIDs[0] != assetID {
		t.Fatalf("expected grouped source aggregation to include group asset IDs, got %#v", logCounter.lastSourceSummaryReq.GroupAssetIDs)
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
