package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWiringPinOpenAPIDocument pins the served v2 OpenAPI document byte for
// byte, as GET /api/v2/openapi.json answers it from the real route table.
// The structural tests in internal/apiv2/openapi check its meaning;
// this pin catches any byte that moves, such as a lost path section, a
// reordered constant or a changed description, when the document moves out
// of cmd. A failure names the first differing lines of the document.
//
// Regenerate (then review the diff line by line):
//
//	LABTETHER_UPDATE_WIRING_PINS=1 go test -count=1 -run 'TestWiringPin' ./cmd/labtether
func TestWiringPinOpenAPIDocument(t *testing.T) {
	handler := newTestAPIServer(t).buildHTTPHandlers(nil, nil, nil)["/api/v2/openapi.json"]
	if handler == nil {
		t.Fatal("/api/v2/openapi.json is not registered")
	}
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/api/v2/openapi.json", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("openapi answer = %d %q, want 200 application/json", rec.Code, rec.Header().Get("Content-Type"))
	}
	compareWiringPin(t, "openapi_v2.json", rec.Body.Bytes())
}
