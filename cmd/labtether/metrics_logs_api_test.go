package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/logs"
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

func TestMetricsOverviewGroupFilterReturnsServiceUnavailableWithoutGroupStore(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupStore = nil

	req := httptest.NewRequest(http.MethodGet, "/metrics/overview?group_id=group-1", nil)
	rec := httptest.NewRecorder()
	sut.handleMetricsOverview(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when group store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "group store unavailable") {
		t.Fatalf("expected group store unavailable error, got %s", rec.Body.String())
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
	if !strings.Contains(strings.ToLower(rec.Body.String()), "group store unavailable") {
		t.Fatalf("expected group store unavailable error, got %s", rec.Body.String())
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
	if !strings.Contains(strings.ToLower(rec.Body.String()), "group store unavailable") {
		t.Fatalf("expected group store unavailable error, got %s", rec.Body.String())
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
	if !strings.Contains(strings.ToLower(rec.Body.String()), "asset store unavailable") {
		t.Fatalf("expected asset store unavailable error, got %s", rec.Body.String())
	}

	sut = newTestAPIServer(t)
	sut.telemetryStore = nil
	req = httptest.NewRequest(http.MethodGet, "/metrics/overview?window=15m", nil)
	rec = httptest.NewRecorder()
	sut.handleMetricsOverview(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when telemetry store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "telemetry store unavailable") {
		t.Fatalf("expected telemetry store unavailable error, got %s", rec.Body.String())
	}

	sut = newTestAPIServer(t)
	sut.assetStore = nil
	req = httptest.NewRequest(http.MethodGet, "/metrics/assets/lab-host-01?window=15m&step=30s", nil)
	rec = httptest.NewRecorder()
	sut.handleAssetMetrics(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when asset store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "asset store unavailable") {
		t.Fatalf("expected asset store unavailable error, got %s", rec.Body.String())
	}

	sut = newTestAPIServer(t)
	sut.telemetryStore = nil
	req = httptest.NewRequest(http.MethodGet, "/metrics/assets/lab-host-01?window=15m&step=30s", nil)
	rec = httptest.NewRecorder()
	sut.handleAssetMetrics(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when telemetry store is unavailable, got %d", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "telemetry store unavailable") {
		t.Fatalf("expected telemetry store unavailable error, got %s", rec.Body.String())
	}
}
