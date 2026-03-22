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

func (s *PostgresStore) CreateOrUpdatePersistentSession(req terminal.CreatePersistentSessionRequest) (terminal.PersistentSession, error) {
	now := time.Now().UTC()
	target := strings.TrimSpace(req.Target)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = target
	}

	// Try to find existing session for this actor+target first.
	existing, found, err := s.getPersistentSessionByActorAndTarget(req.ActorID, target)
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	if found {
		// Update title and bookmark_id if provided; return existing session.
		_, updateErr := s.pool.Exec(context.Background(),
			`UPDATE terminal_persistent_sessions
			 SET title = $2, bookmark_id = COALESCE(NULLIF($3, ''), bookmark_id), updated_at = $4
			 WHERE id = $1`,
			existing.ID, title, req.BookmarkID, now,
		)
		if updateErr != nil {
			return terminal.PersistentSession{}, updateErr
		}
		updated, ok, readErr := s.GetPersistentSession(existing.ID)
		if readErr != nil || !ok {
			return existing, readErr
		}
		return updated, nil
	}

	// No existing session — create new one.
	id := idgen.New("pts")
	tmuxSessionName := persistentTmuxSessionName(id)
	bookmarkID := strings.TrimSpace(req.BookmarkID)

	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO terminal_persistent_sessions (
			id, actor_id, target, title, status, tmux_session_name, bookmark_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, 'detached', $5, NULLIF($6, ''), $7, $8)`,
		id, req.ActorID, target, title, tmuxSessionName, bookmarkID, now, now,
	)
	if err != nil {
		return terminal.PersistentSession{}, err
	}

	persistent, ok, readErr := s.GetPersistentSession(id)
	if readErr != nil {
		return terminal.PersistentSession{}, readErr
	}
	if !ok {
		return terminal.PersistentSession{}, ErrNotFound
	}
	return persistent, nil
}

func (s *PostgresStore) MarkPersistentSessionArchived(id string, archivedAt time.Time) (terminal.PersistentSession, error) {
	result, err := s.pool.Exec(context.Background(),
		`UPDATE terminal_persistent_sessions
		 SET status = 'archived', archived_at = $2, updated_at = $2
		 WHERE id = $1`,
		id, archivedAt,
	)
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	if result.RowsAffected() == 0 {
		return terminal.PersistentSession{}, ErrNotFound
	}
	persistent, ok, err := s.GetPersistentSession(id)
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	if !ok {
		return terminal.PersistentSession{}, ErrNotFound
	}
	return persistent, nil
}

func (s *PostgresStore) ListDetachedOlderThan(threshold time.Time) ([]terminal.PersistentSession, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, actor_id, target, title, status, tmux_session_name, created_at, updated_at,
		        last_attached_at, last_detached_at, bookmark_id, archived_at, archive_after_days, pinned
		 FROM terminal_persistent_sessions
		 WHERE status = 'detached'
		   AND pinned = false
		   AND last_detached_at < $1
		 ORDER BY last_detached_at ASC`,
		threshold,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]terminal.PersistentSession, 0, 16)
	for rows.Next() {
		persistent, scanErr := scanPersistentSession(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, persistent)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) ListAttachedSessions() ([]terminal.PersistentSession, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, actor_id, target, title, status, tmux_session_name, created_at, updated_at,
		        last_attached_at, last_detached_at, bookmark_id, archived_at, archive_after_days, pinned
		 FROM terminal_persistent_sessions
		 WHERE status = 'attached'
		 ORDER BY last_attached_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]terminal.PersistentSession, 0, 16)
	for rows.Next() {
		persistent, scanErr := scanPersistentSession(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, persistent)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) MarkAllAttachedAsDetached() error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(context.Background(),
		`UPDATE terminal_persistent_sessions
		 SET status = 'detached', last_detached_at = $1, updated_at = $1
		 WHERE status = 'attached'`,
		now,
	)
	return err
}

