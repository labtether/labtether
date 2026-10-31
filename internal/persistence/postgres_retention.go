package persistence

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/retention"
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

// retentionDeleteBatchSize keeps every DELETE far inside the 30 s statement
// timeout, even on a table with millions of expired rows. One unbounded DELETE
// per table could time out on every cycle and leave retention stuck for good.
const retentionDeleteBatchSize = 5000

// retentionMaxBatchesPerTable caps one cycle's work on a single table so the
// other tables still get their turn. The next cycle carries on from there.
const retentionMaxBatchesPerTable = 200

// pruneExecDirect executes a single DELETE as an independent auto-committed statement,
// avoiding long-lived transactions that block concurrent reads.
func (s *PostgresStore) pruneExecDirect(query string, args ...any) (int64, error) {
	tag, err := s.pool.Exec(context.Background(), query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// pruneInBatches runs a DELETE that takes the cutoff as $1 and a row limit as
// $2 until it removes fewer rows than the limit or reaches the per-cycle cap.
func (s *PostgresStore) pruneInBatches(query string, cutoff time.Time) (int64, error) {
	var total int64
	for batch := 0; batch < retentionMaxBatchesPerTable; batch++ {
		deleted, err := s.pruneExecDirect(query, cutoff, retentionDeleteBatchSize)
		total += deleted
		if err != nil {
			return total, err
		}
		if deleted < int64(retentionDeleteBatchSize) {
			break
		}
	}
	return total, nil
}

func (s *PostgresStore) PruneExpiredData(now time.Time, settings retention.Settings) (retention.PruneResult, error) {
	settings = retention.Normalize(settings)
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	result := retention.PruneResult{RanAt: now}
	var failures []error
	// Each table is pruned on its own, so a failure on one table no longer
	// stops the others. Every batch is an independent statement; there is no
	// long transaction blocking concurrent readers.
	prune := func(table string, count *int64, query string, cutoff time.Time) {
		deleted, err := s.pruneInBatches(query, cutoff)
		*count += deleted
		if err != nil {
			failures = append(failures, fmt.Errorf("prune %s: %w", table, err))
		}
	}
	prune("log_events", &result.LogsDeleted,
		`DELETE FROM log_events WHERE id IN (SELECT id FROM log_events WHERE timestamp < $1 LIMIT $2)`,
		now.Add(-settings.LogsWindow))
	prune("metric_samples", &result.MetricsDeleted,
		`DELETE FROM metric_samples WHERE id IN (SELECT id FROM metric_samples WHERE collected_at < $1 LIMIT $2)`,
		now.Add(-settings.MetricsWindow))
	prune("hub_metric_samples", &result.MetricsDeleted,
		`DELETE FROM hub_metric_samples WHERE id IN (SELECT id FROM hub_metric_samples WHERE collected_at < $1 LIMIT $2)`,
		now.Add(-settings.MetricsWindow))
	prune("audit_events", &result.AuditDeleted,
		`DELETE FROM audit_events WHERE id IN (SELECT id FROM audit_events WHERE timestamp < $1 LIMIT $2)`,
		now.Add(-settings.AuditWindow))
	prune("terminal_commands", &result.TerminalCommandsDeleted,
		`DELETE FROM terminal_commands WHERE id IN (SELECT id FROM terminal_commands WHERE updated_at < $1 LIMIT $2)`,
		now.Add(-settings.TerminalWindow))
	prune("terminal_sessions", &result.TerminalSessionsDeleted,
		`DELETE FROM terminal_sessions WHERE id IN (
		   SELECT id FROM terminal_sessions
		   WHERE last_action_at < $1
		     AND NOT EXISTS (SELECT 1 FROM terminal_commands WHERE terminal_commands.session_id = terminal_sessions.id)
		   LIMIT $2)`,
		now.Add(-settings.TerminalWindow))
	prune("action_runs", &result.ActionRunsDeleted,
		`DELETE FROM action_runs WHERE id IN (SELECT id FROM action_runs WHERE updated_at < $1 LIMIT $2)`,
		now.Add(-settings.ActionRunsWindow))
	prune("update_runs", &result.UpdateRunsDeleted,
		`DELETE FROM update_runs WHERE id IN (SELECT id FROM update_runs WHERE updated_at < $1 LIMIT $2)`,
		now.Add(-settings.UpdateRunsWindow))
	prune("alert_instances", &result.AlertInstancesDeleted,
		`DELETE FROM alert_instances WHERE id IN (SELECT id FROM alert_instances WHERE status = 'resolved' AND resolved_at < $1 LIMIT $2)`,
		now.Add(-settings.AlertInstancesWindow))
	prune("alert_evaluations", &result.AlertEvaluationsDeleted,
		`DELETE FROM alert_evaluations WHERE id IN (SELECT id FROM alert_evaluations WHERE evaluated_at < $1 LIMIT $2)`,
		now.Add(-settings.AlertEvaluationsWindow))
	prune("notification_history", &result.NotificationHistoryDeleted,
		`DELETE FROM notification_history WHERE id IN (SELECT id FROM notification_history WHERE created_at < $1 LIMIT $2)`,
		now.Add(-settings.NotificationHistoryWindow))

	if deleted, err := s.pruneAlertSilences(now.Add(-settings.AlertSilencesWindow)); err != nil {
		failures = append(failures, fmt.Errorf("prune alert_silences: %w", err))
	} else {
		result.AlertSilencesDeleted = deleted
	}
	if result.LogsDeleted > 0 {
		s.invalidateLogEventsWatermarkCache()
	}
	recordingsDeleted, err := s.pruneExpiredRecordings(now.Add(-settings.RecordingsWindow))
	result.RecordingsDeleted = recordingsDeleted
	if err != nil {
		failures = append(failures, fmt.Errorf("prune recordings: %w", err))
	}
	return result, errors.Join(failures...)
}

// pruneAlertSilences needs a transaction for schema introspection and deletion.
func (s *PostgresStore) pruneAlertSilences(cutoff time.Time) (int64, error) {
	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	deleted, err := pruneExpiredAlertSilences(tx, cutoff)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(context.Background()); err != nil {
		return 0, err
	}
	return deleted, nil
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
