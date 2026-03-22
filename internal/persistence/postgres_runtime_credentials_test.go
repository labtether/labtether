package persistence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveRecordingFiles_RemovesExistingAndMissingAsSuccess(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "recording.bin")
	if err := os.WriteFile(existing, []byte("data"), 0o600); err != nil {
		t.Fatalf("write temp recording: %v", err)
	}
	missing := filepath.Join(dir, "missing.bin")

	removed, failed := removeRecordingFiles([]string{existing, missing, ""})
	if failed != 0 {
		t.Fatalf("expected no failures, got %d", failed)
	}
	if removed != 2 {
		t.Fatalf("expected 2 removals (existing + missing), got %d", removed)
	}
	if _, err := os.Stat(existing); !os.IsNotExist(err) {
		t.Fatalf("expected existing file to be removed, stat err=%v", err)
	}
}

func TestAlertSilencePruneColumnPrefersExpiresAtWhenAvailable(t *testing.T) {
	column := alertSilencePruneColumn([]string{"ends_at", "expires_at"})
	if column != "expires_at" {
		t.Fatalf("expected expires_at to be preferred, got %q", column)
	}
}

func TestAlertSilencePruneColumnFallsBackToEndsAt(t *testing.T) {
	column := alertSilencePruneColumn([]string{"ends_at"})
	if column != "ends_at" {
		t.Fatalf("expected ends_at fallback, got %q", column)
	}
}

func TestAlertSilencePruneColumnHandlesMissingColumns(t *testing.T) {
	column := alertSilencePruneColumn([]string{"created_at", "starts_at"})
	if column != "" {
		t.Fatalf("expected empty column selection, got %q", column)
	}
}
