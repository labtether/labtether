package model

import "time"

type ProviderKind string

const (
	ProviderKindAgent     ProviderKind = "agent"
	ProviderKindConnector ProviderKind = "connector"
)

type ProviderStatus string

const (
	ProviderStatusHealthy  ProviderStatus = "healthy"
	ProviderStatusDegraded ProviderStatus = "degraded"
	ProviderStatusOffline  ProviderStatus = "offline"
	ProviderStatusUnknown  ProviderStatus = "unknown"
)

type ProviderScope string

const (
	ProviderScopeGlobal ProviderScope = "global"
	// Provider instances scoped to a specific group/site are persisted as
	// "site" to match the canonical-model schema and existing migrations.
	ProviderScopeGroup ProviderScope = "site"
)

type ProviderInstance struct {
	ID          string         `json:"id"`
	Kind        ProviderKind   `json:"kind"`
	Provider    string         `json:"provider"`
	DisplayName string         `json:"display_name"`
	Version     string         `json:"version,omitempty"`
	Status      ProviderStatus `json:"status"`
	Scope       ProviderScope  `json:"scope"`
	ConfigRef   string         `json:"config_ref,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	LastSeenAt  time.Time      `json:"last_seen_at"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
