package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/idgen"
)

func (s *PostgresStore) CreateEnrollmentToken(tokenHash, label string, expiresAt time.Time, maxUses int) (enrollment.EnrollmentToken, error) {
	if err := enrollment.ValidateStoredTokenMaxUses(maxUses); err != nil {
		return enrollment.EnrollmentToken{}, err
	}
	ttl, err := databaseTTL(expiresAt)
	if err != nil {
		return enrollment.EnrollmentToken{}, err
	}
	tok := enrollment.EnrollmentToken{
		ID:       idgen.New("etok"),
		Label:    label,
		MaxUses:  maxUses,
		UseCount: 0,
	}
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return enrollment.EnrollmentToken{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAgentEnrollmentIssuance(ctx, tx); err != nil {
		return enrollment.EnrollmentToken{}, err
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO enrollment_tokens (id, token_hash, label, expires_at, max_uses, use_count, created_at)
		 VALUES ($1, $2, $3, clock_timestamp() + ($4::double precision * interval '1 second'), $5, 0, clock_timestamp())
		 RETURNING expires_at, created_at`,
		tok.ID, tokenHash, tok.Label, ttl.Seconds(), tok.MaxUses,
	).Scan(&tok.ExpiresAt, &tok.CreatedAt)
	if err != nil {
		return enrollment.EnrollmentToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return enrollment.EnrollmentToken{}, err
	}
	return tok, nil
}

func (s *PostgresStore) ValidateEnrollmentToken(tokenHash string) (enrollment.EnrollmentToken, bool, error) {
	var tok enrollment.EnrollmentToken
	var revokedAt *time.Time
	err := s.pool.QueryRow(context.Background(),
		`SELECT id, label, expires_at, max_uses, use_count, created_at, revoked_at
		 FROM enrollment_tokens
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > clock_timestamp()
		   AND max_uses BETWEEN 1 AND $2
		   AND use_count < max_uses`, tokenHash, enrollment.HardTokenMaxUsesCeiling,
	).Scan(&tok.ID, &tok.Label, &tok.ExpiresAt, &tok.MaxUses, &tok.UseCount, &tok.CreatedAt, &revokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tok, false, nil
		}
		return tok, false, err
	}
	tok.RevokedAt = revokedAt

	return tok, true, nil
}

func (s *PostgresStore) ConsumeEnrollmentToken(tokenHash string) (enrollment.EnrollmentToken, bool, error) {
	var tok enrollment.EnrollmentToken
	var revokedAt *time.Time
	err := s.pool.QueryRow(context.Background(),
		`UPDATE enrollment_tokens
		 SET use_count = use_count + 1
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > clock_timestamp()
		   AND max_uses BETWEEN 1 AND $2
		   AND use_count < max_uses
		 RETURNING id, label, expires_at, max_uses, use_count, created_at, revoked_at`,
		tokenHash, enrollment.HardTokenMaxUsesCeiling,
	).Scan(&tok.ID, &tok.Label, &tok.ExpiresAt, &tok.MaxUses, &tok.UseCount, &tok.CreatedAt, &revokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tok, false, nil
		}
		return tok, false, err
	}

	tok.RevokedAt = revokedAt
	return tok, true, nil
}

func (s *PostgresStore) IncrementEnrollmentTokenUse(id string) error {
	var foundID string
	err := s.pool.QueryRow(context.Background(),
		`UPDATE enrollment_tokens
		 SET use_count = use_count + 1
		 WHERE id = $1
		   AND revoked_at IS NULL
		   AND expires_at > clock_timestamp()
		   AND max_uses BETWEEN 1 AND $2
		   AND use_count < max_uses
		 RETURNING id`,
		id, enrollment.HardTokenMaxUsesCeiling,
	).Scan(&foundID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEnrollmentTokenInvalid
	}
	return err
}

func (s *PostgresStore) RevokeEnrollmentToken(id string) error {
	var foundID string
	err := s.pool.QueryRow(context.Background(),
		`UPDATE enrollment_tokens
		 SET revoked_at = COALESCE(revoked_at, clock_timestamp())
		 WHERE id = $1
		 RETURNING id`,
		id,
	).Scan(&foundID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *PostgresStore) ListEnrollmentTokens(limit int) ([]enrollment.EnrollmentToken, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, label, expires_at, max_uses, use_count, created_at, revoked_at
		 FROM enrollment_tokens ORDER BY created_at DESC LIMIT $1`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []enrollment.EnrollmentToken
	for rows.Next() {
		var tok enrollment.EnrollmentToken
		var revokedAt *time.Time
		if err := rows.Scan(&tok.ID, &tok.Label, &tok.ExpiresAt, &tok.MaxUses, &tok.UseCount, &tok.CreatedAt, &revokedAt); err != nil {
			return nil, err
		}
		tok.RevokedAt = revokedAt
		tokens = append(tokens, tok)
	}
	return tokens, rows.Err()
}

func (s *PostgresStore) CreateAgentToken(assetID, tokenHash, enrolledVia string, expiresAt time.Time) (enrollment.AgentToken, error) {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return enrollment.AgentToken{}, fmt.Errorf("asset id is required")
	}
	ttl, err := databaseTTL(expiresAt)
	if err != nil {
		return enrollment.AgentToken{}, err
	}
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return enrollment.AgentToken{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAgentIdentityAsset(ctx, tx, assetID); err != nil {
		return enrollment.AgentToken{}, err
	}
	if err := lockAgentEnrollmentIssuance(ctx, tx); err != nil {
		return enrollment.AgentToken{}, err
	}
	tok := enrollment.AgentToken{
		ID:          idgen.New("atok"),
		AssetID:     assetID,
		Status:      "active",
		EnrolledVia: enrolledVia,
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO agent_tokens (id, asset_id, token_hash, status, enrolled_via, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, clock_timestamp() + ($6::double precision * interval '1 second'), clock_timestamp())
		 RETURNING expires_at, created_at`,
		tok.ID, tok.AssetID, tokenHash, tok.Status, tok.EnrolledVia, ttl.Seconds(),
	).Scan(&tok.ExpiresAt, &tok.CreatedAt)
	if err != nil {
		return enrollment.AgentToken{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO agent_identity_state (asset_id, credential_rotated_at)
		 SELECT $1, clock_timestamp() FROM assets WHERE id = $1
		 ON CONFLICT (asset_id) DO UPDATE SET credential_rotated_at = clock_timestamp()`,
		assetID,
	); err != nil {
		return enrollment.AgentToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return enrollment.AgentToken{}, err
	}
	return tok, nil
}

// RotateAgentToken atomically revokes every active credential for an asset and
// inserts its replacement. This prevents a continuity-proven re-enrollment
// from leaving duplicate active credentials or an intermediate committed
// state with all tokens revoked.
func (s *PostgresStore) RotateAgentToken(assetID, tokenHash, enrolledVia string, expiresAt time.Time) (enrollment.AgentToken, error) {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return enrollment.AgentToken{}, fmt.Errorf("asset id is required")
	}
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return enrollment.AgentToken{}, err
	}
	defer tx.Rollback(ctx)

	ttl, err := databaseTTL(expiresAt)
	if err != nil {
		return enrollment.AgentToken{}, err
	}
	// Serialize rotations per asset so two distinct one-time enrollment tokens
	// cannot both commit an active replacement credential.
	if err := lockAgentIdentityAsset(ctx, tx, assetID); err != nil {
		return enrollment.AgentToken{}, err
	}
	if err := lockAgentEnrollmentIssuance(ctx, tx); err != nil {
		return enrollment.AgentToken{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE agent_tokens
		 SET status = 'revoked', revoked_at = COALESCE(revoked_at, clock_timestamp())
		 WHERE asset_id = $1 AND status = 'active'`,
		assetID,
	); err != nil {
		return enrollment.AgentToken{}, err
	}

	tok := enrollment.AgentToken{
		ID:          idgen.New("atok"),
		AssetID:     assetID,
		Status:      "active",
		EnrolledVia: enrolledVia,
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO agent_tokens (id, asset_id, token_hash, status, enrolled_via, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, clock_timestamp() + ($6::double precision * interval '1 second'), clock_timestamp())
		 RETURNING expires_at, created_at`,
		tok.ID, tok.AssetID, tokenHash, tok.Status, tok.EnrolledVia, ttl.Seconds(),
	).Scan(&tok.ExpiresAt, &tok.CreatedAt); err != nil {
		return enrollment.AgentToken{}, err
	}
	// Legacy callers may create orphan token rows for tests. Mark a rotation
	// only when the stable asset exists; production enrollment always uses the
	// transactional enrollment methods below.
	if _, err := tx.Exec(ctx,
		`INSERT INTO agent_identity_state (asset_id, credential_rotated_at)
		 SELECT $1, clock_timestamp() FROM assets WHERE id = $1
		 ON CONFLICT (asset_id) DO UPDATE SET credential_rotated_at = clock_timestamp()`,
		assetID,
	); err != nil {
		return enrollment.AgentToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return enrollment.AgentToken{}, err
	}
	return tok, nil
}

func (s *PostgresStore) ValidateAgentToken(tokenHash string) (enrollment.AgentToken, bool, error) {
	var tok enrollment.AgentToken
	var lastUsedAt, revokedAt *time.Time
	err := s.pool.QueryRow(context.Background(),
		`SELECT id, asset_id, status, enrolled_via, expires_at, last_used_at, created_at, revoked_at
		 FROM agent_tokens
		 WHERE token_hash = $1 AND status = 'active' AND revoked_at IS NULL AND expires_at > clock_timestamp()`, tokenHash,
	).Scan(&tok.ID, &tok.AssetID, &tok.Status, &tok.EnrolledVia, &tok.ExpiresAt, &lastUsedAt, &tok.CreatedAt, &revokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tok, false, nil
		}
		return tok, false, err
	}
	tok.LastUsedAt = lastUsedAt
	tok.RevokedAt = revokedAt
	return tok, true, nil
}

