package persistence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/retention"
	"log"
	"os"
	"strings"
	"time"
)

func (s *PostgresStore) GetRetentionSettings() (retention.Settings, error) {
	defaults := retention.DefaultSettings()
	row := s.pool.QueryRow(context.Background(),
		`SELECT logs_window, metrics_window, audit_window, terminal_window, action_runs_window, update_runs_window, recordings_window
		 FROM retention_settings
		 WHERE id = 'global'`,
	)

	var logsWindow string
	var metricsWindow string
	var auditWindow string
	var terminalWindow string
	var actionRunsWindow string
	var updateRunsWindow string
	var recordingsWindow string
	if err := row.Scan(
		&logsWindow,
		&metricsWindow,
		&auditWindow,
		&terminalWindow,
		&actionRunsWindow,
		&updateRunsWindow,
		&recordingsWindow,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaults, nil
		}
		return retention.Settings{}, err
	}

	settings := retention.Settings{
		LogsWindow:       parseRetentionDuration(logsWindow, defaults.LogsWindow),
		MetricsWindow:    parseRetentionDuration(metricsWindow, defaults.MetricsWindow),
		AuditWindow:      parseRetentionDuration(auditWindow, defaults.AuditWindow),
		TerminalWindow:   parseRetentionDuration(terminalWindow, defaults.TerminalWindow),
		ActionRunsWindow: parseRetentionDuration(actionRunsWindow, defaults.ActionRunsWindow),
		UpdateRunsWindow: parseRetentionDuration(updateRunsWindow, defaults.UpdateRunsWindow),
		RecordingsWindow: parseRetentionDuration(recordingsWindow, defaults.RecordingsWindow),
	}

	return retention.Normalize(settings), nil
}

