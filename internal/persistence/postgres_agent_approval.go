package persistence

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/idgen"
	"strings"
	"time"
)

func (s *PostgresStore) PrepareAgentApproval(ctx context.Context, req AgentApprovalPrepareRequest) (enrollment.AgentToken, error) {
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		return enrollment.AgentToken{}, fmt.Errorf("asset id is required")
	}
	now := time.Now().UTC()
	preparedExpiry := boundedPreparedApprovalExpiry(req.PreparedTokenExpiresAt, now)
	preparedTTL, err := databaseTTL(preparedExpiry)
	if err != nil {
		return enrollment.AgentToken{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return enrollment.AgentToken{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAgentIdentityAsset(ctx, tx, assetID); err != nil {
		return enrollment.AgentToken{}, err
	}
	retired, err := isAgentIdentityRetired(ctx, tx, assetID)
	if err != nil {
		return enrollment.AgentToken{}, err
	}
	if retired {
		return enrollment.AgentToken{}, ErrAgentIdentityRetired
	}
	var existingAssetID string
	if err := tx.QueryRow(ctx, `SELECT id FROM assets WHERE id = $1 FOR UPDATE`, assetID).Scan(&existingAssetID); err == nil {
		return enrollment.AgentToken{}, ErrAgentApprovalAssetConflict
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return enrollment.AgentToken{}, err
	}
	var duplicatePendingID string
	err = tx.QueryRow(ctx,
		`SELECT id
		 FROM agent_tokens
		 WHERE asset_id = $1 AND status = 'pending' AND revoked_at IS NULL AND expires_at > clock_timestamp()
		 FOR UPDATE`,
		assetID,
	).Scan(&duplicatePendingID)
	if err == nil {
		return enrollment.AgentToken{}, ErrAgentApprovalAssetConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return enrollment.AgentToken{}, err
	}
	if err := ensureAgentFleetCapacity(ctx, tx, req.MaxEnrolledAgents); err != nil {
		return enrollment.AgentToken{}, err
	}
	token := enrollment.AgentToken{
		ID:          idgen.New("atok"),
		AssetID:     assetID,
		Status:      "pending",
		EnrolledVia: "console-approval",
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO agent_tokens (id, asset_id, token_hash, status, enrolled_via, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, clock_timestamp() + ($6::double precision * interval '1 second'), clock_timestamp())
		 RETURNING expires_at, created_at`,
		token.ID, token.AssetID, req.AgentTokenHash, token.Status,
		token.EnrolledVia, preparedTTL.Seconds(),
	).Scan(&token.ExpiresAt, &token.CreatedAt); err != nil {
		return enrollment.AgentToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return enrollment.AgentToken{}, err
	}
	return token, nil
}

func (s *PostgresStore) FinalizeAgentApproval(ctx context.Context, req AgentApprovalFinalizeRequest) (assets.Asset, error) {
	assetID := strings.TrimSpace(req.AssetID)
	fingerprint := strings.TrimSpace(req.DeviceFingerprint)
	algorithm := strings.TrimSpace(req.DeviceKeyAlgorithm)
	if assetID == "" || fingerprint == "" || algorithm == "" {
		return assets.Asset{}, ErrAgentIdentityContinuityConflict
	}
	agentTokenTTL, err := databaseTTL(req.AgentTokenExpiresAt)
	if err != nil {
		return assets.Asset{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return assets.Asset{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAgentIdentityAsset(ctx, tx, assetID); err != nil {
		return assets.Asset{}, err
	}
	if err := lockAgentEnrollmentIssuance(ctx, tx); err != nil {
		return assets.Asset{}, err
	}

	var prepared enrollment.AgentToken
	var lastUsedAt, revokedAt *time.Time
	err = tx.QueryRow(ctx,
		`SELECT id, asset_id, status, enrolled_via, expires_at, last_used_at, created_at, revoked_at
		 FROM agent_tokens
		 WHERE id = $1 AND asset_id = $2 AND status = 'pending' AND revoked_at IS NULL AND expires_at > clock_timestamp()
		 FOR UPDATE`,
		strings.TrimSpace(req.PreparedTokenID),
		assetID,
	).Scan(&prepared.ID, &prepared.AssetID, &prepared.Status, &prepared.EnrolledVia,
		&prepared.ExpiresAt, &lastUsedAt, &prepared.CreatedAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return assets.Asset{}, ErrPreparedAgentApprovalNotFound
	}
	if err != nil {
		return assets.Asset{}, err
	}

	retired, err := isAgentIdentityRetired(ctx, tx, assetID)
	if err != nil {
		return assets.Asset{}, err
	}
	if retired {
		return assets.Asset{}, ErrAgentIdentityRetired
	}
	_, exists, err := selectAgentIdentityAsset(ctx, tx, assetID)
	if err != nil {
		return assets.Asset{}, err
	}
	if exists {
		return assets.Asset{}, ErrAgentApprovalAssetConflict
	}
	now := time.Now().UTC()
	asset := assets.Asset{
		ID:       assetID,
		Type:     "node",
		Name:     strings.TrimSpace(req.Hostname),
		Source:   "agent",
		Status:   "pending",
		Platform: strings.TrimSpace(req.Platform),
		Metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint:  fingerprint,
			assets.MetadataKeyAgentDeviceKeyAlgorithm: algorithm,
			assets.MetadataKeyAgentIdentityVerifiedAt: now.Format(time.RFC3339Nano),
		},
		CreatedAt:  now,
		UpdatedAt:  now,
		LastSeenAt: now,
	}
	metadataPayload, err := marshalStringMap(asset.Metadata)
	if err != nil {
		return assets.Asset{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO assets (id, type, name, source, status, platform, metadata, created_at, updated_at, last_seen_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $8, $8)`,
		asset.ID, asset.Type, asset.Name, asset.Source, asset.Status, asset.Platform, metadataPayload, now,
	); err != nil {
		return assets.Asset{}, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE agent_tokens
		 SET status = 'revoked', revoked_at = COALESCE(revoked_at, clock_timestamp())
		 WHERE asset_id = $1 AND status = 'active'`,
		assetID,
	); err != nil {
		return assets.Asset{}, err
	}
	if tag, err := tx.Exec(ctx,
		`UPDATE agent_tokens
		 SET status = 'active', expires_at = clock_timestamp() + ($2::double precision * interval '1 second')
		 WHERE id = $1 AND status = 'pending' AND revoked_at IS NULL AND expires_at > clock_timestamp()`,
		prepared.ID, agentTokenTTL.Seconds(),
	); err != nil {
		return assets.Asset{}, err
	} else if tag.RowsAffected() != 1 {
		return assets.Asset{}, ErrPreparedAgentApprovalNotFound
	}
	if _, err := markAgentIdentityRotated(ctx, tx, assetID); err != nil {
		return assets.Asset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return assets.Asset{}, err
	}
	return asset, nil
}

func (s *PostgresStore) CancelAgentApproval(ctx context.Context, preparedTokenID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE agent_tokens SET status = 'revoked', revoked_at = COALESCE(revoked_at, clock_timestamp())
		 WHERE id = $1 AND status = 'pending'`,
		strings.TrimSpace(preparedTokenID),
	)
	return err
}

const maxPreparedAgentApprovalTTL = 5 * time.Minute

func boundedPreparedApprovalExpiry(requested, now time.Time) time.Time {
	now = now.UTC()
	maximum := now.Add(maxPreparedAgentApprovalTTL)
	requested = requested.UTC()
	if requested.IsZero() || !requested.After(now) || requested.After(maximum) {
		return maximum
	}
	return requested
}
