package persistence

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/retention"
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

func (s *PostgresStore) ListRuntimeSettingOverrides() (map[string]string, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT key, value FROM runtime_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) SaveRuntimeSettingOverrides(values map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return s.ListRuntimeSettingOverrides()
	}

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())

	now := time.Now().UTC()
	for key, value := range values {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if _, err := tx.Exec(context.Background(),
			`INSERT INTO runtime_settings (key, value, updated_at)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (key) DO UPDATE
			 SET value = EXCLUDED.value,
			     updated_at = EXCLUDED.updated_at`,
			key,
			value,
			now,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		return nil, err
	}
	return s.ListRuntimeSettingOverrides()
}

func (s *PostgresStore) DeleteRuntimeSettingOverrides(keys []string) error {
	if len(keys) == 0 {
		_, err := s.pool.Exec(context.Background(), `DELETE FROM runtime_settings`)
		return err
	}

	_, err := s.pool.Exec(context.Background(), `DELETE FROM runtime_settings WHERE key = ANY($1)`, keys)
	return err
}

func (s *PostgresStore) CreateCredentialProfile(profile credentials.Profile) (credentials.Profile, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(profile.ID) == "" {
		profile.ID = idgen.New("cred")
	}
	status := strings.TrimSpace(profile.Status)
	if status == "" {
		status = "active"
	}
	metadataPayload, err := marshalStringMap(profile.Metadata)
	if err != nil {
		return credentials.Profile{}, err
	}

	created, err := scanCredentialProfile(s.pool.QueryRow(context.Background(),
		`INSERT INTO credential_profiles (
			id,
			name,
			kind,
			username,
			description,
			status,
			metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $10, $10, $11)
		RETURNING
			id,
			name,
			kind,
			username,
			description,
			status,
			metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			last_used_at,
			expires_at`,
		profile.ID,
		strings.TrimSpace(profile.Name),
		strings.TrimSpace(profile.Kind),
		nullIfBlank(profile.Username),
		nullIfBlank(profile.Description),
		status,
		metadataPayload,
		strings.TrimSpace(profile.SecretCiphertext),
		nullIfBlank(profile.PassphraseCiphertext),
		now,
		nullTime(profile.ExpiresAt),
	))
	if err != nil {
		return credentials.Profile{}, err
	}
	return created, nil
}

func (s *PostgresStore) UpdateCredentialProfile(profile credentials.Profile) (credentials.Profile, error) {
	now := time.Now().UTC()
	status := strings.TrimSpace(profile.Status)
	if status == "" {
		status = "active"
	}
	metadataPayload, err := marshalStringMap(profile.Metadata)
	if err != nil {
		return credentials.Profile{}, err
	}

	updated, err := scanCredentialProfile(s.pool.QueryRow(context.Background(),
		`UPDATE credential_profiles
		 SET name = $2,
		     kind = $3,
		     username = $4,
		     description = $5,
		     status = $6,
		     metadata = $7::jsonb,
		     secret_ciphertext = $8,
		     passphrase_ciphertext = $9,
		     updated_at = $10,
		     rotated_at = $11,
		     expires_at = $12
		 WHERE id = $1
		 RETURNING
			id,
			name,
			kind,
			username,
			description,
			status,
			metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			last_used_at,
			expires_at`,
		strings.TrimSpace(profile.ID),
		strings.TrimSpace(profile.Name),
		strings.TrimSpace(profile.Kind),
		nullIfBlank(profile.Username),
		nullIfBlank(profile.Description),
		status,
		metadataPayload,
		strings.TrimSpace(profile.SecretCiphertext),
		nullIfBlank(profile.PassphraseCiphertext),
		now,
		nullTime(profile.RotatedAt),
		nullTime(profile.ExpiresAt),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credentials.Profile{}, errors.New("credential profile not found")
		}
		return credentials.Profile{}, err
	}
	return updated, nil
}

func (s *PostgresStore) UpdateCredentialProfileSecret(id, secretCiphertext, passphraseCiphertext string, expiresAt *time.Time) (credentials.Profile, error) {
	now := time.Now().UTC()
	updated, err := scanCredentialProfile(s.pool.QueryRow(context.Background(),
		`UPDATE credential_profiles
		 SET secret_ciphertext = $2,
		     passphrase_ciphertext = $3,
		     rotated_at = $4,
		     updated_at = $4,
		     expires_at = $5
		 WHERE id = $1
		 RETURNING
			id,
			name,
			kind,
			username,
			description,
			status,
			metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			last_used_at,
			expires_at`,
		strings.TrimSpace(id),
		strings.TrimSpace(secretCiphertext),
		nullIfBlank(passphraseCiphertext),
		now,
		nullTime(expiresAt),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credentials.Profile{}, errors.New("credential profile not found")
		}
		return credentials.Profile{}, err
	}
	return updated, nil
}

