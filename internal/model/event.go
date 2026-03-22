package model

import "time"

type EventKind string

const (
	EventKindLog         EventKind = "log"
	EventKindStateChange EventKind = "state_change"
	EventKindOperation   EventKind = "operation"
	EventKindAlert       EventKind = "alert"
	EventKindIncident    EventKind = "incident"
	EventKindTopology    EventKind = "topology"
	EventKindConnector   EventKind = "connector"
)

type EventSeverity string

const (
	EventSeverityDebug    EventSeverity = "debug"
	EventSeverityInfo     EventSeverity = "info"
	EventSeverityWarn     EventSeverity = "warn"
	EventSeverityError    EventSeverity = "error"
	EventSeverityCritical EventSeverity = "critical"
)

type Event struct {
	ID                 string         `json:"id"`
	Kind               EventKind      `json:"kind"`
	ResourceID         string         `json:"resource_id,omitempty"`
	ProviderInstanceID string         `json:"provider_instance_id,omitempty"`
	Severity           EventSeverity  `json:"severity"`
	Title              string         `json:"title"`
	Message            string         `json:"message"`
	Attributes         map[string]any `json:"attributes,omitempty"`
	Fingerprint        string         `json:"fingerprint,omitempty"`
	CorrelationID      string         `json:"correlation_id,omitempty"`
	OccurredAt         time.Time      `json:"occurred_at"`
	IngestedAt         time.Time      `json:"ingested_at"`
}

type EventDescriptor struct {
	ID          string    `json:"id"`
	Kind        EventKind `json:"kind"`
	Severity    []string  `json:"severity,omitempty"`
	Description string    `json:"description,omitempty"`
	TargetKinds []string  `json:"target_kinds,omitempty"`
	Attributes  []string  `json:"attributes,omitempty"`
}
