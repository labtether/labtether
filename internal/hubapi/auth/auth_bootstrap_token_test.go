package auth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateBootstrapSetupTokenFromFile(t *testing.T) {
	t.Setenv("LABTETHER_SETUP_TOKEN", "ignored-env-token")
	path := filepath.Join(t.TempDir(), "setup-token")
	if err := os.WriteFile(path, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("write setup token: %v", err)
	}
	t.Setenv("LABTETHER_SETUP_TOKEN_FILE", path)

	if err := ValidateBootstrapSetupToken("file-token"); err != nil {
		t.Fatalf("expected file token to validate: %v", err)
	}
	if err := ValidateBootstrapSetupToken("ignored-env-token"); !errors.Is(err, ErrBootstrapSetupTokenInvalid) {
		t.Fatalf("expected file token to take precedence, got %v", err)
	}
}

func TestValidateBootstrapSetupTokenFailsClosed(t *testing.T) {
	t.Setenv("LABTETHER_SETUP_TOKEN", "")
	t.Setenv("LABTETHER_SETUP_TOKEN_FILE", filepath.Join(t.TempDir(), "missing-token"))
	if err := ValidateBootstrapSetupToken("anything"); !errors.Is(err, ErrBootstrapSetupTokenNotConfigured) {
		t.Fatalf("expected missing configuration error, got %v", err)
	}
}

func TestReadBootstrapSetupTokenFileRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}

	rootDir := t.TempDir()
	outsidePath := filepath.Join(t.TempDir(), "outside-token")
	if err := os.WriteFile(outsidePath, []byte("outside-secret"), 0o600); err != nil {
		t.Fatalf("write outside token: %v", err)
	}
	linkPath := filepath.Join(rootDir, "setup-token")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Fatalf("create token symlink: %v", err)
	}

	if _, err := readBootstrapSetupTokenFile(linkPath); !errors.Is(err, ErrBootstrapSetupTokenNotConfigured) {
		t.Fatalf("expected an escaping symlink to be rejected, got %v", err)
	}
}

func TestValidateBootstrapSetupTokenGeneratesAndConsumesDefaultFile(t *testing.T) {
	t.Setenv("LABTETHER_SETUP_TOKEN", "")
	t.Setenv("LABTETHER_SETUP_TOKEN_FILE", "")
	dataDir := t.TempDir()
	t.Setenv("LABTETHER_DATA_DIR", dataDir)

	token, err := configuredBootstrapSetupToken()
	if err != nil {
		t.Fatalf("generate default setup token: %v", err)
	}
	if err := ValidateBootstrapSetupToken(token); err != nil {
		t.Fatalf("validate generated token: %v", err)
	}
	path := filepath.Join(dataDir, "install", "setup-token")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat generated token: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("setup token mode = %o, want 600", got)
	}

	ConsumeBootstrapSetupToken()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected consumed token file to be removed, got %v", err)
	}
}