func (s *PostgresStore) GetCredentialProfile(id string) (credentials.Profile, bool, error) {
	profile, err := scanCredentialProfile(s.pool.QueryRow(context.Background(),
		`SELECT
			id,
			name,
			kind,
			username,
			description,
			status,
			metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			last_used_at,
			expires_at
		 FROM credential_profiles
		 WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credentials.Profile{}, false, nil
		}
		return credentials.Profile{}, false, err
	}
	return profile, true, nil
}

func (s *PostgresStore) ListCredentialProfiles(limit int) ([]credentials.Profile, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT
			id,
			name,
			kind,
			username,
			description,
			status,
			metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			last_used_at,
			expires_at
		 FROM credential_profiles
		 ORDER BY updated_at DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]credentials.Profile, 0, limit)
	for rows.Next() {
		profile, scanErr := scanCredentialProfile(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, profile)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) MarkCredentialProfileUsed(id string, usedAt time.Time) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE credential_profiles
		 SET last_used_at = $2,
		     updated_at = GREATEST(updated_at, $2)
		 WHERE id = $1`,
		strings.TrimSpace(id),
		usedAt.UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("credential profile not found")
	}
	return nil
}

func (s *PostgresStore) DeleteCredentialProfile(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM credential_profiles WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) SaveAssetTerminalConfig(cfg credentials.AssetTerminalConfig) (credentials.AssetTerminalConfig, error) {
	if cfg.Port <= 0 {
		cfg.Port = 22
	}
	now := time.Now().UTC()
	saved, err := scanAssetTerminalConfig(s.pool.QueryRow(context.Background(),
		`INSERT INTO asset_terminal_configs (
			asset_id,
			host,
			port,
			username,
			strict_host_key,
			host_key,
			credential_profile_id,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (asset_id) DO UPDATE
		SET host = EXCLUDED.host,
		    port = EXCLUDED.port,
		    username = EXCLUDED.username,
		    strict_host_key = EXCLUDED.strict_host_key,
		    host_key = EXCLUDED.host_key,
		    credential_profile_id = EXCLUDED.credential_profile_id,
		    updated_at = EXCLUDED.updated_at
		RETURNING asset_id, host, port, username, strict_host_key, host_key, credential_profile_id, updated_at`,
		strings.TrimSpace(cfg.AssetID),
		strings.TrimSpace(cfg.Host),
		cfg.Port,
		nullIfBlank(cfg.Username),
		cfg.StrictHostKey,
		nullIfBlank(cfg.HostKey),
		nullIfBlank(cfg.CredentialProfileID),
		now,
	))
	if err != nil {
		return credentials.AssetTerminalConfig{}, err
	}
	return saved, nil
}

func (s *PostgresStore) GetAssetTerminalConfig(assetID string) (credentials.AssetTerminalConfig, bool, error) {
	cfg, err := scanAssetTerminalConfig(s.pool.QueryRow(context.Background(),
		`SELECT asset_id, host, port, username, strict_host_key, host_key, credential_profile_id, updated_at
		 FROM asset_terminal_configs
		 WHERE asset_id = $1`,
		strings.TrimSpace(assetID),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credentials.AssetTerminalConfig{}, false, nil
		}
		return credentials.AssetTerminalConfig{}, false, err
	}
	return cfg, true, nil
}

func (s *PostgresStore) DeleteAssetTerminalConfig(assetID string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM asset_terminal_configs WHERE asset_id = $1`,
		strings.TrimSpace(assetID),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) SaveDesktopConfig(cfg credentials.AssetDesktopConfig) (credentials.AssetDesktopConfig, error) {
	if cfg.VNCPort <= 0 {
		cfg.VNCPort = 5900
	}
	now := time.Now().UTC()
	row := s.pool.QueryRow(context.Background(),
		`INSERT INTO asset_desktop_configs (asset_id, vnc_port, credential_profile_id, updated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (asset_id) DO UPDATE
		 SET vnc_port = EXCLUDED.vnc_port,
		     credential_profile_id = EXCLUDED.credential_profile_id,
		     updated_at = EXCLUDED.updated_at
		 RETURNING asset_id, vnc_port, credential_profile_id, updated_at`,
		strings.TrimSpace(cfg.AssetID),
		cfg.VNCPort,
		nullIfBlank(cfg.CredentialProfileID),
		now,
	)
	var profileID *string
	if err := row.Scan(&cfg.AssetID, &cfg.VNCPort, &profileID, &cfg.UpdatedAt); err != nil {
		return credentials.AssetDesktopConfig{}, err
	}
	if profileID != nil {
		cfg.CredentialProfileID = *profileID
	}
	cfg.UpdatedAt = cfg.UpdatedAt.UTC()
	return cfg, nil
}

func (s *PostgresStore) GetDesktopConfig(assetID string) (credentials.AssetDesktopConfig, bool, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT asset_id, vnc_port, credential_profile_id, updated_at
		 FROM asset_desktop_configs WHERE asset_id = $1`,
		strings.TrimSpace(assetID),
	)
	var cfg credentials.AssetDesktopConfig
	var profileID *string
	if err := row.Scan(&cfg.AssetID, &cfg.VNCPort, &profileID, &cfg.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credentials.AssetDesktopConfig{}, false, nil
		}
		return credentials.AssetDesktopConfig{}, false, err
	}
	if profileID != nil {
		cfg.CredentialProfileID = *profileID
	}
	cfg.UpdatedAt = cfg.UpdatedAt.UTC()
	return cfg, true, nil
}

func (s *PostgresStore) DeleteDesktopConfig(assetID string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM asset_desktop_configs WHERE asset_id = $1`,
		strings.TrimSpace(assetID),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
