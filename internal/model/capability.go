package model

import "time"

type CapabilityScope string

const (
	CapabilityScopeRead   CapabilityScope = "read"
	CapabilityScopeAction CapabilityScope = "action"
	CapabilityScopeStream CapabilityScope = "stream"
	CapabilityScopeAdmin  CapabilityScope = "admin"
)

type CapabilityStability string

const (
	CapabilityStabilityGA           CapabilityStability = "ga"
	CapabilityStabilityBeta         CapabilityStability = "beta"
	CapabilityStabilityExperimental CapabilityStability = "experimental"
)

type CapabilitySpec struct {
	ID             string              `json:"id"`
	Scope          CapabilityScope     `json:"scope"`
	Stability      CapabilityStability `json:"stability,omitempty"`
	SupportsDryRun bool                `json:"supports_dry_run,omitempty"`
	SupportsAsync  bool                `json:"supports_async,omitempty"`
	RequiresTarget bool                `json:"requires_target,omitempty"`
	ParamsSchema   map[string]any      `json:"params_schema,omitempty"`
}

type CapabilitySet struct {
	SubjectType  string           `json:"subject_type"`
	SubjectID    string           `json:"subject_id"`
	Capabilities []CapabilitySpec `json:"capabilities"`
	UpdatedAt    time.Time        `json:"updated_at"`
}