func (s *PostgresStore) GetPersistentSession(id string) (terminal.PersistentSession, bool, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, actor_id, target, title, status, tmux_session_name, created_at, updated_at,
		        last_attached_at, last_detached_at, bookmark_id, archived_at, archive_after_days, pinned
		 FROM terminal_persistent_sessions WHERE id = $1`,
		id,
	)
	persistent, err := scanPersistentSession(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return terminal.PersistentSession{}, false, nil
		}
		return terminal.PersistentSession{}, false, err
	}
	return persistent, true, nil
}

func (s *PostgresStore) ListPersistentSessions() ([]terminal.PersistentSession, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, actor_id, target, title, status, tmux_session_name, created_at, updated_at,
		        last_attached_at, last_detached_at, bookmark_id, archived_at, archive_after_days, pinned
		 FROM terminal_persistent_sessions
		 ORDER BY updated_at DESC, created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]terminal.PersistentSession, 0, 32)
	for rows.Next() {
		persistent, err := scanPersistentSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, persistent)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) UpdatePersistentSession(id string, req terminal.UpdatePersistentSessionRequest) (terminal.PersistentSession, error) {
	persistent, ok, err := s.GetPersistentSession(id)
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	if !ok {
		return terminal.PersistentSession{}, ErrNotFound
	}

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			title = persistent.Target
		}
		persistent.Title = title
	}
	if req.Status != nil {
		persistent.Status = strings.TrimSpace(*req.Status)
	}
	persistent.UpdatedAt = time.Now().UTC()

	_, err = s.pool.Exec(context.Background(),
		`UPDATE terminal_persistent_sessions
		 SET title = $2, status = $3, updated_at = $4
		 WHERE id = $1`,
		id, persistent.Title, persistent.Status, persistent.UpdatedAt,
	)
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	return persistent, nil
}

func (s *PostgresStore) MarkPersistentSessionAttached(id string, attachedAt time.Time) (terminal.PersistentSession, error) {
	_, err := s.pool.Exec(context.Background(),
		`UPDATE terminal_persistent_sessions
		 SET status = 'attached', last_attached_at = $2, updated_at = $2
		 WHERE id = $1`,
		id, attachedAt,
	)
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	persistent, ok, err := s.GetPersistentSession(id)
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	if !ok {
		return terminal.PersistentSession{}, ErrNotFound
	}
	return persistent, nil
}

func (s *PostgresStore) MarkPersistentSessionDetached(id string, detachedAt time.Time) (terminal.PersistentSession, error) {
	_, err := s.pool.Exec(context.Background(),
		`UPDATE terminal_persistent_sessions
		 SET status = 'detached', last_detached_at = $2, updated_at = $2
		 WHERE id = $1`,
		id, detachedAt,
	)
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	persistent, ok, err := s.GetPersistentSession(id)
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	if !ok {
		return terminal.PersistentSession{}, ErrNotFound
	}
	return persistent, nil
}

func (s *PostgresStore) DeletePersistentSession(id string) error {
	result, err := s.pool.Exec(context.Background(), `DELETE FROM terminal_persistent_sessions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) getPersistentSessionByActorAndTarget(actorID, target string) (terminal.PersistentSession, bool, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, actor_id, target, title, status, tmux_session_name, created_at, updated_at,
		        last_attached_at, last_detached_at, bookmark_id, archived_at, archive_after_days, pinned
		 FROM terminal_persistent_sessions
		 WHERE actor_id = $1 AND target = $2`,
		actorID, target,
	)
	persistent, err := scanPersistentSession(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return terminal.PersistentSession{}, false, nil
		}
		return terminal.PersistentSession{}, false, err
	}
	return persistent, true, nil
}

type persistentSessionScanner interface {
	Scan(dest ...any) error
}

func scanPersistentSession(scanner persistentSessionScanner) (terminal.PersistentSession, error) {
	var persistent terminal.PersistentSession
	var lastAttachedAt *time.Time
	var lastDetachedAt *time.Time
	var bookmarkID *string
	err := scanner.Scan(
		&persistent.ID,
		&persistent.ActorID,
		&persistent.Target,
		&persistent.Title,
		&persistent.Status,
		&persistent.TmuxSessionName,
		&persistent.CreatedAt,
		&persistent.UpdatedAt,
		&lastAttachedAt,
		&lastDetachedAt,
		&bookmarkID,
		&persistent.ArchivedAt,
		&persistent.ArchiveAfterDays,
		&persistent.Pinned,
	)
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	persistent.LastAttachedAt = lastAttachedAt
	persistent.LastDetachedAt = lastDetachedAt
	if bookmarkID != nil {
		persistent.BookmarkID = *bookmarkID
	}
	return persistent, nil
}

func persistentTmuxSessionName(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return "lt-shell"
	}
	name := "lt-" + trimmed
	if len(name) > 24 {
		name = name[:24]
	}
	return name
}
