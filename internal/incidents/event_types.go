package incidents

import (
	"strings"
	"time"
)

const (
	EventTypeMetricAnomaly   = "metric_anomaly"
	EventTypeLogBurst        = "log_burst"
	EventTypeActionRun       = "action_run"
	EventTypeUpdateRun       = "update_run"
	EventTypeAlertFired      = "alert_fired"
	EventTypeAlertResolved   = "alert_resolved"
	EventTypeConfigChange    = "config_change"
	EventTypeAudit           = "audit"
	EventTypeHeartbeatChange = "heartbeat_change"
)

type IncidentEvent struct {
	ID         string         `json:"id"`
	IncidentID string         `json:"incident_id"`
	EventType  string         `json:"event_type"`
	SourceRef  string         `json:"source_ref"`
	Summary    string         `json:"summary"`
	Severity   string         `json:"severity,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
	CreatedAt  time.Time      `json:"created_at"`
}

type CreateIncidentEventRequest struct {
	IncidentID string         `json:"incident_id"`
	EventType  string         `json:"event_type"`
	SourceRef  string         `json:"source_ref"`
	Summary    string         `json:"summary"`
	Severity   string         `json:"severity,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
}

func NormalizeEventType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case EventTypeMetricAnomaly:
		return EventTypeMetricAnomaly
	case EventTypeLogBurst:
		return EventTypeLogBurst
	case EventTypeActionRun:
		return EventTypeActionRun
	case EventTypeUpdateRun:
		return EventTypeUpdateRun
	case EventTypeAlertFired:
		return EventTypeAlertFired
	case EventTypeAlertResolved:
		return EventTypeAlertResolved
	case EventTypeConfigChange:
		return EventTypeConfigChange
	case EventTypeAudit:
		return EventTypeAudit
	case EventTypeHeartbeatChange:
		return EventTypeHeartbeatChange
	default:
		return ""
	}
}
