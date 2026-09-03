package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/secrets"
)

const settingsStoreQueryTimeout = 5 * time.Second

const (
	OIDCSystemSettingsKey = "oidc"
	OIDCClientSecretAAD   = "system_settings:oidc:client_secret"
)

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

// MigrateLegacyOIDCClientSecret replaces the old plaintext client_secret JSON
// field with an authenticated ciphertext while holding a row lock. It is safe
// to call more than once. If an old writer left both fields, the plaintext is
// treated as the latest value and replaces the older ciphertext.
func (s *PostgresStore) MigrateLegacyOIDCClientSecret(ctx context.Context, manager *secrets.Manager) error {
	if s == nil || s.pool == nil {
		return errors.New("settings store is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, settingsStoreQueryTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin oidc client secret migration: %w", err)
	}
	defer tx.Rollback(ctx)

	var raw json.RawMessage
	err = tx.QueryRow(ctx,
		`SELECT value FROM system_settings WHERE key = $1 FOR UPDATE`,
		OIDCSystemSettingsKey,
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load oidc settings for client secret migration: %w", err)
	}

	var stored map[string]json.RawMessage
	if err := json.Unmarshal(raw, &stored); err != nil {
		return fmt.Errorf("decode oidc settings for client secret migration: %w", err)
	}
	legacyRaw, hasLegacyField := stored["client_secret"]
	if !hasLegacyField {
		return nil
	}

	var legacySecret string
	if err := json.Unmarshal(legacyRaw, &legacySecret); err != nil {
		return errors.New("decode legacy oidc client secret: invalid value")
	}
	delete(stored, "client_secret")
	if strings.TrimSpace(legacySecret) != "" {
		if manager == nil {
			return errors.New("encrypt legacy oidc client secret: secrets manager is not configured")
		}
		ciphertext, err := manager.EncryptString(legacySecret, OIDCClientSecretAAD)
		if err != nil {
			return fmt.Errorf("encrypt legacy oidc client secret: %w", err)
		}
		ciphertextJSON, err := json.Marshal(ciphertext)
		if err != nil {
			return fmt.Errorf("encode encrypted oidc client secret: %w", err)
		}
		stored["client_secret_ciphertext"] = ciphertextJSON
	}

	updated, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("encode migrated oidc settings: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE system_settings SET value = $2, updated_at = now() WHERE key = $1`,
		OIDCSystemSettingsKey, updated,
	); err != nil {
		return fmt.Errorf("persist encrypted oidc client secret: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit oidc client secret migration: %w", err)
	}
	return nil
}
