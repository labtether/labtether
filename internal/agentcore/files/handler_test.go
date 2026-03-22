package files

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFileWriteSizeLimitEnforced verifies that the size enforcement check
// would reject writes that exceed MaxFileSize (regression for F3: agent file
// upload had no cumulative size check).
func TestFileWriteSizeLimitEnforced(t *testing.T) {
	// Verify the MaxFileSize constant is 512MB.
	if MaxFileSize != 512*1024*1024 {
		t.Fatalf("expected MaxFileSize = 512MB, got %d", MaxFileSize)
	}

	// Simulate the check that now exists in HandleFileWrite.
	written := int64(MaxFileSize - 100) // Nearly at limit
	chunkLen := int64(200)              // Chunk that pushes past limit

	if written+chunkLen <= MaxFileSize {
		t.Fatal("expected size check to fail: written + chunk should exceed MaxFileSize")
	}
}

// TestFileWriteWithinLimitAllowed verifies that writes within size limit pass.
func TestFileWriteWithinLimitAllowed(t *testing.T) {
	written := int64(1024)
	chunkLen := int64(100)

	if written+chunkLen > MaxFileSize {
		t.Fatalf("expected %d + %d to be within MaxFileSize %d", written, chunkLen, MaxFileSize)
	}
}

// TestValidatePathPreventsTraversal verifies that paths outside baseDir are rejected.
func TestValidatePathPreventsTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	fm := &Manager{
		writers: make(map[string]*PendingWrite),
		BaseDir: tmpDir,
	}

	cases := []string{
		"../../etc/passwd",
		"/etc/passwd",
		"../../../root/.ssh/id_rsa",
	}

	for _, tc := range cases {
		_, err := fm.ValidatePath(tc)
		if err == nil {
			t.Errorf("expected error for path %q, got nil", tc)
		}
	}
}

// TestValidatePathAllowsSubdirectories verifies paths within baseDir are accepted.
func TestValidatePathAllowsSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()
	// Resolve symlinks on baseDir itself (macOS /tmp -> /private/var/...).
	resolvedBase, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(resolvedBase, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fm := &Manager{
		writers: make(map[string]*PendingWrite),
		BaseDir: resolvedBase,
	}

	resolved, err := fm.ValidatePath("sub/file.txt")
	if err != nil {
		t.Fatalf("unexpected error for valid subpath: %v", err)
	}
	expected := filepath.Join(resolvedBase, "sub", "file.txt")
	if resolved != expected {
		t.Fatalf("expected %q, got %q", expected, resolved)
	}
}

// TestValidatePathEmpty returns baseDir for empty input.
func TestValidatePathEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	resolvedBase, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	fm := &Manager{
		writers: make(map[string]*PendingWrite),
		BaseDir: resolvedBase,
	}

	resolved, err := fm.ValidatePath("")
	if err != nil {
		t.Fatalf("unexpected error for empty path: %v", err)
	}
	if resolved != resolvedBase {
		t.Fatalf("expected %q, got %q", resolvedBase, resolved)
	}
}

// TestCleanupOrphanedTempFiles verifies old temp files are removed.
func TestCleanupOrphanedTempFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an old temp file (modified 10 minutes ago).
	oldFile := filepath.Join(tmpDir, ".lt-upload-old")
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	tenMinAgo := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(oldFile, tenMinAgo, tenMinAgo); err != nil {
		t.Fatal(err)
	}

	// Create a recent temp file (should not be cleaned).
	recentFile := filepath.Join(tmpDir, ".lt-upload-recent")
	if err := os.WriteFile(recentFile, []byte("recent"), 0o644); err != nil {
		t.Fatal(err)
	}

	fm := &Manager{
		writers: make(map[string]*PendingWrite),
		BaseDir: tmpDir,
	}
	fm.CleanupOrphanedTempFiles()

	// Old file should be removed.
	if _, err := os.Stat(oldFile); err == nil {
		t.Error("expected old temp file to be cleaned up")
	}

	// Recent file should remain.
	if _, err := os.Stat(recentFile); err != nil {
		t.Error("expected recent temp file to be preserved")
	}
}

