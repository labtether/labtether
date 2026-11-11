package desktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type recordingExecs struct{ sql []string }

func (r *recordingExecs) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	r.sql = append(r.sql, strings.Join(strings.Fields(sql), " "))
	return pgconn.CommandTag{}, nil
}

func startedRecording(t *testing.T) *ActiveRecording {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rec_test.bin")
	file, err := openPrivateRecordingFile(path)
	if err != nil {
		t.Fatalf("open recording: %v", err)
	}
	return &ActiveRecording{ID: "rec_test", FilePath: path, File: file, StartedAt: time.Now().Add(-10 * time.Second)}
}

// A Remote View session that never started was listed as a "completed"
// recording of 0 bytes and 10 s. A recording that never got a frame is
// removed with its empty file; one with frames is completed as before.
func TestFinishRecordingRemovesARecordingThatNeverGotAFrame(t *testing.T) {
	empty := startedRecording(t)
	store := &recordingExecs{}
	finishRecording(store, empty)
	if len(store.sql) != 1 || !strings.HasPrefix(store.sql[0], "DELETE FROM session_recordings") {
		t.Fatalf("empty recording ran %q, want one DELETE", store.sql)
	}
	if _, err := os.Stat(empty.FilePath); !os.IsNotExist(err) {
		t.Fatalf("empty recording file still exists (stat err %v)", err)
	}

	full := startedRecording(t)
	full.WriteFrame([]byte("RFB 003.008\n"))
	store = &recordingExecs{}
	finishRecording(store, full)
	if len(store.sql) != 1 || !strings.Contains(store.sql[0], "SET status = 'completed'") {
		t.Fatalf("recording with frames ran %q, want it completed", store.sql)
	}
	if info, err := os.Stat(full.FilePath); err != nil || info.Size() == 0 {
		t.Fatalf("recording with frames lost its file: %v", err)
	}
}