func (s *PostgresStore) SaveRetentionSettings(settings retention.Settings) (retention.Settings, error) {
	normalized := retention.Normalize(settings)
	now := time.Now().UTC()

	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO retention_settings (id, logs_window, metrics_window, audit_window, terminal_window, action_runs_window, update_runs_window, recordings_window, updated_at)
		 VALUES ('global', $1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (id) DO UPDATE
		 SET logs_window = EXCLUDED.logs_window,
		     metrics_window = EXCLUDED.metrics_window,
		     audit_window = EXCLUDED.audit_window,
		     terminal_window = EXCLUDED.terminal_window,
		     action_runs_window = EXCLUDED.action_runs_window,
		     update_runs_window = EXCLUDED.update_runs_window,
		     recordings_window = EXCLUDED.recordings_window,
		     updated_at = EXCLUDED.updated_at`,
		retention.FormatDuration(normalized.LogsWindow),
		retention.FormatDuration(normalized.MetricsWindow),
		retention.FormatDuration(normalized.AuditWindow),
		retention.FormatDuration(normalized.TerminalWindow),
		retention.FormatDuration(normalized.ActionRunsWindow),
		retention.FormatDuration(normalized.UpdateRunsWindow),
		retention.FormatDuration(normalized.RecordingsWindow),
		now,
	)
	if err != nil {
		return retention.Settings{}, err
	}
	return normalized, nil
}

// pruneExecDirect executes a single DELETE as an independent auto-committed statement,
// avoiding long-lived transactions that block concurrent reads.
func (s *PostgresStore) pruneExecDirect(query string, args ...any) (int64, error) {
	tag, err := s.pool.Exec(context.Background(), query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PostgresStore) PruneExpiredData(now time.Time, settings retention.Settings) (retention.PruneResult, error) {
	settings = retention.Normalize(settings)
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	result := retention.PruneResult{RanAt: now}
	var err error

	// Each DELETE runs as an independent statement — no transactional consistency
	// needed between tables, and this avoids holding a long transaction that
	// blocks concurrent readers across 12 hot tables.
	if result.LogsDeleted, err = s.pruneExecDirect(
		`DELETE FROM log_events WHERE timestamp < $1`,
		now.Add(-settings.LogsWindow)); err != nil {
		return retention.PruneResult{}, err
	}
	if result.MetricsDeleted, err = s.pruneExecDirect(
		`DELETE FROM metric_samples WHERE collected_at < $1`,
		now.Add(-settings.MetricsWindow)); err != nil {
		return retention.PruneResult{}, err
	}
	hubMetricsDeleted, err := s.pruneExecDirect(
		`DELETE FROM hub_metric_samples WHERE collected_at < $1`,
		now.Add(-settings.MetricsWindow))
	if err != nil {
		return retention.PruneResult{}, err
	}
	result.MetricsDeleted += hubMetricsDeleted
	if result.AuditDeleted, err = s.pruneExecDirect(
		`DELETE FROM audit_events WHERE timestamp < $1`,
		now.Add(-settings.AuditWindow)); err != nil {
		return retention.PruneResult{}, err
	}
	if result.TerminalCommandsDeleted, err = s.pruneExecDirect(
		`DELETE FROM terminal_commands WHERE updated_at < $1`,
		now.Add(-settings.TerminalWindow)); err != nil {
		return retention.PruneResult{}, err
	}
	if result.TerminalSessionsDeleted, err = s.pruneExecDirect(
		`DELETE FROM terminal_sessions
		 WHERE last_action_at < $1
		   AND NOT EXISTS (SELECT 1 FROM terminal_commands WHERE terminal_commands.session_id = terminal_sessions.id)`,
		now.Add(-settings.TerminalWindow)); err != nil {
		return retention.PruneResult{}, err
	}
	if result.ActionRunsDeleted, err = s.pruneExecDirect(
		`DELETE FROM action_runs WHERE updated_at < $1`,
		now.Add(-settings.ActionRunsWindow)); err != nil {
		return retention.PruneResult{}, err
	}
	if result.UpdateRunsDeleted, err = s.pruneExecDirect(
		`DELETE FROM update_runs WHERE updated_at < $1`,
		now.Add(-settings.UpdateRunsWindow)); err != nil {
		return retention.PruneResult{}, err
	}
	if result.AlertInstancesDeleted, err = s.pruneExecDirect(
		`DELETE FROM alert_instances WHERE status = 'resolved' AND resolved_at < $1`,
		now.Add(-settings.AlertInstancesWindow)); err != nil {
		return retention.PruneResult{}, err
	}
	if result.AlertEvaluationsDeleted, err = s.pruneExecDirect(
		`DELETE FROM alert_evaluations WHERE evaluated_at < $1`,
		now.Add(-settings.AlertEvaluationsWindow)); err != nil {
		return retention.PruneResult{}, err
	}
	if result.NotificationHistoryDeleted, err = s.pruneExecDirect(
		`DELETE FROM notification_history WHERE created_at < $1`,
		now.Add(-settings.NotificationHistoryWindow)); err != nil {
		return retention.PruneResult{}, err
	}

	// Alert silences and recordings still need a transaction for
	// the multi-step logic (schema introspection + delete, RETURNING paths).
	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return retention.PruneResult{}, err
	}
	defer tx.Rollback(context.Background())

	if result.AlertSilencesDeleted, err = pruneExpiredAlertSilences(tx, now.Add(-settings.AlertSilencesWindow)); err != nil {
		return retention.PruneResult{}, err
	}
	recordingsCutoff := now.Add(-settings.RecordingsWindow)
	var recordingPaths []string
	if result.RecordingsDeleted, recordingPaths, err = pruneExpiredRecordings(tx, recordingsCutoff); err != nil {
		return retention.PruneResult{}, err
	}

	if err := tx.Commit(context.Background()); err != nil {
		return retention.PruneResult{}, err
	}
	if result.LogsDeleted > 0 {
		s.invalidateLogEventsWatermarkCache()
	}
	if removed, failed := removeRecordingFiles(recordingPaths); failed > 0 {
		log.Printf("retention: recordings cleanup completed with errors: removed=%d failed=%d", removed, failed)
	}
	return result, nil
}

func pruneExpiredRecordings(tx pgx.Tx, cutoff time.Time) (int64, []string, error) {
	rows, err := tx.Query(context.Background(),
		`DELETE FROM session_recordings
		 WHERE status <> 'recording'
		   AND COALESCE(stopped_at, created_at) < $1
		 RETURNING file_path`,
		cutoff,
	)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	var deleted int64
	paths := make([]string, 0, 16)
	seen := make(map[string]struct{}, 16)

	for rows.Next() {
		var filePath string
		if err := rows.Scan(&filePath); err != nil {
			return 0, nil, err
		}
		deleted++
		trimmed := strings.TrimSpace(filePath)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		paths = append(paths, trimmed)
	}
	if rows.Err() != nil {
		return 0, nil, rows.Err()
	}

	return deleted, paths, nil
}

func pruneExpiredAlertSilences(tx pgx.Tx, cutoff time.Time) (int64, error) {
	if tx == nil {
		return 0, errors.New("nil transaction")
	}
	rows, err := tx.Query(context.Background(),
		`SELECT column_name
		 FROM information_schema.columns
		 WHERE table_schema = current_schema()
		   AND table_name = 'alert_silences'
		   AND column_name IN ('expires_at', 'ends_at')`,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	columns := make([]string, 0, 2)
	for rows.Next() {
		var columnName string
		if scanErr := rows.Scan(&columnName); scanErr != nil {
			return 0, scanErr
		}
		columns = append(columns, strings.TrimSpace(columnName))
	}
	if rows.Err() != nil {
		return 0, rows.Err()
	}

	switch alertSilencePruneColumn(columns) {
	case "expires_at":
		return execDeleteRows(tx,
			`DELETE FROM alert_silences WHERE expires_at IS NOT NULL AND expires_at < $1`,
			cutoff,
		)
	case "ends_at":
		return execDeleteRows(tx,
			`DELETE FROM alert_silences WHERE ends_at < $1`,
			cutoff,
		)
	default:
		// Legacy/partial schemas may not have either column yet.
		return 0, nil
	}
}

func alertSilencePruneColumn(columns []string) string {
	seen := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		trimmed := strings.TrimSpace(strings.ToLower(column))
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	if _, ok := seen["expires_at"]; ok {
		return "expires_at"
	}
	if _, ok := seen["ends_at"]; ok {
		return "ends_at"
	}
	return ""
}

func removeRecordingFiles(paths []string) (removed int64, failed int64) {
	for _, filePath := range paths {
		trimmed := strings.TrimSpace(filePath)
		if trimmed == "" {
			continue
		}
		err := os.Remove(trimmed)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			removed++
			continue
		}
		failed++
		log.Printf("retention: failed to remove recording file %q: %v", trimmed, err)
	}
	return removed, failed
}
