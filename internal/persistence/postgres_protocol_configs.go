package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/protocols"
)

// --- scan helpers ---

type protocolConfigScanner interface {
	Scan(dest ...any) error
}

func scanProtocolConfig(row protocolConfigScanner) (*protocols.ProtocolConfig, error) {
	pc := &protocols.ProtocolConfig{}
	var credentialProfileID *string
	var lastTestedAt *time.Time
	var testError *string
	var config []byte

	if err := row.Scan(
		&pc.ID,
		&pc.AssetID,
		&pc.Protocol,
		&pc.Host,
		&pc.Port,
		&pc.Username,
		&credentialProfileID,
		&pc.Enabled,
		&lastTestedAt,
		&pc.TestStatus,
		&testError,
		&config,
		&pc.CreatedAt,
		&pc.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if credentialProfileID != nil {
		pc.CredentialProfileID = *credentialProfileID
	}
	if lastTestedAt != nil {
		t := lastTestedAt.UTC()
		pc.LastTestedAt = &t
	}
	if testError != nil {
		pc.TestError = *testError
	}
	if len(config) > 0 {
		pc.Config = config
	}
	pc.CreatedAt = pc.CreatedAt.UTC()
	pc.UpdatedAt = pc.UpdatedAt.UTC()
	return pc, nil
}

// --- columns ---

const protocolConfigColumns = `id, asset_id, protocol, host, port, username, credential_profile_id, enabled, last_tested_at, test_status, test_error, config, created_at, updated_at`

// --- store methods ---

// SaveProtocolConfig upserts a protocol config record for the given asset.
// On conflict (asset_id, protocol) the existing row is updated in place,
// and the database-assigned id, created_at, and updated_at are written back
// into pc.
func (s *PostgresStore) SaveProtocolConfig(ctx context.Context, pc *protocols.ProtocolConfig) error {
	configPayload := pc.Config
	if len(configPayload) == 0 {
		configPayload = []byte("{}")
	}

	credentialProfileID := nullIfBlank(pc.CredentialProfileID)

	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO asset_protocol_configs (%s)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, now(), now())
		 ON CONFLICT (asset_id, protocol) DO UPDATE
		 SET host = EXCLUDED.host,
		     port = EXCLUDED.port,
		     username = EXCLUDED.username,
		     credential_profile_id = EXCLUDED.credential_profile_id,
		     enabled = EXCLUDED.enabled,
		     config = EXCLUDED.config,
		     updated_at = now()
		 RETURNING %s`, protocolConfigColumns, protocolConfigColumns),
		pc.ID,
		pc.AssetID,
		pc.Protocol,
		pc.Host,
		pc.Port,
		pc.Username,
		credentialProfileID,
		pc.Enabled,
		nullTime(pc.LastTestedAt),
		pc.TestStatus,
		nullIfBlank(pc.TestError),
		configPayload,
	)

	updated, err := scanProtocolConfig(row)
	if err != nil {
		return err
	}
	pc.ID = updated.ID
	pc.CreatedAt = updated.CreatedAt
	pc.UpdatedAt = updated.UpdatedAt
	return nil
}

// GetProtocolConfig returns the protocol config for the given asset and protocol.
// Returns nil (not an error) when no row exists.
func (s *PostgresStore) GetProtocolConfig(ctx context.Context, assetID, protocol string) (*protocols.ProtocolConfig, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM asset_protocol_configs WHERE asset_id = $1 AND protocol = $2`, protocolConfigColumns),
		strings.TrimSpace(assetID),
		strings.TrimSpace(protocol),
	)
	pc, err := scanProtocolConfig(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return pc, nil
}

// ListProtocolConfigs returns all protocol configs for the given asset, ordered
// by protocol name.
func (s *PostgresStore) ListProtocolConfigs(ctx context.Context, assetID string) ([]*protocols.ProtocolConfig, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM asset_protocol_configs WHERE asset_id = $1 ORDER BY protocol`, protocolConfigColumns),
		strings.TrimSpace(assetID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*protocols.ProtocolConfig, 0, 4)
	for rows.Next() {
		pc, scanErr := scanProtocolConfig(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, pc)
	}
	return out, rows.Err()
}

// DeleteProtocolConfig removes the protocol config for the given asset and
// protocol. Returns an error when the row does not exist.
func (s *PostgresStore) DeleteProtocolConfig(ctx context.Context, assetID, protocol string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM asset_protocol_configs WHERE asset_id = $1 AND protocol = $2`,
		strings.TrimSpace(assetID),
		strings.TrimSpace(protocol),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("protocol config not found for asset %q protocol %q", assetID, protocol)
	}
	return nil
}

// UpdateProtocolTestResult records the outcome of a connectivity test for the
// given asset and protocol.
func (s *PostgresStore) UpdateProtocolTestResult(ctx context.Context, assetID, protocol, status, testError string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE asset_protocol_configs
		 SET test_status = $3, test_error = $4, last_tested_at = $5, updated_at = $5
		 WHERE asset_id = $1 AND protocol = $2`,
		strings.TrimSpace(assetID),
		strings.TrimSpace(protocol),
		strings.TrimSpace(status),
		nullIfBlank(testError),
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("protocol config not found for asset %q protocol %q", assetID, protocol)
	}
	return nil
}

// UpdateProtocolConfigCredential sets the credential_profile_id for the given
// asset and protocol.
func (s *PostgresStore) UpdateProtocolConfigCredential(ctx context.Context, assetID, protocol, credentialProfileID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE asset_protocol_configs
		 SET credential_profile_id = $3, updated_at = $4
		 WHERE asset_id = $1 AND protocol = $2`,
		strings.TrimSpace(assetID),
		strings.TrimSpace(protocol),
		nullIfBlank(credentialProfileID),
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("protocol config not found for asset %q protocol %q", assetID, protocol)
	}
	return nil
}

// ListStaleProtocolConfigs returns enabled protocol configs that haven't been
// tested within the given threshold duration (or have never been tested).
func (s *PostgresStore) ListStaleProtocolConfigs(ctx context.Context, staleThreshold time.Duration) ([]*protocols.ProtocolConfig, error) {
	cutoff := time.Now().UTC().Add(-staleThreshold)
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM asset_protocol_configs
		 WHERE enabled = true AND (last_tested_at IS NULL OR last_tested_at < $1)
		 ORDER BY last_tested_at ASC NULLS FIRST
		 LIMIT 100`, protocolConfigColumns),
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*protocols.ProtocolConfig, 0, 20)
	for rows.Next() {
		pc, scanErr := scanProtocolConfig(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, pc)
	}
	return out, rows.Err()
}
