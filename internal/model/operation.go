package model

import "time"

type OperationSafetyLevel string

const (
	OperationSafetySafe        OperationSafetyLevel = "safe"
	OperationSafetyDisruptive  OperationSafetyLevel = "disruptive"
	OperationSafetyDestructive OperationSafetyLevel = "destructive"
)

type OperationStatus string

const (
	OperationStatusQueued    OperationStatus = "queued"
	OperationStatusRunning   OperationStatus = "running"
	OperationStatusSucceeded OperationStatus = "succeeded"
	OperationStatusFailed    OperationStatus = "failed"
	OperationStatusPartial   OperationStatus = "partial"
	OperationStatusCancelled OperationStatus = "cancelled"
	OperationStatusTimedOut  OperationStatus = "timed_out"
)

type OperationParameter struct {
	Key         string         `json:"key"`
	Label       string         `json:"label"`
	Required    bool           `json:"required"`
	Type        string         `json:"type,omitempty"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
}

type OperationDescriptor struct {
	ID                 string               `json:"id"`
	DisplayName        string               `json:"display_name"`
	Description        string               `json:"description,omitempty"`
	TargetKind         string               `json:"target_kind,omitempty"`
	CapabilityRequired string               `json:"capability_required,omitempty"`
	SafetyLevel        OperationSafetyLevel `json:"safety_level,omitempty"`
	SupportsDryRun     bool                 `json:"supports_dry_run"`
	IsIdempotent       bool                 `json:"is_idempotent,omitempty"`
	SupportsAsync      bool                 `json:"supports_async,omitempty"`
	RequiresTarget     bool                 `json:"requires_target,omitempty"`
	Parameters         []OperationParameter `json:"parameters,omitempty"`
}

type OperationRequest struct {
	ID               string         `json:"id"`
	OperationID      string         `json:"operation_id"`
	TargetResourceID string         `json:"target_resource_id,omitempty"`
	Params           map[string]any `json:"params,omitempty"`
	DryRun           bool           `json:"dry_run,omitempty"`
	ActorID          string         `json:"actor_id,omitempty"`
	IdempotencyKey   string         `json:"idempotency_key,omitempty"`
	RequestedAt      time.Time      `json:"requested_at"`
}

type OperationStep struct {
	Name      string          `json:"name"`
	Status    OperationStatus `json:"status"`
	Summary   string          `json:"summary,omitempty"`
	ErrorCode string          `json:"error_code,omitempty"`
	Error     string          `json:"error,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
	StartedAt *time.Time      `json:"started_at,omitempty"`
	EndedAt   *time.Time      `json:"ended_at,omitempty"`
}

type OperationExecution struct {
	ID                  string          `json:"id"`
	RequestID           string          `json:"request_id"`
	ProviderInstanceID  string          `json:"provider_instance_id,omitempty"`
	ProviderOperationID string          `json:"provider_operation_id,omitempty"`
	Status              OperationStatus `json:"status"`
	Summary             string          `json:"summary,omitempty"`
	ErrorCode           string          `json:"error_code,omitempty"`
	ErrorMessage        string          `json:"error_message,omitempty"`
	Metadata            map[string]any  `json:"metadata,omitempty"`
	Steps               []OperationStep `json:"steps,omitempty"`
	StartedAt           *time.Time      `json:"started_at,omitempty"`
	FinishedAt          *time.Time      `json:"finished_at,omitempty"`
}
