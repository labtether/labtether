package resources

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Transfers stage up to 512 MiB. They must land in the disk-backed spool
// directory, never the memory-backed temp directory, and the leftover sweep
// must remove only old staging files that the Hub itself created.
func TestFileSpoolUsesSpoolDirAndSweepsOnlyOldStagingFiles(t *testing.T) {
	temp := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, temp)
	}
	spoolDir := filepath.Join(t.TempDir(), "spool")
	if err := os.MkdirAll(spoolDir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * staleFileSpoolAge)
	write := func(name string, modified time.Time) string {
		path := filepath.Join(spoolDir, name)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
		return path
	}
	abandoned := write(fileSpoolPrefix+"download-1", old)
	recent := write(fileSpoolPrefix+"transfer-2", time.Now())
	unrelated := write("notes.txt", old)

	spool, err := (&Deps{SpoolDir: spoolDir}).createFileSpool("download-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(spool.Name())
	defer spool.Close()

	if filepath.Dir(spool.Name()) != spoolDir {
		t.Fatalf("spool created in %s, want %s", filepath.Dir(spool.Name()), spoolDir)
	}
	if entries, err := os.ReadDir(temp); err != nil || len(entries) != 0 {
		t.Fatalf("temp directory used: entries=%v err=%v", entries, err)
	}
	if _, err := os.Stat(abandoned); !os.IsNotExist(err) {
		t.Fatalf("abandoned staging file kept: %v", err)
	}
	for _, kept := range []string{recent, unrelated} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("sweep removed %s: %v", kept, err)
		}
	}
}