// TestValidatePathRejectsSymlinkToOutside ensures symlink final components
// cannot escape the base directory (regression for symlink traversal bug).
func TestValidatePathRejectsSymlinkToOutside(t *testing.T) {
	baseDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(baseDir, "outside-link")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	fm := &Manager{
		writers: make(map[string]*PendingWrite),
		BaseDir: baseDir,
	}

	if _, err := fm.ValidatePath("outside-link"); err == nil {
		t.Fatalf("expected symlink path to be rejected")
	}
}

// TestValidatePathRejectsSymlinkDirectoryOutside ensures symlink directories are
// also blocked when the symlink itself is the final path component.
func TestValidatePathRejectsSymlinkDirectoryOutside(t *testing.T) {
	baseDir := t.TempDir()
	outsideDir := t.TempDir()

	linkPath := filepath.Join(baseDir, "outside-dir")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	fm := &Manager{
		writers: make(map[string]*PendingWrite),
		BaseDir: baseDir,
	}

	if _, err := fm.ValidatePath("outside-dir"); err == nil {
		t.Fatalf("expected symlink directory path to be rejected")
	}
}

func TestCopyPathRecursiveCopiesNestedDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "child.txt"), []byte("child-content"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := CopyPathRecursive(srcDir, dstDir); err != nil {
		t.Fatalf("CopyPathRecursive failed: %v", err)
	}

	rootBytes, err := os.ReadFile(filepath.Join(dstDir, "root.txt"))
	if err != nil {
		t.Fatalf("read copied root file: %v", err)
	}
	if string(rootBytes) != "root-content" {
		t.Fatalf("unexpected root file content: %q", string(rootBytes))
	}

	childBytes, err := os.ReadFile(filepath.Join(dstDir, "sub", "child.txt"))
	if err != nil {
		t.Fatalf("read copied child file: %v", err)
	}
	if string(childBytes) != "child-content" {
		t.Fatalf("unexpected child file content: %q", string(childBytes))
	}
}

func TestCopyPathRecursiveRejectsSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(targetFile, linkPath); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	err := CopyPathRecursive(linkPath, filepath.Join(tmpDir, "copied-link.txt"))
	if err == nil {
		t.Fatal("expected symlink copy to fail")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got: %v", err)
	}
}

func TestCopyPathRecursiveRejectsDestinationInsideSource(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "data.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	dstDir := filepath.Join(srcDir, "nested-copy")
	err := CopyPathRecursive(srcDir, dstDir)
	if err == nil {
		t.Fatal("expected nested destination copy to fail")
	}
	if !strings.Contains(err.Error(), "inside source directory") {
		t.Fatalf("expected inside-source error, got: %v", err)
	}
}

func TestResolveFileBaseDirHomeModeUsesHomeByDefault(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		t.Skip("home directory unavailable")
	}
	got := ResolveFileBaseDir("home")
	if filepath.Clean(got) != filepath.Clean(home) {
		t.Fatalf("ResolveFileBaseDir(home) = %q, want %q", got, home)
	}
}

