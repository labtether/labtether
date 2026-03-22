package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) UpsertScrollback(persistentSessionID string, buffer []byte, bufferSize int, totalLines int) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO terminal_session_scrollback (persistent_session_id, buffer, buffer_size, total_lines, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (persistent_session_id) DO UPDATE
		   SET buffer = EXCLUDED.buffer,
		       buffer_size = EXCLUDED.buffer_size,
		       total_lines = EXCLUDED.total_lines,
		       updated_at = EXCLUDED.updated_at`,
		strings.TrimSpace(persistentSessionID),
		buffer,
		bufferSize,
		totalLines,
		now,
	)
	return err
}

func (s *PostgresStore) GetScrollback(persistentSessionID string) ([]byte, error) {
	var buffer []byte
	err := s.pool.QueryRow(context.Background(),
		`SELECT buffer FROM terminal_session_scrollback WHERE persistent_session_id = $1`,
		strings.TrimSpace(persistentSessionID),
	).Scan(&buffer)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return buffer, nil
}

func (s *PostgresStore) DeleteScrollback(persistentSessionID string) error {
	_, err := s.pool.Exec(context.Background(),
		`DELETE FROM terminal_session_scrollback WHERE persistent_session_id = $1`,
		strings.TrimSpace(persistentSessionID),
	)
	return err
}
