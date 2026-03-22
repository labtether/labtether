package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
)

// --- scan helpers ---

type remoteBookmarkScanner interface {
	Scan(dest ...any) error
}

func scanRemoteBookmark(row remoteBookmarkScanner) (RemoteBookmark, error) {
	bm := RemoteBookmark{}
	if err := row.Scan(
		&bm.ID,
		&bm.Label,
		&bm.Protocol,
		&bm.Host,
		&bm.Port,
		&bm.CredentialID,
		&bm.CreatedAt,
		&bm.UpdatedAt,
	); err != nil {
		return RemoteBookmark{}, err
	}
	bm.HasCredentials = bm.CredentialID != nil
	bm.CreatedAt = bm.CreatedAt.UTC()
	bm.UpdatedAt = bm.UpdatedAt.UTC()
	return bm, nil
}

// --- columns ---

const remoteBookmarkColumns = `id, label, protocol, host, port, credential_id, created_at, updated_at`

// --- store methods ---

func (s *PostgresStore) ListRemoteBookmarks(ctx context.Context) ([]RemoteBookmark, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+remoteBookmarkColumns+` FROM remote_bookmarks ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RemoteBookmark, 0, 16)
	for rows.Next() {
		bm, scanErr := scanRemoteBookmark(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, bm)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetRemoteBookmark(ctx context.Context, id string) (*RemoteBookmark, error) {
	bm, err := scanRemoteBookmark(s.pool.QueryRow(ctx,
		`SELECT `+remoteBookmarkColumns+` FROM remote_bookmarks WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &bm, nil
}

func (s *PostgresStore) CreateRemoteBookmark(ctx context.Context, bm *RemoteBookmark) error {
	now := time.Now().UTC()
	bm.ID = idgen.New("rbm")
	bm.CreatedAt = now
	bm.UpdatedAt = now

	_, err := s.pool.Exec(ctx,
		`INSERT INTO remote_bookmarks (id, label, protocol, host, port, credential_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		bm.ID,
		strings.TrimSpace(bm.Label),
		strings.TrimSpace(bm.Protocol),
		strings.TrimSpace(bm.Host),
		bm.Port,
		bm.CredentialID,
		bm.CreatedAt,
		bm.UpdatedAt,
	)
	return err
}

func (s *PostgresStore) UpdateRemoteBookmark(ctx context.Context, bm RemoteBookmark) error {
	now := time.Now().UTC()
	bm.UpdatedAt = now

	tag, err := s.pool.Exec(ctx,
		`UPDATE remote_bookmarks
		 SET label = $2, protocol = $3, host = $4, port = $5, credential_id = $6, updated_at = $7
		 WHERE id = $1`,
		strings.TrimSpace(bm.ID),
		strings.TrimSpace(bm.Label),
		strings.TrimSpace(bm.Protocol),
		strings.TrimSpace(bm.Host),
		bm.Port,
		bm.CredentialID,
		bm.UpdatedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteRemoteBookmark(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM remote_bookmarks WHERE id = $1`,
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
