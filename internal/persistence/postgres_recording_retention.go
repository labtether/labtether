package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const recordingPruneBatchSize = 256

type recordingPruneCursor struct {
	at, cutoff time.Time
	id         string
}

type expiredRecording struct {
	id, path string
	at       time.Time
}

// pruneExpiredRecordings keeps metadata until its file is gone. If the file
// cannot be removed, the row remains eligible for a later cycle. If SQL or the
// process fails after unlink, the retained row is safely retried as a missing
// file. Row locks also keep concurrent retention cycles from owning one file.
func (s *PostgresStore) pruneExpiredRecordings(cutoff time.Time) (int64, error) {
	// Carry a bounded sweep forward even when its oldest files cannot be
	// removed. A stable cutoff makes every sweep finite; the next sweep retries
	// failures. Restarting the store safely restarts that sweep, without losing
	// the durable retry records. Serialize the cursor across manual/timed calls.
	s.recordingPruneMu.Lock()
	defer s.recordingPruneMu.Unlock()
	requestedCutoff := cutoff
	cursor := s.recordingPruneAfter
	if cursor != nil && cursor.cutoff.Before(cutoff) {
		cutoff = cursor.cutoff
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	batch, err := readExpiredRecordingBatch(ctx, tx, cutoff, cursor)
	if err != nil {
		return 0, err
	}
	if len(batch) == 0 && cursor != nil {
		// Wrap an exhausted sweep without wasting a whole retention interval.
		cutoff = requestedCutoff
		batch, err = readExpiredRecordingBatch(ctx, tx, cutoff, nil)
		if err != nil {
			return 0, err
		}
	}
	var deleted int64
	var firstFailure error
	failed := 0
	for _, item := range batch {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if err := removeRecordingFile(item.path); err != nil {
			failed++
			if firstFailure == nil {
				firstFailure = err
			}
			continue
		}
		tag, err := tx.Exec(ctx, `DELETE FROM session_recordings WHERE id = $1`, item.id)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() != 1 {
			return 0, fmt.Errorf("recording retention lost its locked row")
		}
		deleted++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	s.recordingPruneAfter = nil
	if len(batch) == recordingPruneBatchSize {
		last := batch[len(batch)-1]
		s.recordingPruneAfter = &recordingPruneCursor{at: last.at, id: last.id, cutoff: cutoff}
	}
	if failed > 0 {
		return deleted, fmt.Errorf("failed to remove %d expired recording files; metadata retained for retry: %w", failed, firstFailure)
	}
	return deleted, nil
}

func readExpiredRecordingBatch(ctx context.Context, tx pgx.Tx, cutoff time.Time, cursor *recordingPruneCursor) ([]expiredRecording, error) {
	var after *time.Time
	id := ""
	if cursor != nil {
		after, id = &cursor.at, cursor.id
	}
	rows, err := tx.Query(ctx, `SELECT id, file_path, COALESCE(stopped_at, created_at) FROM session_recordings
 WHERE status <> 'recording' AND COALESCE(stopped_at, created_at) < $1
 AND ($2::timestamptz IS NULL OR (COALESCE(stopped_at, created_at), id) > ($2, $3))
 ORDER BY COALESCE(stopped_at, created_at), id
 LIMIT $4 FOR UPDATE SKIP LOCKED`, cutoff, after, id, recordingPruneBatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batch := make([]expiredRecording, 0, recordingPruneBatchSize)
	for rows.Next() {
		var item expiredRecording
		if err := rows.Scan(&item.id, &item.path, &item.at); err != nil {
			return nil, err
		}
		batch = append(batch, item)
	}
	return batch, rows.Err()
}

func removeRecordingFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("recording file path is empty")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("recording file is not a regular file")
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
