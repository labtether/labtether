package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

func TestMetricsOverview(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"asset_id":"lab-host-01","type":"host","name":"Lab Host 01","source":"agent","status":"online","platform":"linux","metadata":{"cpu_percent":"17.2","memory_percent":"22.3","disk_percent":"41.0","temp_celsius":"52.1","network_rx_bytes_per_sec":"1024","network_tx_bytes_per_sec":"2048"}}`)
	createReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
	createRec := httptest.NewRecorder()
	sut.handleAssetActions(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", createRec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics/overview?window=15m", nil)
	rec := httptest.NewRecorder()
	sut.handleMetricsOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAssetMetricsSeries(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"asset_id":"lab-host-01","type":"host","name":"Lab Host 01","source":"agent","status":"online","platform":"linux","metadata":{"cpu_percent":"17.2","memory_percent":"22.3","disk_percent":"41.0","temp_celsius":"52.1","network_rx_bytes_per_sec":"1024","network_tx_bytes_per_sec":"2048"}}`)
	createReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
	createRec := httptest.NewRecorder()
	sut.handleAssetActions(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", createRec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics/assets/lab-host-01?window=15m&step=30s", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetMetrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestLogsQuery(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"asset_id":"lab-host-logs","type":"host","name":"Lab Host Logs","source":"agent","status":"online","platform":"linux"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
	createRec := httptest.NewRecorder()
	sut.handleAssetActions(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", createRec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/query?window=1h&limit=20", nil)
	rec := httptest.NewRecorder()
	sut.handleLogsQuery(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestLogsQueryIncludeFieldsFalseOmitsFields(t *testing.T) {
	sut := newTestAPIServer(t)

	if err := sut.logStore.AppendEvent(logs.Event{
		ID:      "logs-no-fields-1",
		AssetID: "lab-host-logs",
		Source:  "agent",
		Level:   "info",
		Message: "hello",
		Fields: map[string]string{
			"component": "collector",
		},
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append event failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/query?window=1h&limit=20&include_fields=0", nil)
	rec := httptest.NewRecorder()
	sut.handleLogsQuery(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Events []logs.Event `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if len(payload.Events) == 0 {
		t.Fatalf("expected at least one event")
	}
	if len(payload.Events[0].Fields) > 0 {
		t.Fatalf("expected fields to be omitted when include_fields=0")
	}
}

func TestLogsQueryGroupFilterIncludeFieldsFalseStillFiltersByFieldGroupID(t *testing.T) {
	sut := newTestAPIServer(t)

	groupEntry, err := sut.groupStore.CreateGroup(groups.CreateRequest{
		Name: "Group One",
		Slug: "group-one",
	})
	if err != nil {
		t.Fatalf("create group failed: %v", err)
	}

	if err := sut.logStore.AppendEvent(logs.Event{
		ID:      "logs-group-field-1",
		Source:  "agent",
		Level:   "warn",
		Message: "group-scoped event",
		Fields: map[string]string{
			"group_id": groupEntry.ID,
		},
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append event failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/query?window=1h&limit=20&group_id="+groupEntry.ID+"&include_fields=0", nil)
	rec := httptest.NewRecorder()
	sut.handleLogsQuery(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Events []logs.Event `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected one filtered event, got %d", len(payload.Events))
	}
}

func TestLogViewsCreateAndList(t *testing.T) {
	sut := newTestAPIServer(t)

	createPayload := []byte(`{"name":"Errors Last Hour","level":"error","window":"1h"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/logs/views", bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	sut.handleLogViews(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/logs/views", nil)
	listRec := httptest.NewRecorder()
	sut.handleLogViews(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}
}

func TestLogViewsAreScopedToAuthenticatedActor(t *testing.T) {
	sut := newTestAPIServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/logs/views", bytes.NewReader([]byte(`{"name":"Actor A View","level":"error","window":"1h"}`)))
	createReq = createReq.WithContext(contextWithPrincipal(createReq.Context(), "actor-a", "operator"))
	createRec := httptest.NewRecorder()
	sut.handleLogViews(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		View logs.SavedView `json:"view"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	if created.View.ID == "" {
		t.Fatalf("expected created view id")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/logs/views", nil)
	listReq = listReq.WithContext(contextWithPrincipal(listReq.Context(), "actor-b", "operator"))
	listRec := httptest.NewRecorder()
	sut.handleLogViews(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 listing as actor-b, got %d", listRec.Code)
	}

	var listed struct {
		Views []logs.SavedView `json:"views"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response failed: %v", err)
	}
	if len(listed.Views) != 0 {
		t.Fatalf("expected actor-b to see no views, got %d", len(listed.Views))
	}

	getReq := httptest.NewRequest(http.MethodGet, "/logs/views/"+created.View.ID, nil)
	getReq = getReq.WithContext(contextWithPrincipal(getReq.Context(), "actor-b", "operator"))
	getRec := httptest.NewRecorder()
	sut.handleLogViewActions(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected cross-actor GET to return 404, got %d", getRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/logs/views/"+created.View.ID, nil)
	deleteReq = deleteReq.WithContext(contextWithPrincipal(deleteReq.Context(), "actor-b", "operator"))
	deleteRec := httptest.NewRecorder()
	sut.handleLogViewActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNotFound {
		t.Fatalf("expected cross-actor DELETE to return 404, got %d", deleteRec.Code)
	}

	ownerGetReq := httptest.NewRequest(http.MethodGet, "/logs/views/"+created.View.ID, nil)
	ownerGetReq = ownerGetReq.WithContext(contextWithPrincipal(ownerGetReq.Context(), "actor-a", "operator"))
	ownerGetRec := httptest.NewRecorder()
	sut.handleLogViewActions(ownerGetRec, ownerGetReq)
	if ownerGetRec.Code != http.StatusOK {
		t.Fatalf("expected owner GET to succeed, got %d", ownerGetRec.Code)
	}
}

func TestLogViewsCreateRejectsClientSuppliedID(t *testing.T) {
	sut := newTestAPIServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/logs/views", bytes.NewReader([]byte(`{"name":"Original","level":"error","window":"1h"}`)))
	createReq = createReq.WithContext(contextWithPrincipal(createReq.Context(), "actor-a", "operator"))
	createRec := httptest.NewRecorder()
	sut.handleLogViews(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		View logs.SavedView `json:"view"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}

	overwriteBody := []byte(`{"id":"` + created.View.ID + `","name":"Overwritten","level":"warn","window":"24h"}`)
	overwriteReq := httptest.NewRequest(http.MethodPost, "/logs/views", bytes.NewReader(overwriteBody))
	overwriteReq = overwriteReq.WithContext(contextWithPrincipal(overwriteReq.Context(), "actor-a", "operator"))
	overwriteRec := httptest.NewRecorder()
	sut.handleLogViews(overwriteRec, overwriteReq)
	if overwriteRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when client supplies id on create, got %d", overwriteRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/logs/views/"+created.View.ID, nil)
	getReq = getReq.WithContext(contextWithPrincipal(getReq.Context(), "actor-a", "operator"))
	getRec := httptest.NewRecorder()
	sut.handleLogViewActions(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected original view to remain readable, got %d", getRec.Code)
	}

	var fetched struct {
		View logs.SavedView `json:"view"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode get response failed: %v", err)
	}
	if fetched.View.Name != "Original" {
		t.Fatalf("expected original view name to remain unchanged, got %q", fetched.View.Name)
	}
}

func TestLogSourcesGroupFilterCountsBeyondThousandEventsExactly(t *testing.T) {
	sut := newTestAPIServer(t)

	groupID := mustCreateGroup(t, sut, "Logs Exact", "logs-exact")
	seedAssetViaHeartbeatWithSite(t, sut, "logs-exact-asset", groupID)

	now := time.Now().UTC()
	for i := 0; i < 1205; i++ {
		if err := sut.logStore.AppendEvent(logs.Event{
			ID:        "logs-exact-" + strconv.Itoa(i),
			AssetID:   "logs-exact-asset",
			Source:    "collector",
			Level:     "info",
			Message:   "group event",
			Timestamp: now.Add(-time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("append event %d failed: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/sources?group_id="+groupID+"&limit=10&window=1h", nil)
	rec := httptest.NewRecorder()
	sut.handleLogSources(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Sources []logs.SourceSummary `json:"sources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if len(payload.Sources) != 1 {
		t.Fatalf("expected one source summary, got %d", len(payload.Sources))
	}
	if payload.Sources[0].Count != 1205 {
		t.Fatalf("expected exact grouped source count of 1205, got %d", payload.Sources[0].Count)
	}
}

func TestGroupedLogReadsUseGroupedAssetStorePath(t *testing.T) {
	sut := newTestAPIServer(t)
	trackingStore := &trackingGroupAssetStore{MemoryAssetStore: persistence.NewMemoryAssetStore()}
	sut.assetStore = trackingStore

	groupID := mustCreateGroup(t, sut, "Logs Grouped", "logs-grouped")
	seedAssetViaHeartbeatWithSite(t, sut, "logs-grouped-asset", groupID)

	if err := sut.logStore.AppendEvent(logs.Event{
		ID:        "logs-grouped-1",
		AssetID:   "logs-grouped-asset",
		Source:    "agent",
		Level:     "info",
		Message:   "grouped log query",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append event failed: %v", err)
	}

	queryReq := httptest.NewRequest(http.MethodGet, "/logs/query?group_id="+groupID+"&limit=20", nil)
	queryRec := httptest.NewRecorder()
	sut.handleLogsQuery(queryRec, queryReq)
	if queryRec.Code != http.StatusOK {
		t.Fatalf("expected grouped log query 200, got %d", queryRec.Code)
	}
	if trackingStore.listAssetsByGroupCalls != 1 || trackingStore.listAssetsCalls != 0 {
		t.Fatalf("expected grouped log query to use ListAssetsByGroup only, calls=%d full_scans=%d", trackingStore.listAssetsByGroupCalls, trackingStore.listAssetsCalls)
	}

	trackingStore.listAssetsByGroupCalls = 0
	trackingStore.listAssetsCalls = 0

	sourceReq := httptest.NewRequest(http.MethodGet, "/logs/sources?group_id="+groupID+"&limit=20", nil)
	sourceRec := httptest.NewRecorder()
	sut.handleLogSources(sourceRec, sourceReq)
	if sourceRec.Code != http.StatusOK {
		t.Fatalf("expected grouped log sources 200, got %d", sourceRec.Code)
	}
	if trackingStore.listAssetsByGroupCalls != 1 || trackingStore.listAssetsCalls != 0 {
		t.Fatalf("expected grouped log sources to use ListAssetsByGroup only, calls=%d full_scans=%d", trackingStore.listAssetsByGroupCalls, trackingStore.listAssetsCalls)
	}
}

func TestMetricsOverviewGroupFilterReturnsServiceUnavailableWithoutGroupStore(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupStore = nil

	req := httptest.NewRequest(http.MethodGet, "/metrics/overview?group_id=group-1", nil)
	rec := httptest.NewRecorder()
	sut.handleMetricsOverview(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when group store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %s", rec.Body.String())
	}
}

func TestLogsQueryGroupFilterReturnsServiceUnavailableWithoutGroupStore(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupStore = nil

	req := httptest.NewRequest(http.MethodGet, "/logs/query?group_id=group-1", nil)
	rec := httptest.NewRecorder()
	sut.handleLogsQuery(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when group store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %s", rec.Body.String())
	}
}

func TestLogSourcesGroupFilterReturnsServiceUnavailableWithoutGroupStore(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupStore = nil

	req := httptest.NewRequest(http.MethodGet, "/logs/sources?group_id=group-1", nil)
	rec := httptest.NewRecorder()
	sut.handleLogSources(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when group store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %s", rec.Body.String())
	}
}

func TestMetricsHandlersReturnServiceUnavailableWithoutStores(t *testing.T) {
	sut := newTestAPIServer(t)

	sut.assetStore = nil
	req := httptest.NewRequest(http.MethodGet, "/metrics/overview?window=15m", nil)
	rec := httptest.NewRecorder()
	sut.handleMetricsOverview(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when asset store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %s", rec.Body.String())
	}

	sut = newTestAPIServer(t)
	sut.telemetryStore = nil
	req = httptest.NewRequest(http.MethodGet, "/metrics/overview?window=15m", nil)
	rec = httptest.NewRecorder()
	sut.handleMetricsOverview(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when telemetry store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %s", rec.Body.String())
	}

	sut = newTestAPIServer(t)
	sut.assetStore = nil
	req = httptest.NewRequest(http.MethodGet, "/metrics/assets/lab-host-01?window=15m&step=30s", nil)
	rec = httptest.NewRecorder()
	sut.handleAssetMetrics(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when asset store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %s", rec.Body.String())
	}

	sut = newTestAPIServer(t)
	sut.telemetryStore = nil
	req = httptest.NewRequest(http.MethodGet, "/metrics/assets/lab-host-01?window=15m&step=30s", nil)
	rec = httptest.NewRecorder()
	sut.handleAssetMetrics(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when telemetry store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %s", rec.Body.String())
	}
}
