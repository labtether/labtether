package alerting

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleAlertRules(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/alerts/rules" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.AlertStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "alert store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		rules, err := d.AlertStore.ListAlertRules(persistence.AlertRuleFilter{
			Limit:    parseLimit(r, 50),
			Offset:   parseOffset(r),
			Status:   r.URL.Query().Get("status"),
			Kind:     r.URL.Query().Get("kind"),
			Severity: r.URL.Query().Get("severity"),
		})
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list alert rules")
			return
		}
		if shared.HasAssetRestriction(r.Context()) {
			groupAccess, authErr := d.accessibleGroupIDs(r.Context())
			if authErr != nil {
				writeAssetScopeForbidden(w, "unable to prove alert rule asset scope")
				return
			}
			filtered := make([]alerts.Rule, 0, len(rules))
			for _, rule := range rules {
				if alertRuleAllowed(r.Context(), rule, groupAccess) {
					filtered = append(filtered, rule)
				}
			}
			rules = filtered
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"rules": rules})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "alerts.rule.create", 120, time.Minute) {
			return
		}

		var req alerts.CreateRuleRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid alert rule payload")
			return
		}
		NormalizeCreateAlertRuleRequest(&req)
		req.CreatedBy = apiv2.PrincipalActorID(r.Context())
		if err := ValidateCreateAlertRuleRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := d.ValidateAlertRuleTargets(req.Targets); err != nil {
			if errors.Is(err, groups.ErrGroupNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "group not found")
				return
			}
			if strings.Contains(strings.ToLower(err.Error()), "asset not found") {
				servicehttp.WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if shared.HasAssetRestriction(r.Context()) {
			groupAccess, authErr := d.accessibleGroupIDs(r.Context())
			if authErr != nil || !alertRuleInputsAllowed(r.Context(), req.Targets, groupAccess) {
				writeAssetScopeForbidden(w, "api key may only create alert rules targeting explicitly allowed assets")
				return
			}
		}

		rule, err := d.AlertStore.CreateAlertRule(req)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
				strings.Contains(strings.ToLower(err.Error()), "unique") {
				servicehttp.WriteError(w, http.StatusConflict, "alert rule target already exists")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create alert rule")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"rule": rule})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleAlertRuleActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/alerts/rules/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "alert rule path not found")
		return
	}
	if d.AlertStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "alert store unavailable")
		return
	}

	parts := strings.Split(path, "/")
	ruleID := strings.TrimSpace(parts[0])
	if ruleID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "alert rule path not found")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			rule, ok, err := d.AlertStore.GetAlertRule(ruleID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert rule")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
				return
			}
			if !d.requireAlertRuleAccess(w, r, rule) {
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"rule": rule})
		case http.MethodPatch, http.MethodPut:
			if !d.EnforceRateLimit(w, r, "alerts.rule.update", 180, time.Minute) {
				return
			}
			existingRule, ok, err := d.AlertStore.GetAlertRule(ruleID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert rule")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
				return
			}
			if !d.requireAlertRuleAccess(w, r, existingRule) {
				return
			}

			var req alerts.UpdateRuleRequest
			if err := decodeJSONBody(w, r, &req); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid alert rule payload")
				return
			}
			NormalizeUpdateAlertRuleRequest(&req)
			if err := ValidateUpdateAlertRuleRequest(existingRule, req); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			if req.Targets != nil {
				if err := d.ValidateAlertRuleTargets(*req.Targets); err != nil {
					if errors.Is(err, groups.ErrGroupNotFound) {
						servicehttp.WriteError(w, http.StatusNotFound, "group not found")
						return
					}
					if strings.Contains(strings.ToLower(err.Error()), "asset not found") {
						servicehttp.WriteError(w, http.StatusNotFound, err.Error())
						return
					}
					servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
					return
				}
				if shared.HasAssetRestriction(r.Context()) {
					groupAccess, authErr := d.accessibleGroupIDs(r.Context())
					if authErr != nil || !alertRuleInputsAllowed(r.Context(), *req.Targets, groupAccess) {
						writeAssetScopeForbidden(w, "api key may only update alert rules to target explicitly allowed assets")
						return
					}
				}
			}

			updated, err := d.AlertStore.UpdateAlertRule(ruleID, req)
			if err != nil {
				if errors.Is(err, alerts.ErrRuleNotFound) {
					servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
					return
				}
				if strings.Contains(strings.ToLower(err.Error()), "invalid") ||
					strings.Contains(strings.ToLower(err.Error()), "must be") {
					servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update alert rule")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"rule": updated})
		case http.MethodDelete:
			rule, ok, err := d.AlertStore.GetAlertRule(ruleID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert rule")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
				return
			}
			if !d.requireAlertRuleAccess(w, r, rule) {
				return
			}
			if err := d.AlertStore.DeleteAlertRule(ruleID); err != nil {
				if errors.Is(err, alerts.ErrRuleNotFound) {
					servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete alert rule")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "rule_id": ruleID})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "test" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.EnforceRateLimit(w, r, "alerts.rule.test", 240, time.Minute) {
			return
		}

		rule, ok, err := d.AlertStore.GetAlertRule(ruleID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert rule")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
			return
		}
		if !d.requireAlertRuleAccess(w, r, rule) {
			return
		}

		var req struct {
			At *time.Time `json:"at,omitempty"`
		}
		if err := decodeJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid alert test payload")
			return
		}

		evaluatedAt := time.Now().UTC()
		if req.At != nil {
			evaluatedAt = req.At.UTC()
		}

		status := alerts.EvaluationStatusOK
		candidateCount := len(rule.Targets)
		triggeredCount := 0
		details := map[string]any{
			"mode":            "manual_test",
			"rule_status":     rule.Status,
			"candidate_count": candidateCount,
		}
		if rule.Status == alerts.RuleStatusPaused {
			status = alerts.EvaluationStatusSuppressed
			details["reason"] = "rule paused"
		} else if candidateCount > 0 {
			status = alerts.EvaluationStatusTriggered
			triggeredCount = 1
		}

		evaluation, err := d.AlertStore.RecordAlertEvaluation(rule.ID, alerts.Evaluation{
			Status:         status,
			EvaluatedAt:    evaluatedAt,
			DurationMS:     1,
			CandidateCount: candidateCount,
			TriggeredCount: triggeredCount,
			Details:        details,
		})
		if err != nil {
			if errors.Is(err, alerts.ErrRuleNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to record alert evaluation")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"evaluation": evaluation})
		return
	}

	if len(parts) == 2 && parts[1] == "evaluations" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		rule, ok, err := d.AlertStore.GetAlertRule(ruleID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert rule")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
			return
		}
		if !d.requireAlertRuleAccess(w, r, rule) {
			return
		}
		evaluations, err := d.AlertStore.ListAlertEvaluations(ruleID, parseLimit(r, 50))
		if err != nil {
			if errors.Is(err, alerts.ErrRuleNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list alert evaluations")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"evaluations": evaluations})
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "unknown alert rule action")
}