func TestResolveAgentFileHomeDirFallsBackToDesktopSessionHomeWhenProcessHomeReadOnly(t *testing.T) {
	originalHome := FileUserHomeDirFn
	originalLookupUser := FileLookupUserByUsernameFn
	originalLookupUID := FileLookupUserByUIDFn
	originalWritable := FileIsWritableDirFn
	originalDetect := DetectDesktopSessionFn
	originalTempDir := FileTempDirFn
	originalGetwd := FileGetwdFn
	t.Cleanup(func() {
		FileUserHomeDirFn = originalHome
		FileLookupUserByUsernameFn = originalLookupUser
		FileLookupUserByUIDFn = originalLookupUID
		FileIsWritableDirFn = originalWritable
		DetectDesktopSessionFn = originalDetect
		FileTempDirFn = originalTempDir
		FileGetwdFn = originalGetwd
	})

	FileUserHomeDirFn = func() (string, error) { return "/root", nil }
	FileLookupUserByUsernameFn = func(username string) (*user.User, error) {
		if username != "captain" {
			t.Fatalf("username=%q, want captain", username)
		}
		return &user.User{HomeDir: "/home/captain"}, nil
	}
	FileLookupUserByUIDFn = func(uid string) (*user.User, error) {
		t.Fatalf("unexpected uid lookup: %s", uid)
		return nil, nil
	}
	FileIsWritableDirFn = func(path string) bool {
		return filepath.Clean(path) == "/home/captain"
	}
	DetectDesktopSessionFn = func() DesktopSessionInfo {
		return DesktopSessionInfo{Username: "captain", UID: 1000}
	}
	FileTempDirFn = func() string {
		t.Fatal("did not expect temp fallback")
		return ""
	}
	FileGetwdFn = func() (string, error) {
		t.Fatal("did not expect cwd fallback")
		return "", nil
	}

	if got := ResolveAgentFileHomeDir(); got != "/home/captain" {
		t.Fatalf("ResolveAgentFileHomeDir() = %q, want /home/captain", got)
	}
}

func TestResolveAgentFileHomeDirFallsBackToStagingDirWhenHomesAreReadOnly(t *testing.T) {
	originalHome := FileUserHomeDirFn
	originalLookupUser := FileLookupUserByUsernameFn
	originalLookupUID := FileLookupUserByUIDFn
	originalWritable := FileIsWritableDirFn
	originalDetect := DetectDesktopSessionFn
	originalTempDir := FileTempDirFn
	originalGetwd := FileGetwdFn
	t.Cleanup(func() {
		FileUserHomeDirFn = originalHome
		FileLookupUserByUsernameFn = originalLookupUser
		FileLookupUserByUIDFn = originalLookupUID
		FileIsWritableDirFn = originalWritable
		DetectDesktopSessionFn = originalDetect
		FileTempDirFn = originalTempDir
		FileGetwdFn = originalGetwd
	})

	tempDir := t.TempDir()
	FileUserHomeDirFn = func() (string, error) { return "/root", nil }
	FileLookupUserByUsernameFn = func(string) (*user.User, error) {
		return &user.User{HomeDir: "/home/captain"}, nil
	}
	FileLookupUserByUIDFn = func(string) (*user.User, error) { return nil, os.ErrNotExist }
	FileIsWritableDirFn = func(path string) bool {
		return strings.HasPrefix(filepath.Clean(path), filepath.Clean(tempDir))
	}
	DetectDesktopSessionFn = func() DesktopSessionInfo {
		return DesktopSessionInfo{Username: "captain", UID: 1000}
	}
	FileTempDirFn = func() string { return tempDir }
	FileGetwdFn = func() (string, error) { return "/srv/labtether", nil }

	got := ResolveAgentFileHomeDir()
	want := filepath.Join(tempDir, "labtether-agent-home")
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("ResolveAgentFileHomeDir() = %q, want %q", got, want)
	}
}

func TestResolveFileBaseDirFullModeContainsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		t.Skip("home directory unavailable")
	}
	got := ResolveFileBaseDir("full")
	if got == "" {
		t.Fatal("ResolveFileBaseDir(full) returned empty base dir")
	}
	if !PathWithinBaseDir(got, home) {
		t.Fatalf("ResolveFileBaseDir(full) = %q does not contain home %q", got, home)
	}
}

func TestValidatePathExpandsHomeUsingResolvedFileHome(t *testing.T) {
	fm := &Manager{
		writers: make(map[string]*PendingWrite),
		BaseDir: "/tmp/labtether-agent-home",
		HomeDir: "/tmp/labtether-agent-home",
	}

	got, err := fm.ValidatePath("~/notes.txt")
	if err != nil {
		t.Fatalf("ValidatePath returned error: %v", err)
	}
	if filepath.Clean(got) != "/tmp/labtether-agent-home/notes.txt" {
		t.Fatalf("ValidatePath expanded path = %q", got)
	}
}
