package terminal

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TerminalOutputRecorder writes terminal output chunks to the
// terminal_output_buffer table for later replay (e.g. when a browser
// reconnects to a tmux-backed session).
type TerminalOutputRecorder struct {
	pool      *pgxpool.Pool
	sessionID string
}

// NewTerminalOutputRecorder creates a recorder bound to a specific terminal session.
func NewTerminalOutputRecorder(pool *pgxpool.Pool, sessionID string) *TerminalOutputRecorder {
	return &TerminalOutputRecorder{
		pool:      pool,
		sessionID: sessionID,
	}
}

// RecordChunk persists a single output chunk to the database.
// Called from the terminal stream bridge when output is sent to the browser.
func (r *TerminalOutputRecorder) RecordChunk(data []byte) {
	if r == nil || r.pool == nil || len(data) == 0 {
		return
	}
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO terminal_output_buffer (session_id, data) VALUES ($1, $2)`,
		r.sessionID, data,
	)
	if err != nil {
		log.Printf("terminal-buffer: failed to record chunk for session %s: %v", r.sessionID, err)
	}
}

// ReplayBuffer reads stored output chunks from the terminal_output_buffer for
// a given session, ordered by sequence number. Returns up to 1000 chunks
// (roughly equivalent to 5000 lines of terminal output).
func ReplayBuffer(ctx context.Context, pool *pgxpool.Pool, sessionID string) ([][]byte, error) {
	rows, err := pool.Query(ctx,
		`SELECT data FROM terminal_output_buffer WHERE session_id = $1 ORDER BY seq LIMIT 1000`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks [][]byte
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return chunks, err
		}
		chunks = append(chunks, data)
	}
	return chunks, rows.Err()
}

// CleanupExpiredBuffers deletes terminal output buffer entries older than 24 hours.
// Intended to be called from the worker retention loop.
func CleanupExpiredBuffers(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx,
		`DELETE FROM terminal_output_buffer WHERE recorded_at < NOW() - INTERVAL '24 hours'`,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
