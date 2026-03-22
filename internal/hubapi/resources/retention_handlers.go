package resources

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/retention"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleRetentionSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/retention" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.RetentionStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "retention settings unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		settings, err := d.RetentionStore.GetRetentionSettings()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load retention settings")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"settings": shared.FormatRetentionSettings(settings),
			"presets":  shared.FormatRetentionPresets(),
		})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "settings.retention.update", 30, time.Minute) {
			return
		}
		var req shared.RetentionSettingsRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid retention settings payload")
			return
		}

		next, err := resolveRetentionSettings(d.RetentionStore, req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		saved, err := d.RetentionStore.SaveRetentionSettings(next)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save retention settings")
			return
		}

		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"settings": shared.FormatRetentionSettings(saved),
			"presets":  shared.FormatRetentionPresets(),
		})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func resolveRetentionSettings(store persistence.RetentionStore, req shared.RetentionSettingsRequest) (retention.Settings, error) {
	if preset := strings.TrimSpace(req.Preset); preset != "" {
		resolved, ok := retention.ResolvePreset(preset)
		if !ok {
			return retention.Settings{}, fmt.Errorf("unknown retention preset: %s", preset)
		}
		return resolved, nil
	}

	current, err := store.GetRetentionSettings()
	if err != nil {
		return retention.Settings{}, err
	}

	next := current
	if req.LogsWindow != "" {
		value, err := retention.ParseDuration(req.LogsWindow)
		if err != nil {
			return retention.Settings{}, fmt.Errorf("invalid logs_window: %w", err)
		}
		next.LogsWindow = value
	}
	if req.MetricsWindow != "" {
		value, err := retention.ParseDuration(req.MetricsWindow)
		if err != nil {
			return retention.Settings{}, fmt.Errorf("invalid metrics_window: %w", err)
		}
		next.MetricsWindow = value
	}
	if req.AuditWindow != "" {
		value, err := retention.ParseDuration(req.AuditWindow)
		if err != nil {
			return retention.Settings{}, fmt.Errorf("invalid audit_window: %w", err)
		}
		next.AuditWindow = value
	}
	if req.TerminalWindow != "" {
		value, err := retention.ParseDuration(req.TerminalWindow)
		if err != nil {
			return retention.Settings{}, fmt.Errorf("invalid terminal_window: %w", err)
		}
		next.TerminalWindow = value
	}
	if req.ActionRunsWindow != "" {
		value, err := retention.ParseDuration(req.ActionRunsWindow)
		if err != nil {
			return retention.Settings{}, fmt.Errorf("invalid action_runs_window: %w", err)
		}
		next.ActionRunsWindow = value
	}
	if req.UpdateRunsWindow != "" {
		value, err := retention.ParseDuration(req.UpdateRunsWindow)
		if err != nil {
			return retention.Settings{}, fmt.Errorf("invalid update_runs_window: %w", err)
		}
		next.UpdateRunsWindow = value
	}
	if req.RecordingsWindow != "" {
		value, err := retention.ParseDuration(req.RecordingsWindow)
		if err != nil {
			return retention.Settings{}, fmt.Errorf("invalid recordings_window: %w", err)
		}
		next.RecordingsWindow = value
	}

	return retention.Normalize(next), nil
}
