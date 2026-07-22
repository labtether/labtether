package alerting

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleAlertInstances(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/alerts/instances" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.AlertInstanceStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "alert instance store unavailable")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if ruleID := strings.TrimSpace(r.URL.Query().Get("rule_id")); shared.HasAssetRestriction(r.Context()) && ruleID != "" {
		rule, ok, err := d.AlertStore.GetAlertRule(ruleID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize alert rule")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
			return
		}
		if !d.requireAlertRuleAccess(w, r, rule) {
			return
		}
	}

	instances, err := d.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		Limit:    parseLimit(r, 50),
		Offset:   parseOffset(r),
		RuleID:   r.URL.Query().Get("rule_id"),
		Status:   r.URL.Query().Get("status"),
		Severity: r.URL.Query().Get("severity"),
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list alert instances")
		return
	}
	if shared.HasAssetRestriction(r.Context()) {
		groupAccess, authErr := d.accessibleGroupIDs(r.Context())
		if authErr != nil {
			writeAssetScopeForbidden(w, "unable to prove alert instance asset scope")
			return
		}
		filtered := make([]alerts.AlertInstance, 0, len(instances))
		for _, instance := range instances {
			allowed, checkErr := d.alertInstanceAllowed(r.Context(), instance, groupAccess)
			if checkErr != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize alert instances")
				return
			}
			if allowed {
				filtered = append(filtered, instance)
			}
		}
		instances = filtered
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"instances": instances})
}

func (d *Deps) HandleAlertInstanceActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/alerts/instances/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "alert instance path not found")
		return
	}
	if d.AlertInstanceStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "alert instance store unavailable")
		return
	}

	parts := strings.Split(path, "/")
	instanceID := strings.TrimSpace(parts[0])
	if instanceID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "alert instance path not found")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			inst, ok, err := d.AlertInstanceStore.GetAlertInstance(instanceID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert instance")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "alert instance not found")
				return
			}
			if shared.HasAssetRestriction(r.Context()) {
				groupAccess, authErr := d.accessibleGroupIDs(r.Context())
				allowed, checkErr := d.alertInstanceAllowed(r.Context(), inst, groupAccess)
				if authErr != nil || checkErr != nil || !allowed {
					writeAssetScopeForbidden(w, "api key does not have access to this alert instance")
					return
				}
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"instance": inst})
			return
		case http.MethodDelete:
			inst, ok, err := d.AlertInstanceStore.GetAlertInstance(instanceID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert instance")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "alert instance not found")
				return
			}
			if shared.HasAssetRestriction(r.Context()) {
				groupAccess, authErr := d.accessibleGroupIDs(r.Context())
				allowed, checkErr := d.alertInstanceAllowed(r.Context(), inst, groupAccess)
				if authErr != nil || checkErr != nil || !allowed {
					writeAssetScopeForbidden(w, "api key does not have access to this alert instance")
					return
				}
			}
			if err := d.AlertInstanceStore.DeleteAlertInstance(instanceID); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					servicehttp.WriteError(w, http.StatusNotFound, "alert instance not found")
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete alert instance")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
			return
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	if len(parts) == 2 && parts[1] == "ack" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.EnforceRateLimit(w, r, "alerts.instances.ack", 120, time.Minute) {
			return
		}
		inst, ok, err := d.AlertInstanceStore.GetAlertInstance(instanceID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert instance")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "alert instance not found")
			return
		}
		if shared.HasAssetRestriction(r.Context()) {
			groupAccess, authErr := d.accessibleGroupIDs(r.Context())
			allowed, checkErr := d.alertInstanceAllowed(r.Context(), inst, groupAccess)
			if authErr != nil || checkErr != nil || !allowed {
				writeAssetScopeForbidden(w, "api key does not have access to this alert instance")
				return
			}
		}
		updated, err := d.AlertInstanceStore.UpdateAlertInstanceStatus(instanceID, alerts.InstanceStatusAcknowledged)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				servicehttp.WriteError(w, http.StatusNotFound, "alert instance not found")
				return
			}
			if strings.Contains(strings.ToLower(err.Error()), "cannot transition") {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to acknowledge alert instance")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"instance": updated})
		return
	}

	if len(parts) == 2 && parts[1] == "resolve" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.EnforceRateLimit(w, r, "alerts.instances.resolve", 120, time.Minute) {
			return
		}
		inst, ok, err := d.AlertInstanceStore.GetAlertInstance(instanceID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert instance")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "alert instance not found")
			return
		}
		if shared.HasAssetRestriction(r.Context()) {
			groupAccess, authErr := d.accessibleGroupIDs(r.Context())
			allowed, checkErr := d.alertInstanceAllowed(r.Context(), inst, groupAccess)
			if authErr != nil || checkErr != nil || !allowed {
				writeAssetScopeForbidden(w, "api key does not have access to this alert instance")
				return
			}
		}
		updated, err := d.AlertInstanceStore.UpdateAlertInstanceStatus(instanceID, alerts.InstanceStatusResolved)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				servicehttp.WriteError(w, http.StatusNotFound, "alert instance not found")
				return
			}
			if strings.Contains(strings.ToLower(err.Error()), "cannot transition") {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to resolve alert instance")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"instance": updated})
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "unknown alert instance action")
}

func (d *Deps) HandleAlertSilences(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/alerts/silences" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.AlertInstanceStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "alert instance store unavailable")
		return
	}
	if denyAssetRestrictedGlobal(w, r, "alert silences") {
		return
	}

	switch r.Method {
	case http.MethodGet:
		activeOnly := strings.TrimSpace(r.URL.Query().Get("active")) == "true"
		silences, err := d.AlertInstanceStore.ListAlertSilences(parseLimit(r, 50), activeOnly)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list alert silences")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"silences": silences})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "alerts.silences.create", 60, time.Minute) {
			return
		}
		var req alerts.CreateSilenceRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid silence payload")
			return
		}
		if err := ValidateSilenceRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Attribution is derived from the authenticated request. Never trust a
		// caller-supplied created_by value for an operational suppression record.
		req.CreatedBy = apiv2.PrincipalActorID(r.Context())
		silence, err := d.AlertInstanceStore.CreateAlertSilence(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create silence")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"silence": silence})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleAlertSilenceActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/alerts/silences/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "silence path not found")
		return
	}
	if d.AlertInstanceStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "alert instance store unavailable")
		return
	}
	if denyAssetRestrictedGlobal(w, r, "alert silences") {
		return
	}

	silenceID := strings.TrimSpace(path)
	switch r.Method {
	case http.MethodGet:
		silence, ok, err := d.AlertInstanceStore.GetAlertSilence(silenceID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load silence")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "silence not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"silence": silence})
	case http.MethodDelete:
		if err := d.AlertInstanceStore.DeleteAlertSilence(silenceID); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				servicehttp.WriteError(w, http.StatusNotFound, "silence not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete silence")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
