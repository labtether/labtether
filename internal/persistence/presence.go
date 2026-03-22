package persistence

import "time"

// AgentPresence represents a connected agent's presence record.
type AgentPresence struct {
	AssetID         string         `json:"asset_id"`
	Transport       string         `json:"transport"`
	ConnectedAt     time.Time      `json:"connected_at"`
	LastHeartbeatAt time.Time      `json:"last_heartbeat_at"`
	SessionID       string         `json:"session_id"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// PresenceStore manages agent presence records.
type PresenceStore interface {
	UpsertPresence(p AgentPresence) error
	UpdateHeartbeat(assetID string, at time.Time) error
	UpdateHeartbeatForSession(assetID, sessionID string, at time.Time) (bool, error)
	UpdatePresenceMetadata(assetID string, metadata map[string]any) error
	UpdatePresenceMetadataForSession(assetID, sessionID string, metadata map[string]any) (bool, error)
	DeletePresence(assetID string) error
	DeletePresenceForSession(assetID, sessionID string) (bool, error)
	ListPresence() ([]AgentPresence, error)
	GetStalePresence(olderThan time.Time) ([]AgentPresence, error)
	UpdateAssetTransportType(assetID, transportType string) error
}
