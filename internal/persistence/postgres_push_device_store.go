package persistence

import (
	"context"
	"errors"
	"strings"
	"time"
)

const MaxPushDevicesPerUser = 32

var ErrPushDeviceRegistrationLimit = errors.New("push device registration limit reached")

// PushDevice represents a registered push notification device.
type PushDevice struct {
	ID                     string    `json:"id"`
	UserID                 string    `json:"user_id"`
	DeviceID               string    `json:"device_id"`
	Platform               string    `json:"platform"`
	PushToken              string    `json:"push_token"`
	BundleID               string    `json:"bundle_id"`
	Environment            string    `json:"environment"`
	TimeZone               string    `json:"time_zone"`
	NotifyCriticalAlerts   bool      `json:"notify_critical_alerts"`
	NotifyNodeOffline      bool      `json:"notify_node_offline"`
	NotifyServiceDown      bool      `json:"notify_service_down"`
	PushCategory           string    `json:"push_category"`
	MinimumSeverity        string    `json:"minimum_severity"`
	QuietHoursEnabled      bool      `json:"quiet_hours_enabled"`
	QuietHoursStartMinutes int       `json:"quiet_hours_start_minutes"`
	QuietHoursEndMinutes   int       `json:"quiet_hours_end_minutes"`
	DigestWindowSeconds    int       `json:"digest_window_seconds"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// UpsertPushDevice inserts or updates a push device registration.
// On conflict (same user_id + device_id), updates the token, APNs routing
// metadata, and notification preferences.
func (s *PostgresStore) UpsertPushDevice(ctx context.Context, d PushDevice) error {
	d.UserID = strings.TrimSpace(d.UserID)
	if d.UserID == "" {
		return errors.New("push device user id is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Count-then-insert must be serialized per principal. Updates and token
	// rotation that reclaim an existing registration remain allowed at the cap.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "push-device-user:"+d.UserID); err != nil {
		return err
	}
	var registrationCount int
	var replacesExisting bool
	if err = tx.QueryRow(ctx, `
		SELECT COUNT(*), EXISTS (
			SELECT 1
			  FROM push_devices existing
			 WHERE existing.user_id = $1
			   AND (
				existing.device_id = $2
				OR (
					existing.push_token = $3
					AND (existing.bundle_id = $4 OR existing.bundle_id = '' OR $4 = '')
					AND (existing.environment = $5 OR existing.environment = '' OR $5 = '')
				)
			   )
		)
		  FROM push_devices
		 WHERE user_id = $1
	`, d.UserID, d.DeviceID, d.PushToken, d.BundleID, d.Environment).Scan(&registrationCount, &replacesExisting); err != nil {
		return err
	}
	if registrationCount >= MaxPushDevicesPerUser && !replacesExisting {
		return ErrPushDeviceRegistrationLimit
	}

	if _, err = tx.Exec(ctx, `
		WITH reclaimed AS (
			DELETE FROM push_devices
			 WHERE NOT (user_id = $1 AND device_id = $2)
			   AND (
				(device_id = $2
				 AND (bundle_id = $5 OR bundle_id = '' OR $5 = '')
				 AND (environment = $6 OR environment = '' OR $6 = ''))
				OR
				(push_token = $4
				 AND (bundle_id = $5 OR bundle_id = '' OR $5 = '')
				 AND (environment = $6 OR environment = '' OR $6 = ''))
			   )
			RETURNING id
		)
		INSERT INTO push_devices (
			id, user_id, device_id, platform, push_token, bundle_id, environment,
			time_zone,
			notify_critical_alerts, notify_node_offline, notify_service_down,
			push_category, minimum_severity, quiet_hours_enabled,
			quiet_hours_start_minutes, quiet_hours_end_minutes,
			digest_window_seconds, updated_at
		)
		VALUES (
			gen_random_uuid()::TEXT, $1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, $14, $15, $16, NOW()
		)
		ON CONFLICT (user_id, device_id)
		DO UPDATE SET
			platform = EXCLUDED.platform,
			push_token = EXCLUDED.push_token,
			bundle_id = EXCLUDED.bundle_id,
			environment = EXCLUDED.environment,
			time_zone = EXCLUDED.time_zone,
			notify_critical_alerts = EXCLUDED.notify_critical_alerts,
			notify_node_offline = EXCLUDED.notify_node_offline,
			notify_service_down = EXCLUDED.notify_service_down,
			push_category = EXCLUDED.push_category,
			minimum_severity = EXCLUDED.minimum_severity,
			quiet_hours_enabled = EXCLUDED.quiet_hours_enabled,
			quiet_hours_start_minutes = EXCLUDED.quiet_hours_start_minutes,
			quiet_hours_end_minutes = EXCLUDED.quiet_hours_end_minutes,
			digest_window_seconds = EXCLUDED.digest_window_seconds,
			updated_at = NOW()
	`,
		d.UserID,
		d.DeviceID,
		d.Platform,
		d.PushToken,
		d.BundleID,
		d.Environment,
		d.TimeZone,
		d.NotifyCriticalAlerts,
		d.NotifyNodeOffline,
		d.NotifyServiceDown,
		d.PushCategory,
		d.MinimumSeverity,
		d.QuietHoursEnabled,
		d.QuietHoursStartMinutes,
		d.QuietHoursEndMinutes,
		d.DigestWindowSeconds,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeletePushDeviceByToken removes permanently rejected APNs registrations.
// Empty legacy routing fields are matched as fallbacks because older clients
// registered before topic/environment metadata became mandatory.
func (s *PostgresStore) DeletePushDeviceByToken(ctx context.Context, pushToken, bundleID, environment string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM push_devices
		 WHERE push_token = $1
		   AND (bundle_id = $2 OR bundle_id = '')
		   AND (environment = $3 OR environment = '')
	`, pushToken, bundleID, environment)
	return err
}

