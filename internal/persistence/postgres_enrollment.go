package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/idgen"
)

func (s *PostgresStore) CreateEnrollmentToken(tokenHash, label string, expiresAt time.Time, maxUses int) (enrollment.EnrollmentToken, error) {
	now := time.Now().UTC()
	tok := enrollment.EnrollmentToken{
		ID:        idgen.New("etok"),
		Label:     label,
		ExpiresAt: expiresAt,
		MaxUses:   maxUses,
		UseCount:  0,
		CreatedAt: now,
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO enrollment_tokens (id, token_hash, label, expires_at, max_uses, use_count, created_at)
		 VALUES ($1, $2, $3, $4, $5, 0, $6)`,
		tok.ID, tokenHash, tok.Label, tok.ExpiresAt, tok.MaxUses, tok.CreatedAt,
	)
	return tok, err
}

func (s *PostgresStore) ValidateEnrollmentToken(tokenHash string) (enrollment.EnrollmentToken, bool, error) {
	var tok enrollment.EnrollmentToken
	var revokedAt *time.Time
	err := s.pool.QueryRow(context.Background(),
		`SELECT id, label, expires_at, max_uses, use_count, created_at, revoked_at
		 FROM enrollment_tokens WHERE token_hash = $1`, tokenHash,
	).Scan(&tok.ID, &tok.Label, &tok.ExpiresAt, &tok.MaxUses, &tok.UseCount, &tok.CreatedAt, &revokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tok, false, nil
		}
		return tok, false, err
	}
	tok.RevokedAt = revokedAt

	now := time.Now().UTC()
	if revokedAt != nil {
		return tok, false, nil
	}
	if now.After(tok.ExpiresAt) {
		return tok, false, nil
	}
	if tok.MaxUses > 0 && tok.UseCount >= tok.MaxUses {
		return tok, false, nil
	}
	return tok, true, nil
}

func (s *PostgresStore) ConsumeEnrollmentToken(tokenHash string) (enrollment.EnrollmentToken, bool, error) {
	var tok enrollment.EnrollmentToken
	var revokedAt *time.Time
	now := time.Now().UTC()

	err := s.pool.QueryRow(context.Background(),
		`UPDATE enrollment_tokens
		 SET use_count = use_count + 1
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > $2
		   AND (max_uses <= 0 OR use_count < max_uses)
		 RETURNING id, label, expires_at, max_uses, use_count, created_at, revoked_at`,
		tokenHash,
		now,
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
	_, err := s.pool.Exec(context.Background(),
		`UPDATE enrollment_tokens SET use_count = use_count + 1 WHERE id = $1`, id,
	)
	return err
}

func (s *PostgresStore) RevokeEnrollmentToken(id string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(context.Background(),
		`UPDATE enrollment_tokens SET revoked_at = $1 WHERE id = $2 AND revoked_at IS NULL`, now, id,
	)
	return err
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
	now := time.Now().UTC()
	tok := enrollment.AgentToken{
		ID:          idgen.New("atok"),
		AssetID:     assetID,
		Status:      "active",
		EnrolledVia: enrolledVia,
		ExpiresAt:   expiresAt.UTC(),
		CreatedAt:   now,
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO agent_tokens (id, asset_id, token_hash, status, enrolled_via, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tok.ID, tok.AssetID, tokenHash, tok.Status, tok.EnrolledVia, tok.ExpiresAt, tok.CreatedAt,
	)
	return tok, err
}

func (s *PostgresStore) ValidateAgentToken(tokenHash string) (enrollment.AgentToken, bool, error) {
	var tok enrollment.AgentToken
	var lastUsedAt, revokedAt *time.Time
	now := time.Now().UTC()
	err := s.pool.QueryRow(context.Background(),
		`SELECT id, asset_id, status, enrolled_via, expires_at, last_used_at, created_at, revoked_at
		 FROM agent_tokens WHERE token_hash = $1 AND status = 'active' AND expires_at > $2`, tokenHash, now,
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
	now := time.Now().UTC()
	_, err := s.pool.Exec(context.Background(),
		`UPDATE agent_tokens SET last_used_at = $1 WHERE id = $2`, now, id,
	)
	return err
}

func (s *PostgresStore) RevokeAgentToken(id string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(context.Background(),
		`UPDATE agent_tokens SET status = 'revoked', revoked_at = $1 WHERE id = $2 AND status = 'active'`, now, id,
	)
	return err
}

func (s *PostgresStore) RevokeAgentTokensByAsset(assetID string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(context.Background(),
		`UPDATE agent_tokens SET status = 'revoked', revoked_at = $1 WHERE asset_id = $2 AND status = 'active'`, now, assetID,
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
		    OR expires_at < NOW()
		    OR (max_uses > 0 AND use_count >= max_uses)`)
	if err != nil {
		return 0, 0, err
	}

	// Delete agent tokens that are revoked and were never used by a device.
	agentTag, err := tx.Exec(ctx,
		`DELETE FROM agent_tokens
		 WHERE status = 'revoked'
		   AND last_used_at IS NULL`)
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
