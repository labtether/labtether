package admin

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/telemetry/remotewrite"
)

const (
	prometheusTestConnectionRoute = "/settings/prometheus/test-connection"

	// PrometheusTestConnectionRateLimitKey, PrometheusTestConnectionRateLimitCount,
	// PrometheusTestConnectionRateLimitWindow, and PrometheusTestConnectionTimeout
	// are exported so that apiv2_advanced.go can reference them via the bridge.
	PrometheusTestConnectionRateLimitKey    = "settings.prometheus.test_connection"
	PrometheusTestConnectionRateLimitCount  = 20
	PrometheusTestConnectionRateLimitWindow = time.Minute

	PrometheusTestConnectionTimeout = 15 * time.Second

	prometheusTestMetricName = "labtether_remote_write_test"

	// ErrPrometheusURLRequired is the error message for a missing Prometheus URL.
	ErrPrometheusURLRequired = "url is required"
)

// PrometheusTestConnectionRequest is the request body for
// POST /settings/prometheus/test-connection.
type PrometheusTestConnectionRequest struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"` // #nosec G117 -- Request payload intentionally carries runtime credential material.
}

// PrometheusTestConnectionResponse is the response body for
// POST /settings/prometheus/test-connection.
type PrometheusTestConnectionResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// HandlePrometheusTestConnection handles POST /settings/prometheus/test-connection.
// It accepts URL and optional credentials from the request body, sends a single
// test sample to the remote_write endpoint, and returns success/failure.
func (d *Deps) HandlePrometheusTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != prometheusTestConnectionRoute {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.enforceRateLimit(
		w,
		r,
		PrometheusTestConnectionRateLimitKey,
		PrometheusTestConnectionRateLimitCount,
		PrometheusTestConnectionRateLimitWindow,
	) {
		return
	}

	var req PrometheusTestConnectionRequest
	if err := d.decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid prometheus test connection payload")
		return
	}

	targetURL := strings.TrimSpace(req.URL)
	if targetURL == "" {
		servicehttp.WriteJSON(w, http.StatusOK, PrometheusTestConnectionResponse{
			Success: false,
			Error:   ErrPrometheusURLRequired,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), PrometheusTestConnectionTimeout)
	defer cancel()

	result := TestPrometheusRemoteWriteConnection(ctx, targetURL, req.Username, req.Password)
	servicehttp.WriteJSON(w, http.StatusOK, result)
}

// TestPrometheusRemoteWriteConnection sends a single test metric sample to the
// remote_write endpoint and returns a structured success/failure response.
// Exported so that apiv2_advanced.go can call it via the bridge.
func TestPrometheusRemoteWriteConnection(ctx context.Context, url, username, password string) PrometheusTestConnectionResponse {
	sample := remotewrite.SampleWithLabels{
		Labels: map[string]string{
			"__name__": prometheusTestMetricName,
			"job":      "labtether-hub",
		},
		Value:     1,
		Timestamp: time.Now().UnixMilli(),
	}

	body, err := remotewrite.SerializeWriteRequest([]remotewrite.SampleWithLabels{sample})
	if err != nil {
		return PrometheusTestConnectionResponse{
			Success: false,
			Error:   "failed to serialize test metric: " + err.Error(),
		}
	}

	if err := remotewrite.Push(ctx, url, body, username, password); err != nil {
		return PrometheusTestConnectionResponse{
			Success: false,
			Error:   err.Error(),
		}
	}

	return PrometheusTestConnectionResponse{Success: true}
}

// PrometheusSettingsEnabled returns the effective value of a boolean runtime
// setting key, defaulting to false when the store is unavailable or the setting
// has not been configured. It is exported so that cmd/labtether can call it
// from its metrics-export startup without importing internal details.
func PrometheusSettingsEnabled(store interface {
	ListRuntimeSettingOverrides() (map[string]string, error)
}, key string) bool {
	if store == nil {
		return false
	}
	overrides, err := store.ListRuntimeSettingOverrides()
	if err != nil {
		return false
	}
	if raw, ok := overrides[key]; ok {
		def, defOK := runtimesettings.DefinitionByKey(key)
		if !defOK {
			return false
		}
		normalized, normErr := runtimesettings.NormalizeValue(def, raw)
		if normErr != nil {
			return false
		}
		return normalized == "true"
	}
	// Fall back to default value.
	def, ok := runtimesettings.DefinitionByKey(key)
	if !ok {
		return false
	}
	return def.DefaultValue == "true"
}
