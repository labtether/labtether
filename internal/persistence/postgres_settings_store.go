package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const settingsStoreQueryTimeout = 5 * time.Second

var _ SettingsStore = (*PostgresStore)(nil)

func (s *PostgresStore) GetSystemSetting(ctx context.Context, key string) (json.RawMessage, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false, errors.New("setting key is required")
	}
	ctx, cancel := context.WithTimeout(ctx, settingsStoreQueryTimeout)
	defer cancel()

	var value json.RawMessage
	err := s.pool.QueryRow(ctx,
		`SELECT value FROM system_settings WHERE key = $1`, key,
	).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (s *PostgresStore) PutSystemSetting(ctx context.Context, key string, value json.RawMessage) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("setting key is required")
	}
	if len(value) == 0 {
		return errors.New("setting value is required")
	}
	ctx, cancel := context.WithTimeout(ctx, settingsStoreQueryTimeout)
	defer cancel()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO system_settings (key, value, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		key, value,
	)
	return err
}
