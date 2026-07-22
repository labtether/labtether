package admin

import (
	"errors"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	runtimeSettingsRoute      = "/settings/runtime"
	runtimeSettingsResetRoute = "/settings/runtime/reset"

	runtimeSettingsUpdateRateLimitKey    = "settings.runtime.update"
	runtimeSettingsUpdateRateLimitCount  = 60
	runtimeSettingsUpdateRateLimitWindow = time.Minute

	runtimeSettingsResetRateLimitKey    = "settings.runtime.reset"
	runtimeSettingsResetRateLimitCount  = 60
	runtimeSettingsResetRateLimitWindow = time.Minute

	runtimeSettingsUpdatedAuditType = "settings.runtime.updated"
	runtimeSettingsResetAuditType   = "settings.runtime.reset"

	runtimeSettingsValuesRequiredError = "settings values are required"
	runtimeSettingsSaveFailureError    = "failed to save runtime settings"
	runtimeSettingsResetFailureError   = "failed to reset runtime settings"

	runtimeSettingsUpdateAuditWarning = "api warning: failed to append runtime settings update audit event"
	runtimeSettingsResetAuditWarning  = "api warning: failed to append runtime settings reset audit event"
)

// HandleRuntimeSettings handles GET and PATCH /settings/runtime.
func (d *Deps) HandleRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != runtimeSettingsRoute {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.RuntimeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "runtime settings unavailable")
		return
	}
	d.runtimeSettingsMu.Lock()
	defer d.runtimeSettingsMu.Unlock()

	switch r.Method {
	case http.MethodGet:
		d.writeRuntimeSettingsPayloadLocked(w)
	case http.MethodPatch:
		if !d.enforceRateLimit(
			w,
			r,
			runtimeSettingsUpdateRateLimitKey,
			runtimeSettingsUpdateRateLimitCount,
			runtimeSettingsUpdateRateLimitWindow,
		) {
			return
		}

		var req shared.RuntimeSettingsUpdateRequest
		if err := d.decodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid runtime settings payload")
			return
		}
		normalized, err := shared.NormalizeRuntimeOverrides(req.Values)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(normalized) == 0 {
			servicehttp.WriteError(w, http.StatusBadRequest, runtimeSettingsValuesRequiredError)
			return
		}
		if err := d.validatePrometheusRemoteWriteMutation(normalized, nil); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		stored, err := shared.PrepareRuntimeOverridesForStorage(normalized, d.SecretsManager)
		if err != nil {
			writeAdminInternalError(w, http.StatusServiceUnavailable, "runtime secret storage unavailable", err)
			return
		}
		overrides, err := d.RuntimeStore.SaveRuntimeSettingOverrides(stored)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, runtimeSettingsSaveFailureError)
			return
		}
		if d.PolicyState != nil {
			d.PolicyState.ApplyOverrides(shared.NonSensitiveRuntimeOverrides(overrides))
		}
		if d.ApplySecurityRuntimeOverrides != nil {
			d.ApplySecurityRuntimeOverrides(shared.NonSensitiveRuntimeOverrides(overrides))
		}
		if d.InvalidateWebServiceURLGroupingConfigCache != nil {
			d.InvalidateWebServiceURLGroupingConfigCache()
		}
		updatedKeys := make([]string, 0, len(normalized))
		for key := range normalized {
			updatedKeys = append(updatedKeys, key)
		}
		sort.Strings(updatedKeys)
		auditEvent := audit.NewEvent(runtimeSettingsUpdatedAuditType)
		auditEvent.ActorID = d.principalActorID(r.Context())
		auditEvent.Decision = "applied"
		auditEvent.Details = map[string]any{
			"keys":  updatedKeys,
			"count": len(updatedKeys),
		}
		d.appendAuditEventBestEffort(auditEvent, runtimeSettingsUpdateAuditWarning)
		if d.ApplyPrometheusRemoteWriteSettings != nil && prometheusRemoteWriteKeysTouched(normalized, nil) {
			if err := d.ApplyPrometheusRemoteWriteSettings(); err != nil {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "settings saved but prometheus remote write runtime reload failed")
				return
			}
		}
		d.writeRuntimeSettingsPayloadLocked(w)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleRuntimeSettingsReset handles POST /settings/runtime/reset.
func (d *Deps) HandleRuntimeSettingsReset(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != runtimeSettingsResetRoute {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.RuntimeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "runtime settings unavailable")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.enforceRateLimit(
		w,
		r,
		runtimeSettingsResetRateLimitKey,
		runtimeSettingsResetRateLimitCount,
		runtimeSettingsResetRateLimitWindow,
	) {
		return
	}
	d.runtimeSettingsMu.Lock()
	defer d.runtimeSettingsMu.Unlock()

	var req shared.RuntimeSettingsResetRequest
	if err := d.decodeJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid runtime settings reset payload")
		return
	}

	keys, err := shared.SanitizeRuntimeSettingKeys(req.Keys)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := d.validatePrometheusRemoteWriteMutation(nil, keys); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := d.RuntimeStore.DeleteRuntimeSettingOverrides(keys); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, runtimeSettingsResetFailureError)
		return
	}
	overrides, err := d.RuntimeStore.ListRuntimeSettingOverrides()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, shared.RuntimeSettingsLoadFailureError)
		return
	}
	if d.PolicyState != nil {
		d.PolicyState.ApplyOverrides(shared.NonSensitiveRuntimeOverrides(overrides))
	}
	if d.ApplySecurityRuntimeOverrides != nil {
		d.ApplySecurityRuntimeOverrides(shared.NonSensitiveRuntimeOverrides(overrides))
	}
	if d.InvalidateWebServiceURLGroupingConfigCache != nil {
		d.InvalidateWebServiceURLGroupingConfigCache()
	}
	resetKeys := append([]string(nil), keys...)
	sort.Strings(resetKeys)
	auditEvent := audit.NewEvent(runtimeSettingsResetAuditType)
	auditEvent.ActorID = d.principalActorID(r.Context())
	auditEvent.Decision = "applied"
	auditEvent.Details = map[string]any{
		"keys":  resetKeys,
		"count": len(resetKeys),
	}
	d.appendAuditEventBestEffort(auditEvent, runtimeSettingsResetAuditWarning)
	if d.ApplyPrometheusRemoteWriteSettings != nil && prometheusRemoteWriteKeysTouched(nil, keys) {
		if err := d.ApplyPrometheusRemoteWriteSettings(); err != nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "settings reset but prometheus remote write runtime reload failed")
			return
		}
	}
	d.writeRuntimeSettingsPayloadLocked(w)
}

func (d *Deps) writeRuntimeSettingsPayloadLocked(w http.ResponseWriter) {
	if err := shared.WriteRuntimeSettingsPayload(w, d.RuntimeStore, d.SecretsManager); err != nil {
		writeAdminInternalError(w, http.StatusInternalServerError, shared.RuntimeSettingsLoadFailureError, err)
	}
}

// WriteRuntimeSettingsPayload is the exported equivalent used by the bridge.
func (d *Deps) WriteRuntimeSettingsPayload(w http.ResponseWriter) {
	d.runtimeSettingsMu.Lock()
	defer d.runtimeSettingsMu.Unlock()
	d.writeRuntimeSettingsPayloadLocked(w)
}
