package persistence

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
)

func (s *PostgresStore) CreateLinkSuggestion(sourceAssetID, targetAssetID, matchReason string, confidence float64) (LinkSuggestion, error) {
	now := time.Now().UTC()
	source := strings.TrimSpace(sourceAssetID)
	target := strings.TrimSpace(targetAssetID)

	row := s.pool.QueryRow(context.Background(),
		`INSERT INTO asset_link_suggestions (
			id, source_asset_id, target_asset_id, match_reason,
			confidence, status, created_at
		)
		VALUES ($1, $2, $3, $4, $5, 'pending', $6)
		ON CONFLICT (source_asset_id, target_asset_id) DO NOTHING
		RETURNING id, source_asset_id, target_asset_id, match_reason,
			confidence, status, created_at, resolved_at, resolved_by`,
		idgen.New("lsug"),
		source,
		target,
		strings.TrimSpace(matchReason),
		confidence,
		now,
	)

	sug, err := scanLinkSuggestion(row)
	if err != nil {
		// ON CONFLICT DO NOTHING returns no rows — fetch the existing one.
		if err == pgx.ErrNoRows {
			return s.getLinkSuggestionByPair(source, target)
		}
		return LinkSuggestion{}, err
	}
	return sug, nil
}

func (s *PostgresStore) getLinkSuggestionByPair(sourceAssetID, targetAssetID string) (LinkSuggestion, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, source_asset_id, target_asset_id, match_reason,
			confidence, status, created_at, resolved_at, resolved_by
		 FROM asset_link_suggestions
		 WHERE source_asset_id = $1 AND target_asset_id = $2`,
		sourceAssetID, targetAssetID,
	)
	return scanLinkSuggestion(row)
}

func (s *PostgresStore) ListPendingLinkSuggestions() ([]LinkSuggestion, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, source_asset_id, target_asset_id, match_reason,
			confidence, status, created_at, resolved_at, resolved_by
		 FROM asset_link_suggestions
		 WHERE status = 'pending'
		 ORDER BY confidence DESC, created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]LinkSuggestion, 0, 32)
	for rows.Next() {
		sug, scanErr := scanLinkSuggestion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, sug)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ResolveLinkSuggestion(id, status, resolvedBy string) error {
	now := time.Now().UTC()
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE asset_link_suggestions
		 SET status = $2, resolved_at = $3, resolved_by = $4
		 WHERE id = $1`,
		strings.TrimSpace(id),
		strings.TrimSpace(status),
		now,
		strings.TrimSpace(resolvedBy),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type linkSuggestionScanner interface {
	Scan(dest ...any) error
}

func scanLinkSuggestion(row linkSuggestionScanner) (LinkSuggestion, error) {
	var sug LinkSuggestion
	var resolvedBy *string
	if err := row.Scan(
		&sug.ID,
		&sug.SourceAssetID,
		&sug.TargetAssetID,
		&sug.MatchReason,
		&sug.Confidence,
		&sug.Status,
		&sug.CreatedAt,
		&sug.ResolvedAt,
		&resolvedBy,
	); err != nil {
		return LinkSuggestion{}, err
	}
	if resolvedBy != nil {
		sug.ResolvedBy = *resolvedBy
	}
	sug.CreatedAt = sug.CreatedAt.UTC()
	if sug.ResolvedAt != nil {
		t := sug.ResolvedAt.UTC()
		sug.ResolvedAt = &t
	}
	return sug, nil
}
