package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParsePromSampleValueRejectsNonFinite(t *testing.T) {
	for _, raw := range []any{"NaN", "Inf", "-Inf"} {
		if got, err := parsePromSampleValue(raw); err == nil {
			t.Fatalf("parsePromSampleValue(%#v) = %v, nil; want error", raw, got)
		}
	}
}

func TestParsePromSampleTSRejectsNonFinite(t *testing.T) {
	for _, raw := range []any{"NaN", "Inf", "-Inf"} {
		if got, err := parsePromSampleTS(raw); err == nil {
			t.Fatalf("parsePromSampleTS(%#v) = %v, nil; want error", raw, got)
		}
	}
}

func TestParsePromSampleTSRejectsOutOfRange(t *testing.T) {
	for _, raw := range []any{"1e100", 1e100} {
		if got, err := parsePromSampleTS(raw); err == nil {
			t.Fatalf("parsePromSampleTS(%#v) = %v, nil; want error", raw, got)
		}
	}
}

func TestPrometheusClientRejectsOversizedResponse(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","padding":"`))
		_, _ = w.Write([]byte(strings.Repeat("x", maxPrometheusResponseBytes)))
		_, _ = w.Write([]byte(`"}`))
	}))
	defer server.Close()

	_, err := NewPrometheusClient(server.URL).QuerySingleValue(context.Background(), "up", time.Now())
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("expected response-size error, got %v", err)
	}
}
