package admin

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
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

	errPrometheusStoredPasswordConflict    = "password must be omitted when using the stored password"
	errPrometheusStoredPasswordMismatch    = "stored password reuse requires the exact configured url and username"
	errPrometheusStoredPasswordUnavailable = "stored prometheus password is not configured"
)

// PrometheusTestConnectionRequest is the request body for
// POST /settings/prometheus/test-connection.
type PrometheusTestConnectionRequest struct {
	URL               string `json:"url"`
	Username          string `json:"username"`
	Password          string `json:"password"` // #nosec G117 -- Request payload intentionally carries runtime credential material.
	UseStoredPassword bool   `json:"use_stored_password,omitempty"`
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

	username := strings.TrimSpace(req.Username)
	password := req.Password
	if req.UseStoredPassword {
		if req.Password != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, errPrometheusStoredPasswordConflict)
			return
		}
		if !apiv2.RequireScope(w, r, "credentials:use") {
			return
		}
		if d.RuntimeStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "prometheus credentials unavailable")
			return
		}

		d.runtimeSettingsMu.Lock()
		values, _, err := shared.ResolveRuntimeSettingEffectiveValues(d.RuntimeStore, d.SecretsManager)
		d.runtimeSettingsMu.Unlock()
		if err != nil {
			writeAdminInternalError(w, http.StatusServiceUnavailable, "prometheus credentials unavailable", err)
			return
		}
		if targetURL != values[runtimesettings.KeyPrometheusRemoteWriteURL] ||
			username != values[runtimesettings.KeyPrometheusRemoteWriteUsername] {
			servicehttp.WriteError(w, http.StatusBadRequest, errPrometheusStoredPasswordMismatch)
			return
		}
		password = values[runtimesettings.KeyPrometheusRemoteWritePassword]
		if password == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, errPrometheusStoredPasswordUnavailable)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), PrometheusTestConnectionTimeout)
	defer cancel()

	tester := d.PrometheusRemoteWriteTester
	if tester == nil {
		tester = TestPrometheusRemoteWriteConnection
	}
	result := tester(ctx, targetURL, username, password)
	servicehttp.WriteJSON(w, http.StatusOK, result)
}

// TestPrometheusRemoteWriteConnection sends a single test metric sample to the
// remote_write endpoint and returns a structured success/failure response.
// Exported so that apiv2_advanced.go can call it via the bridge.
func TestPrometheusRemoteWriteConnection(ctx context.Context, url, username, password string) PrometheusTestConnectionResponse {
	config, err := remotewrite.NormalizeConfig(remotewrite.Config{
		Enabled:  true,
		URL:      url,
		Username: username,
		Password: password,
		Interval: remotewrite.DefaultInterval,
	})
	if err != nil {
		return PrometheusTestConnectionResponse{Success: false, Error: err.Error()}
	}
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

	if err := remotewrite.Push(ctx, config.URL, body, config.Username, config.Password); err != nil {
		return PrometheusTestConnectionResponse{
			Success: false,
			Error:   err.Error(),
		}
	}

	return PrometheusTestConnectionResponse{Success: true}
}

func (d *Deps) validatePrometheusRemoteWriteMutation(changes map[string]string, resetKeys []string) error {
	if !prometheusRemoteWriteKeysTouched(changes, resetKeys) {
		return nil
	}
	remoteKeys := prometheusRemoteWriteSettingKeys()
	values, _, err := shared.ResolveRuntimeSettingEffectiveValues(d.RuntimeStore, d.SecretsManager)
	if err != nil {
		return fmt.Errorf("prometheus remote write settings are unavailable")
	}
	for key, value := range changes {
		if _, ok := remoteKeys[key]; !ok {
			continue
		}
		definition, _ := runtimesettings.DefinitionByKey(key)
		envValue := runtimesettings.ResolveEnvValue(definition, os.Getenv)
		values[key], _ = runtimesettings.EffectiveValue(definition, envValue, value)
	}
	if changes == nil && len(resetKeys) == 0 {
		resetKeys = make([]string, 0, len(remoteKeys))
		for key := range remoteKeys {
			resetKeys = append(resetKeys, key)
		}
	}
	for _, key := range resetKeys {
		if _, ok := remoteKeys[key]; !ok {
			continue
		}
		definition, _ := runtimesettings.DefinitionByKey(key)
		envValue := runtimesettings.ResolveEnvValue(definition, os.Getenv)
		values[key], _ = runtimesettings.EffectiveValue(definition, envValue, "")
	}
	_, err = remotewrite.ConfigFromRuntimeValues(values)
	return err
}

func prometheusRemoteWriteSettingKeys() map[string]struct{} {
	return map[string]struct{}{
		runtimesettings.KeyPrometheusRemoteWriteEnabled:  {},
		runtimesettings.KeyPrometheusRemoteWriteURL:      {},
		runtimesettings.KeyPrometheusRemoteWriteUsername: {},
		runtimesettings.KeyPrometheusRemoteWritePassword: {},
		runtimesettings.KeyPrometheusRemoteWriteInterval: {},
	}
}

func prometheusRemoteWriteKeysTouched(changes map[string]string, resetKeys []string) bool {
	remoteKeys := prometheusRemoteWriteSettingKeys()
	if changes == nil && len(resetKeys) == 0 {
		return true
	}
	for key := range changes {
		if _, ok := remoteKeys[key]; ok {
			return true
		}
	}
	for _, key := range resetKeys {
		if _, ok := remoteKeys[key]; ok {
			return true
		}
	}
	return false
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
