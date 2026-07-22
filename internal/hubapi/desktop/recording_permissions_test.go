package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsurePrivateRecordingDirTightensExistingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are not enforced on Windows")
	}

	dir := filepath.Join(t.TempDir(), "recordings")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateRecordingDir(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("recordings directory mode = %o, want 700", got)
	}
}

func TestEnsurePrivateRecordingDirRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on Windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "recordings")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateRecordingDir(link); err == nil {
		t.Fatal("expected recordings-directory symlink to be rejected")
	}
}

func TestOpenPrivateRecordingFileUsesOwnerOnlyModeAndExclusiveCreate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are not enforced on Windows")
	}

	path := filepath.Join(t.TempDir(), "recording.bin")
	file, err := openPrivateRecordingFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("recording file mode = %o, want 600", got)
	}
	if _, err := openPrivateRecordingFile(path); err == nil {
		t.Fatal("expected exclusive creation to reject an existing recording")
	}
}
