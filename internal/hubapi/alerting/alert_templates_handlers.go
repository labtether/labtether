package alerting

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

type alertRuleTemplate struct {
	ID                 string
	Name               string
	Description        string
	Kind               string
	Severity           string
	TargetScope        string
	CooldownSeconds    int
	ReopenAfterSeconds int
	EvaluationInterval int
	WindowSeconds      int
	Condition          map[string]any
	Labels             map[string]string
	Metadata           map[string]string
	DefaultCreatedBy   string
}

// AlertRuleTemplateResponse is the JSON response shape for an alert rule template.
type AlertRuleTemplateResponse struct {
	ID                        string            `json:"id"`
	Name                      string            `json:"name"`
	Description               string            `json:"description"`
	Kind                      string            `json:"kind"`
	Severity                  string            `json:"severity"`
	TargetScope               string            `json:"target_scope"`
	CooldownSeconds           int               `json:"cooldown_seconds"`
	ReopenAfterSeconds        int               `json:"reopen_after_seconds"`
	EvaluationIntervalSeconds int               `json:"evaluation_interval_seconds"`
	WindowSeconds             int               `json:"window_seconds"`
	Condition                 map[string]any    `json:"condition"`
	Labels                    map[string]string `json:"labels,omitempty"`
	Metadata                  map[string]string `json:"metadata,omitempty"`
}

func (t alertRuleTemplate) toResponse() AlertRuleTemplateResponse {
	return AlertRuleTemplateResponse{
		ID:                        t.ID,
		Name:                      t.Name,
		Description:               t.Description,
		Kind:                      t.Kind,
		Severity:                  t.Severity,
		TargetScope:               t.TargetScope,
		CooldownSeconds:           t.CooldownSeconds,
		ReopenAfterSeconds:        t.ReopenAfterSeconds,
		EvaluationIntervalSeconds: t.EvaluationInterval,
		WindowSeconds:             t.WindowSeconds,
		Condition:                 cloneAnyMap(t.Condition),
		Labels:                    cloneMetadata(t.Labels),
		Metadata:                  cloneMetadata(t.Metadata),
	}
}

func (t alertRuleTemplate) toCreateRequest(createdBy string) alerts.CreateRuleRequest {
	actor := strings.TrimSpace(createdBy)
	if actor == "" {
		actor = strings.TrimSpace(t.DefaultCreatedBy)
	}
	if actor == "" {
		actor = "owner"
	}

	metadata := cloneMetadata(t.Metadata)
	metadata["template_id"] = t.ID

	return alerts.CreateRuleRequest{
		Name:                      t.Name,
		Description:               t.Description,
		Status:                    alerts.RuleStatusActive,
		Kind:                      t.Kind,
		Severity:                  t.Severity,
		TargetScope:               t.TargetScope,
		CooldownSeconds:           t.CooldownSeconds,
		ReopenAfterSeconds:        t.ReopenAfterSeconds,
		EvaluationIntervalSeconds: t.EvaluationInterval,
		WindowSeconds:             t.WindowSeconds,
		Condition:                 cloneAnyMap(t.Condition),
		Labels:                    cloneMetadata(t.Labels),
		Metadata:                  metadata,
		CreatedBy:                 actor,
	}
}

type enableAlertTemplateRequest struct {
	CreatedBy   string                   `json:"created_by,omitempty"`
	Name        string                   `json:"name,omitempty"`
	Description string                   `json:"description,omitempty"`
	Targets     []alerts.RuleTargetInput `json:"targets,omitempty"`
}

func (d *Deps) HandleAlertTemplates(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/alerts/templates" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	templates := d.alertRuleTemplatesCatalog()
	response := make([]AlertRuleTemplateResponse, 0, len(templates))
	for _, template := range templates {
		response = append(response, template.toResponse())
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"templates": response})
}

