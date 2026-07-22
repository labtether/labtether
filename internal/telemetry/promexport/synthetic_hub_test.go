package promexport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestSyntheticHubMetricsExportWithClosedSecretFreeLabels(t *testing.T) {
	labels := map[string]string{
		"check_id": "check-1", "check_name": "Private endpoint", "check_type": "http",
	}
	source := &mockSource{hub: map[string][]LabeledMetric{
		telemetry.MetricScopeHubSynthetic: {
			{Metric: telemetry.MetricSyntheticLatencyMs, Value: 12, Labels: labels},
			{Metric: telemetry.MetricSyntheticStatus, Value: 1, Labels: labels},
		},
	}}
	recorder := httptest.NewRecorder()
	NewHandler(source).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("synthetic metrics status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		"labtether_synthetic_latency_ms", "labtether_synthetic_status",
		`scope="hub-synthetic"`, `check_id="check-1"`, `check_name="Private endpoint"`, `check_type="http"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("synthetic scrape missing %s:\n%s", expected, body)
		}
	}
	for _, forbidden := range []string{"password", "token=", `target="`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("synthetic scrape leaked %q:\n%s", forbidden, body)
		}
	}
}
