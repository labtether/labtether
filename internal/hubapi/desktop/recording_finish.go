package desktop

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// recordedBytes is how many bytes of the session reached the file.
func (r *ActiveRecording) recordedBytes() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Bytes
}

// recordingStore is the part of the database pool that finalizes recordings.
type recordingStore interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// DefaultStopRecording finalizes a recording. Used as a default for Deps.StopRecording.
// A recording that never received a frame (the session never started, for
// example because the device's VNC server failed) is removed with its empty
// file instead of being listed as a completed 0-byte recording.
func DefaultStopRecording(dbPool *pgxpool.Pool, rec *ActiveRecording) {
	if dbPool == nil {
		if rec != nil {
			rec.Stop()
		}
		return
	}
	finishRecording(dbPool, rec)
}

func finishRecording(store recordingStore, rec *ActiveRecording) {
	if rec == nil {
		return
	}
	rec.Stop()
	if rec.recordedBytes() == 0 {
		if _, err := store.Exec(context.Background(), `DELETE FROM session_recordings WHERE id = $1`, rec.ID); err != nil {
			log.Printf("recording: failed to remove empty recording %s: %v", rec.ID, err)
			return
		}
		if err := os.Remove(rec.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("recording: failed to remove empty recording file %s: %v", rec.ID, err)
		}
		return
	}
	now := time.Now().UTC()
	durationMS := now.Sub(rec.StartedAt).Milliseconds()
	_, err := store.Exec(
		context.Background(),
		`UPDATE session_recordings
		 SET status = 'completed',
		     file_size = $1,
		     duration_ms = $2,
		     stopped_at = $3
		 WHERE id = $4`,
		rec.recordedBytes(),
		durationMS,
		now,
		rec.ID,
	)
	if err != nil {
		log.Printf("recording: failed to finalize %s: %v", rec.ID, err)
	}
}