func (d *Deps) HandleAlertTemplateActions(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/alerts/templates/") {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/alerts/templates/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "template action not found")
		return
	}

	templateID := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	if templateID == "" || action != "enable" {
		servicehttp.WriteError(w, http.StatusNotFound, "template action not found")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	template, ok := d.findAlertRuleTemplateByID(templateID)
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "alert template not found")
		return
	}

	if d.AlertStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "alert store unavailable")
		return
	}

	var req enableAlertTemplateRequest
	if err := decodeJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid template enable payload")
		return
	}

	createRequest := template.toCreateRequest(req.CreatedBy)
	if strings.TrimSpace(req.Name) != "" {
		createRequest.Name = req.Name
	}
	if strings.TrimSpace(req.Description) != "" {
		createRequest.Description = req.Description
	}
	if len(req.Targets) > 0 {
		createRequest.Targets = append([]alerts.RuleTargetInput(nil), req.Targets...)
	}
	NormalizeCreateAlertRuleRequest(&createRequest)
	if err := ValidateCreateAlertRuleRequest(createRequest); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := d.ValidateAlertRuleTargets(createRequest.Targets); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := d.findAlertRuleForTemplate(template.ID, createRequest.Targets)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to query existing alert rules")
		return
	}
	if existing != nil {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"template_id":     template.ID,
			"already_enabled": true,
			"rule":            existing,
		})
		return
	}

	createdRule, err := d.AlertStore.CreateAlertRule(createRequest)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to enable alert template")
		return
	}

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
		"template_id":     template.ID,
		"already_enabled": false,
		"rule":            createdRule,
	})
}

func (d *Deps) findAlertRuleForTemplate(templateID string, targets []alerts.RuleTargetInput) (*alerts.Rule, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" || d.AlertStore == nil {
		return nil, nil
	}

	rules, err := d.AlertStore.ListAlertRules(persistence.AlertRuleFilter{Limit: 500})
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		if !strings.EqualFold(strings.TrimSpace(rule.Metadata["template_id"]), templateID) {
			continue
		}
		if !alertRuleTargetsMatch(rule.Targets, targets) {
			continue
		}
		matchedRule := rule
		return &matchedRule, nil
	}
	return nil, nil
}

func alertRuleTargetsMatch(existing []alerts.RuleTarget, requested []alerts.RuleTargetInput) bool {
	if len(existing) != len(requested) {
		return false
	}
	if len(existing) == 0 && len(requested) == 0 {
		return true
	}

	existingKeys := make([]string, 0, len(existing))
	for _, target := range existing {
		existingKeys = append(existingKeys, alertRuleTargetKey(target.AssetID, target.GroupID, target.Selector))
	}

	requestedKeys := make([]string, 0, len(requested))
	for _, target := range requested {
		requestedKeys = append(requestedKeys, alertRuleTargetKey(target.AssetID, target.GroupID, target.Selector))
	}

	sort.Strings(existingKeys)
	sort.Strings(requestedKeys)
	for idx := range existingKeys {
		if existingKeys[idx] != requestedKeys[idx] {
			return false
		}
	}
	return true
}

func alertRuleTargetKey(assetID, groupID string, selector map[string]any) string {
	assetID = strings.TrimSpace(assetID)
	groupID = strings.TrimSpace(groupID)
	if assetID != "" {
		return "asset:" + assetID
	}
	if groupID != "" {
		return "group:" + groupID
	}
	if len(selector) == 0 {
		return ""
	}
	encoded, err := json.Marshal(selector)
	if err != nil {
		return "selector:"
	}
	return "selector:" + string(encoded)
}

