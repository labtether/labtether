package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/terminal"
)

func (s *PostgresStore) CreateBookmark(req terminal.CreateBookmarkRequest) (terminal.Bookmark, error) {
	now := time.Now().UTC()

	tagsPayload, err := marshalStringSlice(req.Tags)
	if err != nil {
		return terminal.Bookmark{}, err
	}

	id := idgen.New("bkm")
	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO terminal_session_bookmarks (
			id, actor_id, title, asset_id, host, port, username,
			credential_profile_id, jump_chain_group_id, tags, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $11)`,
		id,
		strings.TrimSpace(req.ActorID),
		strings.TrimSpace(req.Title),
		nullIfBlank(req.AssetID),
		nullIfBlank(req.Host),
		req.Port,
		nullIfBlank(req.Username),
		nullIfBlank(req.CredentialProfileID),
		nullIfBlank(req.JumpChainGroupID),
		tagsPayload,
		now,
	)
	if err != nil {
		return terminal.Bookmark{}, err
	}

	bkm, ok, err := s.GetBookmark(id)
	if err != nil {
		return terminal.Bookmark{}, err
	}
	if !ok {
		return terminal.Bookmark{}, ErrNotFound
	}
	return bkm, nil
}

func (s *PostgresStore) GetBookmark(id string) (terminal.Bookmark, bool, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, actor_id, title, asset_id, host, port, username,
		        credential_profile_id, jump_chain_group_id, tags, created_at, updated_at, last_used_at
		 FROM terminal_session_bookmarks WHERE id = $1`,
		strings.TrimSpace(id),
	)
	bkm, err := scanBookmark(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return terminal.Bookmark{}, false, nil
		}
		return terminal.Bookmark{}, false, err
	}
	return bkm, true, nil
}

func (s *PostgresStore) ListBookmarks(actorID string) ([]terminal.Bookmark, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, actor_id, title, asset_id, host, port, username,
		        credential_profile_id, jump_chain_group_id, tags, created_at, updated_at, last_used_at
		 FROM terminal_session_bookmarks
		 WHERE actor_id = $1
		 ORDER BY last_used_at DESC NULLS LAST, created_at DESC`,
		strings.TrimSpace(actorID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]terminal.Bookmark, 0, 32)
	for rows.Next() {
		bkm, scanErr := scanBookmark(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, bkm)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) UpdateBookmark(id string, req terminal.UpdateBookmarkRequest) (terminal.Bookmark, error) {
	existing, ok, err := s.GetBookmark(id)
	if err != nil {
		return terminal.Bookmark{}, err
	}
	if !ok {
		return terminal.Bookmark{}, ErrNotFound
	}

	if req.Title != nil {
		existing.Title = strings.TrimSpace(*req.Title)
	}
	if req.AssetID != nil {
		existing.AssetID = strings.TrimSpace(*req.AssetID)
	}
	if req.Host != nil {
		existing.Host = strings.TrimSpace(*req.Host)
	}
	if req.Port != nil {
		existing.Port = req.Port
	}
	if req.Username != nil {
		existing.Username = strings.TrimSpace(*req.Username)
	}
	if req.CredentialProfileID != nil {
		existing.CredentialProfileID = strings.TrimSpace(*req.CredentialProfileID)
	}
	if req.JumpChainGroupID != nil {
		existing.JumpChainGroupID = strings.TrimSpace(*req.JumpChainGroupID)
	}
	if req.Tags != nil {
		existing.Tags = req.Tags
	}

	tagsPayload, err := marshalStringSlice(existing.Tags)
	if err != nil {
		return terminal.Bookmark{}, err
	}

	now := time.Now().UTC()
	_, err = s.pool.Exec(context.Background(),
		`UPDATE terminal_session_bookmarks
		 SET title = $2, asset_id = $3, host = $4, port = $5, username = $6,
		     credential_profile_id = $7, jump_chain_group_id = $8, tags = $9::jsonb, updated_at = $10
		 WHERE id = $1`,
		id,
		existing.Title,
		nullIfBlank(existing.AssetID),
		nullIfBlank(existing.Host),
		existing.Port,
		nullIfBlank(existing.Username),
		nullIfBlank(existing.CredentialProfileID),
		nullIfBlank(existing.JumpChainGroupID),
		tagsPayload,
		now,
	)
	if err != nil {
		return terminal.Bookmark{}, err
	}

	existing.UpdatedAt = now
	return existing, nil
}

func (s *PostgresStore) DeleteBookmark(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM terminal_session_bookmarks WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) TouchBookmarkLastUsed(id string, at time.Time) error {
	_, err := s.pool.Exec(context.Background(),
		`UPDATE terminal_session_bookmarks SET last_used_at = $2 WHERE id = $1`,
		strings.TrimSpace(id),
		at.UTC(),
	)
	return err
}

type bookmarkScanner interface {
	Scan(dest ...any) error
}

func scanBookmark(scanner bookmarkScanner) (terminal.Bookmark, error) {
	var bkm terminal.Bookmark
	var assetID, host, username, credentialProfileID, jumpChainGroupID *string
	var tagsPayload []byte
	var lastUsedAt *time.Time

	err := scanner.Scan(
		&bkm.ID,
		&bkm.ActorID,
		&bkm.Title,
		&assetID,
		&host,
		&bkm.Port,
		&username,
		&credentialProfileID,
		&jumpChainGroupID,
		&tagsPayload,
		&bkm.CreatedAt,
		&bkm.UpdatedAt,
		&lastUsedAt,
	)
	if err != nil {
		return terminal.Bookmark{}, err
	}

	if assetID != nil {
		bkm.AssetID = *assetID
	}
	if host != nil {
		bkm.Host = *host
	}
	if username != nil {
		bkm.Username = *username
	}
	if credentialProfileID != nil {
		bkm.CredentialProfileID = *credentialProfileID
	}
	if jumpChainGroupID != nil {
		bkm.JumpChainGroupID = *jumpChainGroupID
	}
	bkm.Tags = unmarshalStringSlice(tagsPayload)
	if bkm.Tags == nil {
		bkm.Tags = []string{}
	}
	bkm.LastUsedAt = lastUsedAt
	return bkm, nil
}
