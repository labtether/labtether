package assets

import "time"

const (
	// MetadataKeyNameOverride stores a user-managed display name that should not
	// be replaced by heartbeat-reported names.
	MetadataKeyNameOverride = "name_override"
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
}

// UpdateRequest applies partial updates to editable asset fields.
type UpdateRequest struct {
	Name    *string   `json:"name,omitempty"`
	GroupID *string   `json:"group_id,omitempty"`
	Tags    *[]string `json:"tags,omitempty"`
}
