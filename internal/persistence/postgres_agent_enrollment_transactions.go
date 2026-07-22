package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/idgen"
)

const selectAssetForAgentIdentityUpdate = `SELECT id, type, name, source, group_id, status, platform, metadata, tags, created_at, updated_at, last_seen_at, host, transport_type
	FROM assets WHERE id = $1 FOR UPDATE`

// lockAgentIdentityAsset serializes every transition that can create, rotate,
// or remove an agent identity. Keep this key shared with routine heartbeats so
// an old bearer cannot race a continuity decision inside one hub instance.
func lockAgentIdentityAsset(ctx context.Context, tx pgx.Tx, assetID string) error {
	_, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"agent-identity:"+strings.TrimSpace(assetID),
	)
	return err
}

func (s *PostgresStore) CommitAgentEnrollment(ctx context.Context, req AgentEnrollmentCommitRequest) (AgentEnrollmentCommitResult, error) {
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		return AgentEnrollmentCommitResult{}, fmt.Errorf("asset id is required")
	}
	req.AssetID = assetID
	agentTokenTTL, err := databaseTTL(req.AgentTokenExpiresAt)
	if err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAgentIdentityAsset(ctx, tx, assetID); err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	if err := lockAgentEnrollmentIssuance(ctx, tx); err != nil {
		return AgentEnrollmentCommitResult{}, err
	}

	existing, exists, err := selectAgentIdentityAsset(ctx, tx, assetID)
	if err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	etok, valid, err := selectValidEnrollmentTokenForUpdate(ctx, tx, req.EnrollmentTokenHash)
	if err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	if !valid {
		return AgentEnrollmentCommitResult{}, ErrEnrollmentTokenInvalid
	}

	if exists {
		if strings.TrimSpace(req.DeviceProofVersion) != enrollment.DeviceProofVersionV2 {
			return AgentEnrollmentCommitResult{}, ErrAgentIdentityProofV2Required
		}
		if etok.MaxUses != 1 {
			return AgentEnrollmentCommitResult{}, ErrRecoveryRequiresSingleUseToken
		}
		storedFingerprint := strings.TrimSpace(existing.Metadata[assets.MetadataKeyAgentDeviceFingerprint])
		storedAlgorithm := strings.TrimSpace(existing.Metadata[assets.MetadataKeyAgentDeviceKeyAlgorithm])
		if storedFingerprint == "" || storedAlgorithm == "" ||
			storedFingerprint != strings.TrimSpace(req.DeviceFingerprint) ||
			storedAlgorithm != strings.TrimSpace(req.DeviceKeyAlgorithm) {
			return AgentEnrollmentCommitResult{}, ErrAgentIdentityContinuityConflict
		}
		marker, err := selectAgentIdentityMarkerForUpdate(ctx, tx, assetID)
		if err != nil {
			return AgentEnrollmentCommitResult{}, err
		}
		if !etok.CreatedAt.After(marker) {
			return AgentEnrollmentCommitResult{}, ErrEnrollmentTokenPredatesRotation
		}
	} else {
		if err := validateInitialIdentityFields(req); err != nil {
			return AgentEnrollmentCommitResult{}, err
		}
		if err := ensureAgentFleetCapacity(ctx, tx, req.MaxEnrolledAgents); err != nil {
			return AgentEnrollmentCommitResult{}, err
		}
		req.GroupID, err = resolveInitialEnrollmentGroupID(ctx, tx, req.GroupID)
		if err != nil {
			return AgentEnrollmentCommitResult{}, err
		}
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE enrollment_tokens SET use_count = use_count + 1 WHERE id = $1`, etok.ID); err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	etok.UseCount++

	if !exists {
		existing = buildInitialEnrolledAsset(req, now)
		metadataPayload, err := marshalStringMap(existing.Metadata)
		if err != nil {
			return AgentEnrollmentCommitResult{}, err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO assets (id, type, name, source, group_id, status, platform, metadata, created_at, updated_at, last_seen_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $9, $9)`,
			existing.ID, existing.Type, existing.Name, existing.Source, nullIfBlank(existing.GroupID),
			existing.Status, existing.Platform, metadataPayload, now,
		); err != nil {
			return AgentEnrollmentCommitResult{}, err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO asset_heartbeats (id, asset_id, source, status, metadata, received_at)
			 VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
			idgen.New("hb"), existing.ID, existing.Source, existing.Status, metadataPayload, now,
		); err != nil {
			return AgentEnrollmentCommitResult{}, err
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE agent_tokens
		 SET status = 'revoked', revoked_at = COALESCE(revoked_at, clock_timestamp())
		 WHERE asset_id = $1 AND status = 'active'`,
		assetID,
	); err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	agentToken := enrollment.AgentToken{
		ID:          idgen.New("atok"),
		AssetID:     assetID,
		Status:      "active",
		EnrolledVia: etok.ID,
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO agent_tokens (id, asset_id, token_hash, status, enrolled_via, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, clock_timestamp() + ($6::double precision * interval '1 second'), clock_timestamp())
		 RETURNING expires_at, created_at`,
		agentToken.ID, agentToken.AssetID, req.AgentTokenHash, agentToken.Status,
		agentToken.EnrolledVia, agentTokenTTL.Seconds(),
	).Scan(&agentToken.ExpiresAt, &agentToken.CreatedAt); err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	if _, err := markAgentIdentityRotated(ctx, tx, assetID); err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	return AgentEnrollmentCommitResult{
		EnrollmentToken: etok,
		AgentToken:      agentToken,
		Asset:           existing,
		Recovery:        exists,
	}, nil
}