func (d *Deps) alertRuleTemplatesCatalog() []alertRuleTemplate {
	return []alertRuleTemplate{
		{
			ID:                 "starter.agent_offline",
			Name:               "Agent Offline",
			Description:        "Alert when an asset stops sending heartbeats for five minutes.",
			Kind:               alerts.RuleKindHeartbeatStale,
			Severity:           alerts.SeverityCritical,
			TargetScope:        alerts.TargetScopeAsset,
			CooldownSeconds:    300,
			ReopenAfterSeconds: 120,
			EvaluationInterval: 30,
			WindowSeconds:      300,
			Condition: map[string]any{
				"max_stale_seconds": float64(300),
			},
			Labels: map[string]string{
				"starter": "true",
				"signal":  "offline",
			},
			Metadata: map[string]string{
				"category": "starter",
			},
			DefaultCreatedBy: "system",
		},
		{
			ID:                 "starter.cpu_saturation",
			Name:               "CPU Saturation",
			Description:        "Alert when CPU usage stays above 90 percent for five minutes.",
			Kind:               alerts.RuleKindMetricThreshold,
			Severity:           alerts.SeverityHigh,
			TargetScope:        alerts.TargetScopeAsset,
			CooldownSeconds:    300,
			ReopenAfterSeconds: 120,
			EvaluationInterval: 30,
			WindowSeconds:      300,
			Condition: map[string]any{
				"metric":    "cpu_used_percent",
				"operator":  ">=",
				"value":     float64(90),
				"aggregate": "avg",
			},
			Labels: map[string]string{
				"starter": "true",
				"signal":  "cpu",
			},
			Metadata: map[string]string{
				"category": "starter",
			},
			DefaultCreatedBy: "system",
		},
		{
			ID:                 "starter.memory_pressure",
			Name:               "Memory Pressure",
			Description:        "Alert when memory usage stays above 90 percent for five minutes.",
			Kind:               alerts.RuleKindMetricThreshold,
			Severity:           alerts.SeverityHigh,
			TargetScope:        alerts.TargetScopeAsset,
			CooldownSeconds:    300,
			ReopenAfterSeconds: 120,
			EvaluationInterval: 30,
			WindowSeconds:      300,
			Condition: map[string]any{
				"metric":    "memory_used_percent",
				"operator":  ">=",
				"value":     float64(90),
				"aggregate": "avg",
			},
			Labels: map[string]string{
				"starter": "true",
				"signal":  "memory",
			},
			Metadata: map[string]string{
				"category": "starter",
			},
			DefaultCreatedBy: "system",
		},
		{
			ID:                 "starter.disk_nearly_full",
			Name:               "Disk Nearly Full",
			Description:        "Alert when disk usage reaches 90 percent on an asset.",
			Kind:               alerts.RuleKindMetricThreshold,
			Severity:           alerts.SeverityCritical,
			TargetScope:        alerts.TargetScopeAsset,
			CooldownSeconds:    300,
			ReopenAfterSeconds: 120,
			EvaluationInterval: 30,
			WindowSeconds:      600,
			Condition: map[string]any{
				"metric":    "disk_used_percent",
				"operator":  ">=",
				"value":     float64(90),
				"aggregate": "max",
			},
			Labels: map[string]string{
				"starter": "true",
				"signal":  "disk",
			},
			Metadata: map[string]string{
				"category": "starter",
			},
			DefaultCreatedBy: "system",
		},
		{
			ID:                 "starter.error_burst",
			Name:               "Error Burst",
			Description:        "Alert when repeated error-pattern log lines appear in a short window.",
			Kind:               alerts.RuleKindLogPattern,
			Severity:           alerts.SeverityMedium,
			TargetScope:        alerts.TargetScopeGlobal,
			CooldownSeconds:    300,
			ReopenAfterSeconds: 120,
			EvaluationInterval: 30,
			WindowSeconds:      300,
			Condition: map[string]any{
				"pattern":         "ERROR|FATAL|panic",
				"min_occurrences": float64(10),
			},
			Labels: map[string]string{
				"starter": "true",
				"signal":  "errors",
			},
			Metadata: map[string]string{
				"category": "starter",
			},
			DefaultCreatedBy: "system",
		},
		{
			ID:                 "mobile.reconnect_storm",
			Name:               "Mobile Reconnect Storm",
			Description:        "High-frequency iOS realtime reconnect scheduling events in a short window.",
			Kind:               alerts.RuleKindLogPattern,
			Severity:           alerts.SeverityHigh,
			TargetScope:        alerts.TargetScopeGlobal,
			CooldownSeconds:    300,
			ReopenAfterSeconds: 120,
			EvaluationInterval: 30,
			WindowSeconds:      300,
			Condition: map[string]any{
				"pattern":         "mobile client telemetry metric",
				"source":          "mobile_client_telemetry",
				"min_occurrences": float64(25),
				"field_equals": map[string]any{
					"metric": "reconnect_scheduled",
				},
			},
			Labels: map[string]string{
				"channel": "mobile",
				"signal":  "reconnect",
			},
			Metadata: map[string]string{
				"category": "mobile_observability",
			},
			DefaultCreatedBy: "system",
		},
		{
			ID:                 "mobile.api_error_burst",
			Name:               "Mobile API Error Burst",
			Description:        "Burst of iOS API request telemetry events with error status.",
			Kind:               alerts.RuleKindLogPattern,
			Severity:           alerts.SeverityCritical,
			TargetScope:        alerts.TargetScopeGlobal,
			CooldownSeconds:    300,
			ReopenAfterSeconds: 120,
			EvaluationInterval: 30,
			WindowSeconds:      300,
			Condition: map[string]any{
				"pattern":         "mobile client telemetry metric",
				"source":          "mobile_client_telemetry",
				"min_occurrences": float64(20),
				"field_equals": map[string]any{
					"metric": "request.duration",
					"status": "error",
				},
			},
			Labels: map[string]string{
				"channel": "mobile",
				"signal":  "api_errors",
			},
			Metadata: map[string]string{
				"category": "mobile_observability",
			},
			DefaultCreatedBy: "system",
		},
		{
			ID:                 "services.web.down_transition_burst",
			Name:               "Service Down Transition Burst",
			Description:        "Multiple discovered services transitioned to down state within a short window.",
			Kind:               alerts.RuleKindLogPattern,
			Severity:           alerts.SeverityHigh,
			TargetScope:        alerts.TargetScopeGlobal,
			CooldownSeconds:    180,
			ReopenAfterSeconds: 120,
			EvaluationInterval: 30,
			WindowSeconds:      300,
			Condition: map[string]any{
				"pattern":         "web service status changed",
				"source":          d.WebServiceHealthLogSource,
				"min_occurrences": float64(3),
				"field_equals": map[string]any{
					"event_kind": d.WebServiceStatusTransitionKind,
					"status":     "down",
				},
			},
			Labels: map[string]string{
				"channel": "services",
				"signal":  "down_transition_burst",
			},
			Metadata: map[string]string{
				"category": "service_health",
			},
			DefaultCreatedBy: "system",
		},
		{
			ID:                 "services.web.uptime_drop",
			Name:               "Service Uptime Drop",
			Description:        "A discovered service crossed below the rolling uptime threshold.",
			Kind:               alerts.RuleKindLogPattern,
			Severity:           alerts.SeverityMedium,
			TargetScope:        alerts.TargetScopeGlobal,
			CooldownSeconds:    300,
			ReopenAfterSeconds: 120,
			EvaluationInterval: 30,
			WindowSeconds:      600,
			Condition: map[string]any{
				"pattern":         "web service rolling uptime dropped below threshold",
				"source":          d.WebServiceHealthLogSource,
				"min_occurrences": float64(1),
				"field_equals": map[string]any{
					"event_kind": d.WebServiceUptimeDropKind,
				},
			},
			Labels: map[string]string{
				"channel": "services",
				"signal":  "uptime_drop",
			},
			Metadata: map[string]string{
				"category":                  "service_health",
				"recommended_threshold_pct": strconv.FormatFloat(d.WebServiceUptimeDropThreshold, 'f', 0, 64),
			},
			DefaultCreatedBy: "system",
		},
	}
}

func (d *Deps) findAlertRuleTemplateByID(templateID string) (alertRuleTemplate, bool) {
	normalizedID := strings.TrimSpace(templateID)
	for _, template := range d.alertRuleTemplatesCatalog() {
		if template.ID == normalizedID {
			return template, true
		}
	}
	return alertRuleTemplate{}, false
}
