package persistence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/credentials"
	"strings"
	"time"
)

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
