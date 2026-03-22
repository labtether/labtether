package operations

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/retention"
)

// RetentionState tracks the state of the retention pruning loop.
type RetentionState struct {
	Mu       sync.RWMutex
	LastRun  retention.PruneResult
	LastErr  string
	Settings retention.Settings
}

// RunRetentionLoop periodically prunes expired data according to retention settings.
func RunRetentionLoop(ctx context.Context, retentionStore persistence.RetentionStore, runtimeState *WorkerRuntimeState, tracker *RetentionState) {
	if retentionStore == nil {
		return
	}

	ApplyRetentionCycle(retentionStore, tracker)

	for {
		interval := 5 * time.Minute
		if runtimeState != nil {
			interval = runtimeState.RetentionInterval()
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Printf("worker retention loop stopped")
			return
		case <-timer.C:
			ApplyRetentionCycle(retentionStore, tracker)
		}
	}
}

// ApplyRetentionCycle runs a single retention prune cycle.
func ApplyRetentionCycle(retentionStore persistence.RetentionStore, tracker *RetentionState) {
	if retentionStore == nil {
		return
	}
	now := time.Now().UTC()

	settings, err := retentionStore.GetRetentionSettings()
	if err != nil {
		log.Printf("worker retention read failed: %v", err)
		if tracker != nil {
			tracker.Mu.Lock()
			tracker.LastErr = err.Error()
			tracker.Mu.Unlock()
		}
		return
	}
	settings = retention.Normalize(settings)

	result, err := retentionStore.PruneExpiredData(now, settings)
	if err != nil {
		log.Printf("worker retention prune failed: %v", err)
		if tracker != nil {
			tracker.Mu.Lock()
			tracker.Settings = settings
			tracker.LastErr = err.Error()
			tracker.Mu.Unlock()
		}
		return
	}
	if result.RanAt.IsZero() {
		result.RanAt = now
	}

	if tracker != nil {
		tracker.Mu.Lock()
		tracker.Settings = settings
		tracker.LastRun = result
		tracker.LastErr = ""
		tracker.Mu.Unlock()
	}

	if result.TotalDeleted() > 0 {
		log.Printf("worker retention prune completed: deleted=%d logs=%d metrics=%d audit=%d terminal_commands=%d terminal_sessions=%d action_runs=%d update_runs=%d alert_instances=%d alert_evaluations=%d notification_history=%d alert_silences=%d recordings=%d",
			result.TotalDeleted(),
			result.LogsDeleted,
			result.MetricsDeleted,
			result.AuditDeleted,
			result.TerminalCommandsDeleted,
			result.TerminalSessionsDeleted,
			result.ActionRunsDeleted,
			result.UpdateRunsDeleted,
			result.AlertInstancesDeleted,
			result.AlertEvaluationsDeleted,
			result.NotificationHistoryDeleted,
			result.AlertSilencesDeleted,
			result.RecordingsDeleted,
		)
	}
}

// FormatRetentionSettings formats retention settings for API display.
func FormatRetentionSettings(settings retention.Settings) map[string]string {
	normalized := retention.Normalize(settings)
	return map[string]string{
		"logs_window":        retention.FormatDuration(normalized.LogsWindow),
		"metrics_window":     retention.FormatDuration(normalized.MetricsWindow),
		"audit_window":       retention.FormatDuration(normalized.AuditWindow),
		"terminal_window":    retention.FormatDuration(normalized.TerminalWindow),
		"action_runs_window": retention.FormatDuration(normalized.ActionRunsWindow),
		"update_runs_window": retention.FormatDuration(normalized.UpdateRunsWindow),
		"recordings_window":  retention.FormatDuration(normalized.RecordingsWindow),
	}
}
