package resources

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/logs"
)

func TestHandleMobileClientTelemetryAppendsLogEvent(t *testing.T) {
	deps := newTestResourcesDeps(t)

	req := httptest.NewRequest(http.MethodPost, "/telemetry/mobile/client", bytes.NewBufferString(`{
		"route":"api.logs",
		"metric":"request.duration",
		"duration_ms":52.73,
		"status":"ok",
		"sample_size":1,
		"platform":"ios",
		"app_version":"1.0.0",
		"build":"42",
		"metadata":{"method":"get","status_code":200}
	}`))
	rec := httptest.NewRecorder()

	deps.HandleMobileClientTelemetry(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	events, err := deps.LogStore.QueryEvents(logs.QueryRequest{
		Source: "mobile_client_telemetry",
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

	fields := events[0].Fields
	if fields["route"] != "api.logs" {
		t.Fatalf("expected route field api.logs, got %q", fields["route"])
	}
	if fields["metric"] != "request.duration" {
		t.Fatalf("expected metric field request.duration, got %q", fields["metric"])
	}
	if fields["platform"] != "ios" {
		t.Fatalf("expected platform field ios, got %q", fields["platform"])
	}
	if fields["meta_method"] != "get" {
		t.Fatalf("expected metadata field meta_method=get, got %q", fields["meta_method"])
	}
}

func TestHandleMobileClientTelemetryRejectsInvalidPayload(t *testing.T) {
	deps := newTestResourcesDeps(t)

	req := httptest.NewRequest(http.MethodPost, "/telemetry/mobile/client", bytes.NewBufferString(`{
		"route":"",
		"metric":"request.duration",
		"duration_ms":-1
	}`))
	rec := httptest.NewRecorder()

	deps.HandleMobileClientTelemetry(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleMobileClientTelemetryAcceptsBatchPayload(t *testing.T) {
	deps := newTestResourcesDeps(t)

	req := httptest.NewRequest(http.MethodPost, "/telemetry/mobile/client", bytes.NewBufferString(`{
		"events":[
			{
				"route":"api.logs",
				"metric":"request.duration",
				"duration_ms":21.5,
				"status":"ok",
				"sample_size":1,
				"platform":"ios",
				"app_version":"1.0.0",
				"build":"42",
				"metadata":{"method":"get"}
			},
			{
				"route":"realtime.events",
				"metric":"connected",
				"duration_ms":0,
				"status":"ok",
				"sample_size":1,
				"platform":"ios",
				"app_version":"1.0.0",
				"build":"42",
				"metadata":{"source":"socket"}
			}
		]
	}`))
	rec := httptest.NewRecorder()

	deps.HandleMobileClientTelemetry(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	events, err := deps.LogStore.QueryEvents(logs.QueryRequest{
		Source: "mobile_client_telemetry",
		From:   time.Now().UTC().Add(-time.Minute),
		To:     time.Now().UTC().Add(time.Minute),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("query events failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 telemetry events, got %d", len(events))
	}
}
