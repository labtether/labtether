package groupfeatures

import (
	"time"

	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/groups"
)

const (
	GroupOnlineWindow = 65 * time.Second
	GroupStaleWindow  = 5 * time.Minute
)

// GroupMaintenanceGuardrails summarises the active maintenance constraints for a group.
type GroupMaintenanceGuardrails struct {
	GroupID        string                               `json:"group_id,omitempty"`
	ActiveWindows  []groupmaintenance.MaintenanceWindow `json:"active_windows,omitempty"`
	SuppressAlerts bool                                 `json:"suppress_alerts"`
	BlockActions   bool                                 `json:"block_actions"`
	BlockUpdates   bool                                 `json:"block_updates"`
}

// GroupReliabilityRecord holds the computed reliability score and supporting metrics for a group.
type GroupReliabilityRecord struct {
	Group             groups.Group `json:"group"`
	Score             int          `json:"score"`
	Grade             string       `json:"grade"`
	AssetsTotal       int          `json:"assets_total"`
	AssetsOnline      int          `json:"assets_online"`
	AssetsStale       int          `json:"assets_stale"`
	AssetsOffline     int          `json:"assets_offline"`
	FailedActions     int          `json:"failed_actions"`
	FailedUpdates     int          `json:"failed_updates"`
	ErrorLogs         int          `json:"error_logs"`
	WarnLogs          int          `json:"warn_logs"`
	DeadLetters       int          `json:"dead_letters"`
	MaintenanceActive bool         `json:"maintenance_active"`
	SuppressAlerts    bool         `json:"suppress_alerts"`
	BlockActions      bool         `json:"block_actions"`
	BlockUpdates      bool         `json:"block_updates"`
}

// GroupTimelineEvent is a single event entry in a group's activity timeline.
type GroupTimelineEvent struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Severity  string    `json:"severity"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	Source    string    `json:"source,omitempty"`
	AssetID   string    `json:"asset_id,omitempty"`
	RunID     string    `json:"run_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// GroupTimelineImpact aggregates impact counters across a timeline window.
type GroupTimelineImpact struct {
	TotalEvents   int `json:"total_events"`
	ErrorEvents   int `json:"error_events"`
	WarnEvents    int `json:"warn_events"`
	InfoEvents    int `json:"info_events"`
	FailedActions int `json:"failed_actions"`
	FailedUpdates int `json:"failed_updates"`
	AssetsStale   int `json:"assets_stale"`
	AssetsOffline int `json:"assets_offline"`
	DeadLetters   int `json:"dead_letters"`
}
