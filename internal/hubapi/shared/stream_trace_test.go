package shared

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserStreamTraceIDSanitizesValue(t *testing.T) {
	req := httptest.NewRequest("GET", "/terminal/sessions/s1/stream?lt_trace=ios-ABC_123.%24bad%20chars", nil)
	got := BrowserStreamTraceID(req)
	if got != "ios-ABC_123.badchars" {
		t.Fatalf("unexpected sanitized trace id: %q", got)
	}
}

func TestBrowserStreamTraceIDAppliesLengthLimit(t *testing.T) {
	raw := strings.Repeat("a", StreamTraceMaxLen+12)
	req := httptest.NewRequest("GET", "/terminal/sessions/s1/stream?lt_trace="+raw, nil)
	got := BrowserStreamTraceID(req)
	if len(got) != StreamTraceMaxLen {
		t.Fatalf("expected trace length %d, got %d", StreamTraceMaxLen, len(got))
	}
}

func TestStreamTraceLogValueFallback(t *testing.T) {
	if got := StreamTraceLogValue(""); got != "-" {
		t.Fatalf("expected fallback '-', got %q", got)
	}
	if got := StreamTraceLogValue("ios-123"); got != "ios-123" {
		t.Fatalf("expected passthrough trace id, got %q", got)
	}
}
