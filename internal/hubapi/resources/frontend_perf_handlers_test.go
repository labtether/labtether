package resources

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/logs"
)

func TestHandleFrontendPerfTelemetryAppendsLogEvent(t *testing.T) {
	deps := newTestResourcesDeps(t)

	req := httptest.NewRequest(http.MethodPost, "/telemetry/frontend/perf", bytes.NewBufferString(`{
		"route":"logs",
		"metric":"request.logs_query",
		"duration_ms":87.42,
		"status":"ok",
		"sample_size":42,
		"metadata":{"window":"1h","query_active":true,"result_count":42}
	}`))
	rec := httptest.NewRecorder()

	deps.HandleFrontendPerfTelemetry(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	events, err := deps.LogStore.QueryEvents(logs.QueryRequest{
		Source: "frontend_perf",
		From:   time.Now().UTC().Add(-time.Minute),
		To:     time.Now().UTC().Add(time.Minute),
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("query events failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 telemetry event, got %d", len(events))
	}
	if events[0].Fields["route"] != "logs" {
		t.Fatalf("expected route field logs, got %q", events[0].Fields["route"])
	}
	if events[0].Fields["metric"] != "request.logs_query" {
		t.Fatalf("expected metric field request.logs_query, got %q", events[0].Fields["metric"])
	}
	if events[0].Fields["meta_window"] != "1h" {
		t.Fatalf("expected metadata field meta_window=1h, got %q", events[0].Fields["meta_window"])
	}
}

func TestHandleFrontendPerfTelemetryRejectsInvalidPayload(t *testing.T) {
	deps := newTestResourcesDeps(t)

	req := httptest.NewRequest(http.MethodPost, "/telemetry/frontend/perf", bytes.NewBufferString(`{
		"route":"logs",
		"metric":"",
		"duration_ms":-1
	}`))
	rec := httptest.NewRecorder()

	deps.HandleFrontendPerfTelemetry(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleFrontendPerfTelemetryAcceptsExpandedRoutes(t *testing.T) {
	deps := newTestResourcesDeps(t)

	for _, route := range []string{"dashboard", "services", "topology"} {
		req := httptest.NewRequest(http.MethodPost, "/telemetry/frontend/perf", bytes.NewBufferString(`{
			"route":"`+route+`",
			"metric":"render.route",
			"duration_ms":12.5
		}`))
		rec := httptest.NewRecorder()

		deps.HandleFrontendPerfTelemetry(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("route %s: expected 202, got %d", route, rec.Code)
		}
	}
}
