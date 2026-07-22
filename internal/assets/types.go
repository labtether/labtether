package assets

import "time"

const (
	// MetadataKeyNameOverride stores a user-managed display name that should not
	// be replaced by heartbeat-reported names.
	MetadataKeyNameOverride = "name_override"

	// These fields form the agent device-identity trust-on-first-use anchor.
	// Routine heartbeats are bearer-authenticated but do not carry a fresh
	// device-key signature, so once populated these values must be immutable
	// unless a separately verified enrollment flow explicitly rotates them.
	MetadataKeyAgentDeviceFingerprint  = "agent_device_fingerprint"
	MetadataKeyAgentDeviceKeyAlgorithm = "agent_device_key_alg"
	MetadataKeyAgentIdentityVerifiedAt = "agent_identity_verified_at"
)

// Asset represents an inventory object tracked by LabTether.
type Asset struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	Name          string            `json:"name"`
	Source        string            `json:"source"`
	Tags          []string          `json:"tags,omitempty"`
	GroupID       string            `json:"group_id,omitempty"`
	Status        string            `json:"status"`
	Platform      string            `json:"platform,omitempty"`
	ResourceClass string            `json:"resource_class,omitempty"`
	ResourceKind  string            `json:"resource_kind,omitempty"`
	Host          string            `json:"host,omitempty"`
	TransportType string            `json:"transport_type,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Attributes    map[string]any    `json:"attributes,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	LastSeenAt    time.Time         `json:"last_seen_at"`
}

// HeartbeatRequest is the payload used to refresh asset liveness/state.
type HeartbeatRequest struct {
	AssetID  string            `json:"asset_id"`
	Type     string            `json:"type"`
	Name     string            `json:"name"`
	Source   string            `json:"source"`
	GroupID  string            `json:"group_id,omitempty"`
	Status   string            `json:"status,omitempty"`
	Platform string            `json:"platform,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`

	// AllowAgentIdentityRotation is internal-only and must be set exclusively by
	// a flow that has separately verified possession of the replacement device
	// private key. It is intentionally unavailable to JSON heartbeat clients.
	AllowAgentIdentityRotation bool `json:"-"`

	// AllowAgentIdentityTOFU is internal-only and is set by the persistence
	// transaction after it validates an active, asset-bound agent token. Generic
	// admin/API-key and owner-token heartbeats must leave it false.
	AllowAgentIdentityTOFU bool `json:"-"`
}

// UpdateRequest applies partial updates to editable asset fields.
type UpdateRequest struct {
	Name    *string   `json:"name,omitempty"`
	GroupID *string   `json:"group_id,omitempty"`
	Tags    *[]string `json:"tags,omitempty"`
}
