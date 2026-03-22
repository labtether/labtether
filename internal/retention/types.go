package retention

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Settings defines retention windows per data domain.
type Settings struct {
	LogsWindow                time.Duration
	MetricsWindow             time.Duration
	AuditWindow               time.Duration
	TerminalWindow            time.Duration
	ActionRunsWindow          time.Duration
	UpdateRunsWindow          time.Duration
	AlertInstancesWindow      time.Duration
	AlertEvaluationsWindow    time.Duration
	NotificationHistoryWindow time.Duration
	AlertSilencesWindow       time.Duration
	RecordingsWindow          time.Duration
}

// PruneResult summarizes delete counts for a retention cycle.
type PruneResult struct {
	RanAt                      time.Time `json:"ran_at"`
	LogsDeleted                int64     `json:"logs_deleted"`
	MetricsDeleted             int64     `json:"metrics_deleted"`
	AuditDeleted               int64     `json:"audit_deleted"`
	TerminalCommandsDeleted    int64     `json:"terminal_commands_deleted"`
	TerminalSessionsDeleted    int64     `json:"terminal_sessions_deleted"`
	ActionRunsDeleted          int64     `json:"action_runs_deleted"`
	UpdateRunsDeleted          int64     `json:"update_runs_deleted"`
	AlertInstancesDeleted      int64     `json:"alert_instances_deleted"`
	AlertEvaluationsDeleted    int64     `json:"alert_evaluations_deleted"`
	NotificationHistoryDeleted int64     `json:"notification_history_deleted"`
	AlertSilencesDeleted       int64     `json:"alert_silences_deleted"`
	RecordingsDeleted          int64     `json:"recordings_deleted"`
}

func (r PruneResult) TotalDeleted() int64 {
	return r.LogsDeleted + r.MetricsDeleted + r.AuditDeleted + r.TerminalCommandsDeleted + r.TerminalSessionsDeleted + r.ActionRunsDeleted + r.UpdateRunsDeleted + r.AlertInstancesDeleted + r.AlertEvaluationsDeleted + r.NotificationHistoryDeleted + r.AlertSilencesDeleted + r.RecordingsDeleted
}

func DefaultSettings() Settings {
	return Settings{
		LogsWindow:                7 * 24 * time.Hour,
		MetricsWindow:             14 * 24 * time.Hour,
		AuditWindow:               180 * 24 * time.Hour,
		TerminalWindow:            30 * 24 * time.Hour,
		ActionRunsWindow:          30 * 24 * time.Hour,
		UpdateRunsWindow:          90 * 24 * time.Hour,
		AlertInstancesWindow:      90 * 24 * time.Hour,
		AlertEvaluationsWindow:    30 * 24 * time.Hour,
		NotificationHistoryWindow: 60 * 24 * time.Hour,
		AlertSilencesWindow:       30 * 24 * time.Hour,
		RecordingsWindow:          30 * 24 * time.Hour,
	}
}

func Normalize(settings Settings) Settings {
	defaults := DefaultSettings()

	if settings.LogsWindow <= 0 {
		settings.LogsWindow = defaults.LogsWindow
	}
	if settings.MetricsWindow <= 0 {
		settings.MetricsWindow = defaults.MetricsWindow
	}
	if settings.AuditWindow <= 0 {
		settings.AuditWindow = defaults.AuditWindow
	}
	if settings.TerminalWindow <= 0 {
		settings.TerminalWindow = defaults.TerminalWindow
	}
	if settings.ActionRunsWindow <= 0 {
		settings.ActionRunsWindow = defaults.ActionRunsWindow
	}
	if settings.UpdateRunsWindow <= 0 {
		settings.UpdateRunsWindow = defaults.UpdateRunsWindow
	}
	if settings.AlertInstancesWindow <= 0 {
		settings.AlertInstancesWindow = defaults.AlertInstancesWindow
	}
	if settings.AlertEvaluationsWindow <= 0 {
		settings.AlertEvaluationsWindow = defaults.AlertEvaluationsWindow
	}
	if settings.NotificationHistoryWindow <= 0 {
		settings.NotificationHistoryWindow = defaults.NotificationHistoryWindow
	}
	if settings.AlertSilencesWindow <= 0 {
		settings.AlertSilencesWindow = defaults.AlertSilencesWindow
	}
	if settings.RecordingsWindow <= 0 {
		settings.RecordingsWindow = defaults.RecordingsWindow
	}

	return settings
}

func FormatDuration(value time.Duration) string {
	if value%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int64(value/(24*time.Hour)))
	}
	return value.String()
}

func ParseDuration(raw string) (time.Duration, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return 0, fmt.Errorf("duration is required")
	}

	if strings.HasSuffix(trimmed, "w") {
		base := strings.TrimSpace(strings.TrimSuffix(trimmed, "w"))
		parsed, err := strconv.Atoi(base)
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("invalid week duration: %s", raw)
		}
		return time.Duration(parsed) * 7 * 24 * time.Hour, nil
	}

	if strings.HasSuffix(trimmed, "d") {
		base := strings.TrimSpace(strings.TrimSuffix(trimmed, "d"))
		parsed, err := strconv.Atoi(base)
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("invalid day duration: %s", raw)
		}
		return time.Duration(parsed) * 24 * time.Hour, nil
	}

	duration, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return duration, nil
}

type Preset struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Settings    Settings `json:"settings"`
}

func Presets() []Preset {
	return []Preset{
		{
			ID:          "compact",
			Name:        "Compact",
			Description: "Lower storage footprint for small homelabs.",
			Settings: Settings{
				LogsWindow:                3 * 24 * time.Hour,
				MetricsWindow:             7 * 24 * time.Hour,
				AuditWindow:               90 * 24 * time.Hour,
				TerminalWindow:            14 * 24 * time.Hour,
				ActionRunsWindow:          14 * 24 * time.Hour,
				UpdateRunsWindow:          30 * 24 * time.Hour,
				AlertInstancesWindow:      30 * 24 * time.Hour,
				AlertEvaluationsWindow:    14 * 24 * time.Hour,
				NotificationHistoryWindow: 30 * 24 * time.Hour,
				AlertSilencesWindow:       14 * 24 * time.Hour,
				RecordingsWindow:          14 * 24 * time.Hour,
			},
		},
		{
			ID:          "balanced",
			Name:        "Balanced",
			Description: "Default retention profile for most installations.",
			Settings:    DefaultSettings(),
		},
		{
			ID:          "extended",
			Name:        "Extended",
			Description: "Longer lookback for deep incident forensics.",
			Settings: Settings{
				LogsWindow:                30 * 24 * time.Hour,
				MetricsWindow:             30 * 24 * time.Hour,
				AuditWindow:               365 * 24 * time.Hour,
				TerminalWindow:            90 * 24 * time.Hour,
				ActionRunsWindow:          90 * 24 * time.Hour,
				UpdateRunsWindow:          180 * 24 * time.Hour,
				AlertInstancesWindow:      180 * 24 * time.Hour,
				AlertEvaluationsWindow:    90 * 24 * time.Hour,
				NotificationHistoryWindow: 180 * 24 * time.Hour,
				AlertSilencesWindow:       90 * 24 * time.Hour,
				RecordingsWindow:          90 * 24 * time.Hour,
			},
		},
	}
}

func ResolvePreset(id string) (Settings, bool) {
	needle := strings.TrimSpace(strings.ToLower(id))
	for _, preset := range Presets() {
		if strings.ToLower(preset.ID) == needle {
			return preset.Settings, true
		}
	}
	return Settings{}, false
}
