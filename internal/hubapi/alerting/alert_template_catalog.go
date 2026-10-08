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