// DeletePushDevice removes a push device registration for a user.
func (s *PostgresStore) DeletePushDevice(ctx context.Context, userID, deviceID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM push_devices WHERE user_id = $1 AND device_id = $2`, userID, deviceID)
	return err
}

// GetPushDevicesForUser returns all push devices registered by a user.
func (s *PostgresStore) GetPushDevicesForUser(ctx context.Context, userID string) ([]PushDevice, error) {
	rows, err := s.pool.Query(ctx, pushDeviceSelectSQL+` WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []PushDevice
	for rows.Next() {
		var d PushDevice
		if err := scanPushDevice(rows, &d); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// GetAllPushTokens returns all registered push devices across all users.
func (s *PostgresStore) GetAllPushTokens(ctx context.Context) ([]PushDevice, error) {
	rows, err := s.pool.Query(ctx, pushDeviceSelectSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []PushDevice
	for rows.Next() {
		var d PushDevice
		if err := scanPushDevice(rows, &d); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

const pushDeviceSelectSQL = `SELECT
	id, user_id, device_id, platform, push_token, bundle_id, environment,
	time_zone,
	notify_critical_alerts, notify_node_offline, notify_service_down,
	push_category, minimum_severity, quiet_hours_enabled,
	quiet_hours_start_minutes, quiet_hours_end_minutes, digest_window_seconds,
	created_at, updated_at
FROM push_devices`

type pushDeviceScanner interface {
	Scan(dest ...any) error
}

func scanPushDevice(row pushDeviceScanner, d *PushDevice) error {
	return row.Scan(
		&d.ID,
		&d.UserID,
		&d.DeviceID,
		&d.Platform,
		&d.PushToken,
		&d.BundleID,
		&d.Environment,
		&d.TimeZone,
		&d.NotifyCriticalAlerts,
		&d.NotifyNodeOffline,
		&d.NotifyServiceDown,
		&d.PushCategory,
		&d.MinimumSeverity,
		&d.QuietHoursEnabled,
		&d.QuietHoursStartMinutes,
		&d.QuietHoursEndMinutes,
		&d.DigestWindowSeconds,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
}
