package persistence

import (
	"context"
	"strings"
	"time"
)

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
