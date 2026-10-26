package persistence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/retention"
)

// Retention can delete across the table, so use the existing guard for the
// exact stamped, disposable database before migrations or any fixture writes.
func recordingRetentionStore(t *testing.T) *PostgresStore {
	t.Helper()
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM session_recordings`).Scan(&count); err != nil || count != 0 {
		t.Fatal("recording retention fixture requires an empty owned table")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := s.pool.Exec(ctx, `DELETE FROM session_recordings WHERE session_id = 'retention-test-session'`); err != nil {
			t.Errorf("remove recording fixture rows: %v", err)
		}
	})
	return s
}

func insertRetentionRecording(t *testing.T, s *PostgresStore, id, path, status string, created time.Time) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `INSERT INTO session_recordings
 (id, session_id, asset_id, actor_id, protocol, file_path, status, created_at)
 VALUES ($1, 'retention-test-session', 'retention-test-asset', 'owner', 'vnc', $2, $3, $4)`, id, path, status, created)
	if err != nil {
		t.Fatal(err)
	}
}

func assertRetentionRecordingExists(t *testing.T, s *PostgresStore, id string, expected bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM session_recordings WHERE id = $1)`, id).Scan(&exists)
	if err != nil || exists != expected {
		t.Fatalf("recording %s exists=%v want=%v error=%v", id, exists, expected, err)
	}
}

func TestPostgresRecordingRetentionRetriesFailedFiles(t *testing.T) {
	s := recordingRetentionStore(t)
	dir := t.TempDir()
	now := time.Now().UTC()
	settings := retention.DefaultSettings()
	settings.RecordingsWindow = time.Hour
	paths := make(map[string]string)
	for _, id := range []string{"failed", "removed", "missing", "active", "recent"} {
		path := filepath.Join(dir, id)
		paths[id] = path
		if id == "failed" {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "preserved"), []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
		} else if id != "missing" {
			if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		status, created := "completed", now.Add(-2*time.Hour)
		if id == "active" {
			status = "recording"
		}
		if id == "recent" {
			created = now
		}
		insertRetentionRecording(t, s, id, path, status, created)
	}
	result, err := s.PruneExpiredData(now, settings)
	if err == nil || !strings.Contains(err.Error(), "metadata retained for retry") || result.RecordingsDeleted != 2 {
		t.Fatalf("first cleanup deleted=%d error=%v", result.RecordingsDeleted, err)
	}
	for _, id := range []string{"failed", "removed", "missing", "active", "recent"} {
		assertRetentionRecordingExists(t, s, id, id != "removed" && id != "missing")
	}
	if _, err := os.Stat(paths["removed"]); !os.IsNotExist(err) {
		t.Fatalf("expired recording file remains: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(paths["failed"], "preserved")); err != nil || string(data) != "keep" {
		t.Fatal("failed path contents were changed")
	}
	if err := os.Remove(filepath.Join(paths["failed"], "preserved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths["failed"]); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths["failed"], []byte("retry"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = s.PruneExpiredData(now, settings)
	if err != nil || result.RecordingsDeleted != 1 {
		t.Fatalf("retry deleted=%d error=%v", result.RecordingsDeleted, err)
	}
	assertRetentionRecordingExists(t, s, "failed", false)
	if _, err := os.Stat(paths["failed"]); !os.IsNotExist(err) {
		t.Fatalf("retry file remains: %v", err)
	}
	for _, id := range []string{"active", "recent"} {
		assertRetentionRecordingExists(t, s, id, true)
		if data, err := os.ReadFile(paths[id]); err != nil || string(data) != id {
			t.Fatalf("protected %s recording changed", id)
		}
	}
}

func TestPostgresRecordingRetentionRetriesAfterSQLRollback(t *testing.T) {
	s := recordingRetentionStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "recording.bin")
	if err := os.WriteFile(path, []byte("expired"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	insertRetentionRecording(t, s, "rollback", path, "completed", now.Add(-2*time.Hour))
	_, err := s.pool.Exec(ctx, `CREATE FUNCTION retention_fixture_reject_delete() RETURNS trigger
 LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'retention fixture delete failure'; END $$;
 CREATE TRIGGER retention_fixture_delete BEFORE DELETE ON session_recordings
 FOR EACH ROW EXECUTE FUNCTION retention_fixture_reject_delete()`)
	if err != nil {
		t.Fatal(err)
	}
	removeTrigger := func() error {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, err := s.pool.Exec(cleanup, `DROP TRIGGER IF EXISTS retention_fixture_delete ON session_recordings;
 DROP FUNCTION IF EXISTS retention_fixture_reject_delete()`)
		return err
	}
	t.Cleanup(func() {
		if err := removeTrigger(); err != nil {
			t.Errorf("remove fixture trigger: %v", err)
		}
	})
	settings := retention.DefaultSettings()
	settings.RecordingsWindow = time.Hour
	if _, err := s.PruneExpiredData(now, settings); err == nil || !strings.Contains(err.Error(), "retention fixture delete failure") {
		t.Fatalf("expected SQL failure after unlink: %v", err)
	}
	assertRetentionRecordingExists(t, s, "rollback", true)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected unlink before failed SQL: %v", err)
	}
	if err := removeTrigger(); err != nil {
		t.Fatal(err)
	}
	result, err := s.PruneExpiredData(now, settings)
	if err != nil || result.RecordingsDeleted != 1 {
		t.Fatalf("missing-file retry deleted=%d error=%v", result.RecordingsDeleted, err)
	}
	assertRetentionRecordingExists(t, s, "rollback", false)
}

func TestPostgresRecordingRetentionAdvancesPastFailedBatch(t *testing.T) {
	s := recordingRetentionStore(t)
	dir := t.TempDir()
	now := time.Now().UTC()
	settings := retention.DefaultSettings()
	settings.RecordingsWindow = time.Hour
	for index := 0; index < recordingPruneBatchSize; index++ {
		id := fmt.Sprintf("failed-%03d", index)
		path := filepath.Join(dir, id)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		insertRetentionRecording(t, s, id, path, "completed", now.Add(-3*time.Hour))
	}
	tail := filepath.Join(dir, "healthy-tail")
	if err := os.WriteFile(tail, []byte("expired"), 0o600); err != nil {
		t.Fatal(err)
	}
	insertRetentionRecording(t, s, "healthy-tail", tail, "completed", now.Add(-2*time.Hour))
	result, err := s.PruneExpiredData(now, settings)
	if err == nil || result.RecordingsDeleted != 0 {
		t.Fatalf("failed head batch deleted=%d error=%v", result.RecordingsDeleted, err)
	}
	assertRetentionRecordingExists(t, s, "healthy-tail", true)
	result, err = s.PruneExpiredData(now, settings)
	if err != nil || result.RecordingsDeleted != 1 {
		t.Fatalf("healthy tail was starved: deleted=%d error=%v", result.RecordingsDeleted, err)
	}
	assertRetentionRecordingExists(t, s, "healthy-tail", false)
	if _, err := os.Stat(tail); !os.IsNotExist(err) {
		t.Fatalf("healthy tail file remains: %v", err)
	}
	// A later sweep must retry its old failures, not abandon them forever.
	head := filepath.Join(dir, "failed-000")
	if err := os.Remove(head); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(head, []byte("retry"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = s.PruneExpiredData(now, settings)
	if err == nil || result.RecordingsDeleted != 1 {
		t.Fatalf("head retry deleted=%d error=%v", result.RecordingsDeleted, err)
	}
	assertRetentionRecordingExists(t, s, "failed-000", false)
	assertRetentionRecordingExists(t, s, "failed-001", true)
}
