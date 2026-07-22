package shared

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInsecureSSHHostKeysRequireExplicitAcknowledgement(t *testing.T) {
	t.Setenv(EnvAllowInsecureSSHHostKeys, "")
	if InsecureSSHHostKeysAllowed() {
		t.Fatal("insecure SSH host keys must be disabled by default")
	}

	t.Setenv(EnvAllowInsecureSSHHostKeys, "true")
	if !InsecureSSHHostKeysAllowed() {
		t.Fatal("expected explicit insecure SSH host-key acknowledgement to be honored")
	}
}

func TestDiscoverKnownHostsFilesTreatsExplicitPathsAsAuthoritative(t *testing.T) {
	home := t.TempDir()
	defaultPath := filepath.Join(home, ".ssh", "known_hosts")
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0o700); err != nil {
		t.Fatalf("create default known_hosts directory: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte("default-key\n"), 0o600); err != nil {
		t.Fatalf("create default known_hosts file: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv(EnvSSHKnownHostsPath, "")
	t.Setenv(EnvSSHKnownHostsPaths, "")

	if paths := DiscoverKnownHostsFiles(); len(paths) != 0 {
		t.Fatalf("explicitly empty known_hosts paths must suppress runtime defaults, got %v", paths)
	}
}

func TestDiscoverKnownHostsFilesUsesConfiguredPathOnly(t *testing.T) {
	home := t.TempDir()
	defaultPath := filepath.Join(home, ".ssh", "known_hosts")
	configuredPath := filepath.Join(t.TempDir(), "known_hosts")
	for _, path := range []string{defaultPath, configuredPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create known_hosts directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("key\n"), 0o600); err != nil {
			t.Fatalf("create known_hosts file: %v", err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv(EnvSSHKnownHostsPath, configuredPath)
	t.Setenv(EnvSSHKnownHostsPaths, "")

	paths := DiscoverKnownHostsFiles()
	if len(paths) != 1 || paths[0] != configuredPath {
		t.Fatalf("configured known_hosts path must be authoritative, got %v", paths)
	}
}
