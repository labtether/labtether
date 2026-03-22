package shared

import (
	"time"

	"github.com/labtether/labtether/internal/retention"
	"github.com/labtether/labtether/internal/telemetry"
)

type TelemetrySnapshot = telemetry.Snapshot

type AssetTelemetryOverview struct {
	AssetID        string             `json:"asset_id"`
	Name           string             `json:"name"`
	Type           string             `json:"type"`
	Source         string             `json:"source"`
	GroupID        string             `json:"group_id,omitempty"`
	Status         string             `json:"status"`
	Platform       string             `json:"platform,omitempty"`
	LastSeenAt     time.Time          `json:"last_seen_at"`
	Metrics        TelemetrySnapshot  `json:"metrics"`
	DynamicMetrics map[string]float64 `json:"dynamic_metrics,omitempty"`
}

type RetentionSettingsRequest struct {
	Preset           string `json:"preset,omitempty"`
	LogsWindow       string `json:"logs_window,omitempty"`
	MetricsWindow    string `json:"metrics_window,omitempty"`
	AuditWindow      string `json:"audit_window,omitempty"`
	TerminalWindow   string `json:"terminal_window,omitempty"`
	ActionRunsWindow string `json:"action_runs_window,omitempty"`
	UpdateRunsWindow string `json:"update_runs_window,omitempty"`
	RecordingsWindow string `json:"recordings_window,omitempty"`
}

type RetentionSettingsResponse struct {
	LogsWindow       string `json:"logs_window"`
	MetricsWindow    string `json:"metrics_window"`
	AuditWindow      string `json:"audit_window"`
	TerminalWindow   string `json:"terminal_window"`
	ActionRunsWindow string `json:"action_runs_window"`
	UpdateRunsWindow string `json:"update_runs_window"`
	RecordingsWindow string `json:"recordings_window"`
}

type RetentionPresetResponse struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Settings    RetentionSettingsResponse `json:"settings"`
}

type RuntimeSettingsUpdateRequest struct {
	Values map[string]string `json:"values"`
}

type RuntimeSettingsResetRequest struct {
	Keys []string `json:"keys"`
}

type RuntimeSettingEntry struct {
	Key            string   `json:"key"`
	Label          string   `json:"label"`
	Description    string   `json:"description"`
	Scope          string   `json:"scope"`
	Type           string   `json:"type"`
	EnvVar         string   `json:"env_var"`
	DefaultValue   string   `json:"default_value"`
	EnvValue       string   `json:"env_value,omitempty"`
	OverrideValue  string   `json:"override_value,omitempty"`
	EffectiveValue string   `json:"effective_value"`
	Source         string   `json:"source"`
	AllowedValues  []string `json:"allowed_values,omitempty"`
	MinInt         int      `json:"min_int,omitempty"`
	MaxInt         int      `json:"max_int,omitempty"`
}

type RuntimeSettingsPayload struct {
	Settings  []RuntimeSettingEntry `json:"settings"`
	Overrides map[string]string     `json:"overrides"`
}

func FormatRetentionSettings(settings retention.Settings) RetentionSettingsResponse {
	normalized := retention.Normalize(settings)
	return RetentionSettingsResponse{
		LogsWindow:       retention.FormatDuration(normalized.LogsWindow),
		MetricsWindow:    retention.FormatDuration(normalized.MetricsWindow),
		AuditWindow:      retention.FormatDuration(normalized.AuditWindow),
		TerminalWindow:   retention.FormatDuration(normalized.TerminalWindow),
		ActionRunsWindow: retention.FormatDuration(normalized.ActionRunsWindow),
		UpdateRunsWindow: retention.FormatDuration(normalized.UpdateRunsWindow),
		RecordingsWindow: retention.FormatDuration(normalized.RecordingsWindow),
	}
}

func FormatRetentionPresets() []RetentionPresetResponse {
	presets := retention.Presets()
	out := make([]RetentionPresetResponse, 0, len(presets))
	for _, preset := range presets {
		out = append(out, RetentionPresetResponse{
			ID:          preset.ID,
			Name:        preset.Name,
			Description: preset.Description,
			Settings:    FormatRetentionSettings(preset.Settings),
		})
	}
	return out
}
