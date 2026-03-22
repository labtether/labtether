package remoteaccess

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverDisplayXAuthorityFromProc(t *testing.T) {
	tempDir := t.TempDir()
	procDir := filepath.Join(tempDir, "proc")
	if err := os.MkdirAll(filepath.Join(procDir, "901"), 0o755); err != nil {
		t.Fatalf("mkdir proc: %v", err)
	}

	authPath := filepath.Join(tempDir, "run", "lightdm", "root", ":0")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.WriteFile(authPath, []byte("cookie"), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	cmdline := "/usr/lib/xorg/Xorg\x00:0\x00-seat\x00seat0\x00-auth\x00" + authPath + "\x00-nolisten\x00tcp\x00"
	if err := os.WriteFile(filepath.Join(procDir, "901", "cmdline"), []byte(cmdline), 0o644); err != nil {
		t.Fatalf("write cmdline: %v", err)
	}

	originalProcRoot := X11ProcRoot
	t.Cleanup(func() {
		X11ProcRoot = originalProcRoot
	})
	X11ProcRoot = procDir

	if got := DiscoverDisplayXAuthority(":0"); got != authPath {
		t.Fatalf("DiscoverDisplayXAuthority(:0)=%q, want %q", got, authPath)
	}
}