// resolveInitialEnrollmentGroupID preserves a valid operator-selected
// placement while treating an agent's stale or unknown group id as unplaced.
// FOR KEY SHARE keeps a validated group from being deleted before the asset
// insert commits, so token consumption and placement remain one transaction.
func resolveInitialEnrollmentGroupID(ctx context.Context, tx pgx.Tx, groupID string) (string, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return "", nil
	}
	var resolved string
	err := tx.QueryRow(ctx, `SELECT id FROM groups WHERE id = $1 FOR KEY SHARE`, groupID).Scan(&resolved)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return resolved, nil
}

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

func (s *PostgresStore) DecommissionAgentAsset(ctx context.Context, assetID string) error {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockAgentIdentityAsset(ctx, tx, assetID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE agent_tokens SET status = 'revoked', revoked_at = COALESCE(revoked_at, clock_timestamp())
		 WHERE asset_id = $1 AND status IN ('active', 'pending')`,
		assetID,
	); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) CommitAuthenticatedAgentHeartbeat(ctx context.Context, agentTokenID string, req assets.HeartbeatRequest) (assets.Asset, error) {
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		return assets.Asset{}, ErrAgentCredentialInactive
	}
	req.AssetID = assetID
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return assets.Asset{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAgentIdentityAsset(ctx, tx, assetID); err != nil {
		return assets.Asset{}, err
	}
	var tokenAssetID string
	err = tx.QueryRow(ctx,
		`SELECT asset_id
		 FROM agent_tokens
		 WHERE id = $1 AND status = 'active' AND revoked_at IS NULL AND expires_at > clock_timestamp()
		 FOR UPDATE`,
		strings.TrimSpace(agentTokenID),
	).Scan(&tokenAssetID)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && tokenAssetID != assetID {
		return assets.Asset{}, ErrAgentCredentialInactive
	}
	if err != nil {
		return assets.Asset{}, err
	}
	var existingAssetID string
	var existingGroupID *string
	if err := tx.QueryRow(ctx, `SELECT id, group_id FROM assets WHERE id = $1 FOR UPDATE`, assetID).Scan(&existingAssetID, &existingGroupID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return assets.Asset{}, ErrAgentCredentialInactive
		}
		return assets.Asset{}, err
	}
	req.AllowAgentIdentityTOFU = true
	req.Source = "agent"
	req.GroupID = ""
	if existingGroupID != nil {
		req.GroupID = *existingGroupID
	}
	asset, err := upsertAssetHeartbeatTx(ctx, tx, req, time.Now().UTC())
	if err != nil {
		return assets.Asset{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE agent_tokens SET last_used_at = clock_timestamp() WHERE id = $1`, strings.TrimSpace(agentTokenID)); err != nil {
		return assets.Asset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return assets.Asset{}, err
	}
	return asset, nil
}

func (s *PostgresStore) CommitExistingOwnerAgentHeartbeat(ctx context.Context, req assets.HeartbeatRequest) (assets.Asset, error) {
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		return assets.Asset{}, ErrNotFound
	}
	req.AssetID = assetID
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return assets.Asset{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAgentIdentityAsset(ctx, tx, assetID); err != nil {
		return assets.Asset{}, err
	}
	var foundID string
	var existingGroupID *string
	if err := tx.QueryRow(ctx, `SELECT id, group_id FROM assets WHERE id = $1 FOR UPDATE`, assetID).Scan(&foundID, &existingGroupID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return assets.Asset{}, ErrNotFound
		}
		return assets.Asset{}, err
	}
	req.Source = "agent"
	req.GroupID = ""
	if existingGroupID != nil {
		req.GroupID = *existingGroupID
	}
	asset, err := upsertAssetHeartbeatTx(ctx, tx, req, time.Now().UTC())
	if err != nil {
		return assets.Asset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return assets.Asset{}, err
	}
	return asset, nil
}

func (s *PostgresStore) ValidateActiveAgentTokenID(ctx context.Context, agentTokenID, assetID string) error {
	var foundID string
	err := s.pool.QueryRow(ctx,
		`SELECT t.id
		 FROM agent_tokens t
		 JOIN assets a ON a.id = t.asset_id
		 WHERE t.id = $1 AND t.asset_id = $2 AND t.status = 'active' AND t.revoked_at IS NULL AND t.expires_at > clock_timestamp()`,
		strings.TrimSpace(agentTokenID), strings.TrimSpace(assetID),
	).Scan(&foundID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAgentCredentialInactive
	}
	return err
}

func selectAgentIdentityAsset(ctx context.Context, tx pgx.Tx, assetID string) (assets.Asset, bool, error) {
	asset, err := scanAsset(tx.QueryRow(ctx, selectAssetForAgentIdentityUpdate, assetID))
	if errors.Is(err, pgx.ErrNoRows) {
		return assets.Asset{}, false, nil
	}
	if err != nil {
		return assets.Asset{}, false, err
	}
	return asset, true, nil
}

func selectValidEnrollmentTokenForUpdate(ctx context.Context, tx pgx.Tx, tokenHash string) (enrollment.EnrollmentToken, bool, error) {
	var token enrollment.EnrollmentToken
	var revokedAt *time.Time
	err := tx.QueryRow(ctx,
		`SELECT id, label, expires_at, max_uses, use_count, created_at, revoked_at
		 FROM enrollment_tokens
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > clock_timestamp()
		   AND max_uses BETWEEN 1 AND $2
		   AND use_count < max_uses
		 FOR UPDATE`,
		strings.TrimSpace(tokenHash), enrollment.HardTokenMaxUsesCeiling,
	).Scan(&token.ID, &token.Label, &token.ExpiresAt, &token.MaxUses, &token.UseCount, &token.CreatedAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return enrollment.EnrollmentToken{}, false, nil
	}
	if err != nil {
		return enrollment.EnrollmentToken{}, false, err
	}
	token.RevokedAt = revokedAt
	return token, true, nil
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
