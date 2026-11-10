package alerting

import (
	"github.com/labtether/labtether/internal/alerts"
	"strconv"
	"strings"
)

func (d *Deps) alertRuleTemplatesCatalog() []alertRuleTemplate {
	return []alertRuleTemplate{
		{
			ID:                 "starter.agent_offline",
			Name:               "Agent offline",
			Description:        "No heartbeat from a device for 5 minutes.",
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
			Name:               "High CPU",
			Description:        "CPU at 90% or more for 5 minutes.",
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
			Name:               "High memory",
			Description:        "Memory at 90% or more for 5 minutes.",
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
			Name:               "Disk nearly full",
			Description:        "A disk at 90% or more.",
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
			Name:               "Error log lines",
			Description:        "10 or more ERROR, FATAL or panic log lines in 5 minutes.",
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
			Name:               "Mobile app reconnects",
			Description:        "25 or more reconnects from the mobile app in 5 minutes.",
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
			Name:               "Mobile app errors",
			Description:        "20 or more failed requests from the mobile app in 5 minutes.",
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
			Name:               "Services going down",
			Description:        "3 or more services went down in 5 minutes.",
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
			Name:               "Service uptime drop",
			Description:        "A service's rolling uptime fell below " + strconv.FormatFloat(d.WebServiceUptimeDropThreshold, 'f', 0, 64) + "%.",
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
