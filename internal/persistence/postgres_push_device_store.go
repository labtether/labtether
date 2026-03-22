package persistence

import (
	"context"
	"time"
)

// PushDevice represents a registered push notification device.
type PushDevice struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	DeviceID  string    `json:"device_id"`
	Platform  string    `json:"platform"`
	PushToken string    `json:"push_token"`
	BundleID  string    `json:"bundle_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpsertPushDevice inserts or updates a push device registration.
// On conflict (same user_id + device_id), updates the token and bundle_id.
func (s *PostgresStore) UpsertPushDevice(ctx context.Context, d PushDevice) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO push_devices (id, user_id, device_id, platform, push_token, bundle_id, updated_at)
		VALUES (gen_random_uuid()::TEXT, $1, $2, $3, $4, $5, NOW())
		ON CONFLICT (user_id, device_id)
		DO UPDATE SET push_token = $4, bundle_id = $5, updated_at = NOW()
	`, d.UserID, d.DeviceID, d.Platform, d.PushToken, d.BundleID)
	return err
}

// DeletePushDevice removes a push device registration for a user.
func (s *PostgresStore) DeletePushDevice(ctx context.Context, userID, deviceID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM push_devices WHERE user_id = $1 AND device_id = $2`, userID, deviceID)
	return err
}

// GetPushDevicesForUser returns all push devices registered by a user.
func (s *PostgresStore) GetPushDevicesForUser(ctx context.Context, userID string) ([]PushDevice, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, user_id, device_id, platform, push_token, bundle_id, created_at, updated_at FROM push_devices WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []PushDevice
	for rows.Next() {
		var d PushDevice
		if err := rows.Scan(&d.ID, &d.UserID, &d.DeviceID, &d.Platform, &d.PushToken, &d.BundleID, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// GetAllPushTokens returns all registered push devices across all users.
func (s *PostgresStore) GetAllPushTokens(ctx context.Context) ([]PushDevice, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, user_id, device_id, platform, push_token, bundle_id, created_at, updated_at FROM push_devices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []PushDevice
	for rows.Next() {
		var d PushDevice
		if err := rows.Scan(&d.ID, &d.UserID, &d.DeviceID, &d.Platform, &d.PushToken, &d.BundleID, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}