func (s *PostgresStore) TouchAgentTokenLastUsed(id string) error {
	_, err := s.pool.Exec(context.Background(),
		`UPDATE agent_tokens SET last_used_at = clock_timestamp() WHERE id = $1`, id,
	)
	return err
}

func (s *PostgresStore) RevokeAgentToken(id string) error {
	var foundID string
	err := s.pool.QueryRow(context.Background(),
		`UPDATE agent_tokens
		 SET status = 'revoked',
		     revoked_at = COALESCE(revoked_at, clock_timestamp())
		 WHERE id = $1
		 RETURNING id`,
		id,
	).Scan(&foundID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *PostgresStore) RevokeAgentTokensByAsset(assetID string) error {
	_, err := s.pool.Exec(context.Background(),
		`UPDATE agent_tokens SET status = 'revoked', revoked_at = clock_timestamp() WHERE asset_id = $1 AND status = 'active'`, assetID,
	)
	return err
}

func (s *PostgresStore) DeleteDeadTokens() (int, int, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	// Delete enrollment tokens that are revoked, expired, or exhausted.
	enrollTag, err := tx.Exec(ctx,
		`DELETE FROM enrollment_tokens
		 WHERE revoked_at IS NOT NULL
		    OR expires_at <= clock_timestamp()
		    OR max_uses < 1
		    OR max_uses > $1
		    OR use_count >= max_uses`,
		enrollment.HardTokenMaxUsesCeiling,
	)
	if err != nil {
		return 0, 0, err
	}

	// Delete agent tokens that are revoked and were never used by a device.
	agentTag, err := tx.Exec(ctx,
		`DELETE FROM agent_tokens
		 WHERE (status = 'revoked' AND last_used_at IS NULL)
		    OR (status = 'pending' AND expires_at <= clock_timestamp())`)
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return int(enrollTag.RowsAffected()), int(agentTag.RowsAffected()), nil
}

func (s *PostgresStore) ListAgentTokens(limit int) ([]enrollment.AgentToken, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, asset_id, status, enrolled_via, expires_at, last_used_at, created_at, revoked_at
		 FROM agent_tokens ORDER BY created_at DESC LIMIT $1`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []enrollment.AgentToken
	for rows.Next() {
		var tok enrollment.AgentToken
		var lastUsedAt, revokedAt *time.Time
		if err := rows.Scan(&tok.ID, &tok.AssetID, &tok.Status, &tok.EnrolledVia, &tok.ExpiresAt, &lastUsedAt, &tok.CreatedAt, &revokedAt); err != nil {
			return nil, err
		}
		tok.LastUsedAt = lastUsedAt
		tok.RevokedAt = revokedAt
		tokens = append(tokens, tok)
	}
	return tokens, rows.Err()
}
