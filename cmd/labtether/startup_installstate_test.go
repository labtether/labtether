package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/labtether/labtether/internal/installstate"
)

func installStateTestEncryptionKey(seed byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 32))
}

func TestResolveRuntimeInstallSecretsGeneratesAndPersistsMissingValues(t *testing.T) {
	store := installstate.New(filepath.Join(t.TempDir(), "install"))

	got, err := resolveRuntimeInstallSecrets(store)
	if err != nil {
		t.Fatalf("resolveRuntimeInstallSecrets() error = %v", err)
	}
	if got.OwnerToken == "" {
		t.Fatalf("OwnerToken is empty")
	}
	if got.APIToken == "" {
		t.Fatalf("APIToken is empty")
	}
	if got.EncryptionKey == "" {
		t.Fatalf("EncryptionKey is empty")
	}

	got2, err := resolveRuntimeInstallSecrets(store)
	if err != nil {
		t.Fatalf("resolveRuntimeInstallSecrets() second call error = %v", err)
	}
	if got2 != got {
		t.Fatalf("resolved secrets changed across restart: first=%+v second=%+v", got, got2)
	}
}

func TestResolveRuntimeInstallSecretsMigratesLegacyEnvSecrets(t *testing.T) {
	legacyEncryptionKey := installStateTestEncryptionKey(0x31)
	t.Setenv("LABTETHER_OWNER_TOKEN", "legacy-owner-token")
	t.Setenv("LABTETHER_API_TOKEN", "legacy-api-token")
	t.Setenv("LABTETHER_ENCRYPTION_KEY", legacyEncryptionKey)
	t.Setenv("POSTGRES_PASSWORD", "generated-postgres-password")

	store := installstate.New(filepath.Join(t.TempDir(), "install"))

	got, err := resolveRuntimeInstallSecrets(store)
	if err != nil {
		t.Fatalf("resolveRuntimeInstallSecrets() error = %v", err)
	}
	if got.OwnerToken != "legacy-owner-token" {
		t.Fatalf("OwnerToken = %q, want legacy-owner-token", got.OwnerToken)
	}
	if got.APIToken != "legacy-api-token" {
		t.Fatalf("APIToken = %q, want legacy-api-token", got.APIToken)
	}
	if got.EncryptionKey != legacyEncryptionKey {
		t.Fatalf("EncryptionKey = %q, want migrated env value", got.EncryptionKey)
	}

	t.Setenv("LABTETHER_OWNER_TOKEN", "")
	t.Setenv("LABTETHER_API_TOKEN", "")
	t.Setenv("LABTETHER_ENCRYPTION_KEY", "")

	got2, err := resolveRuntimeInstallSecrets(store)
	if err != nil {
		t.Fatalf("resolveRuntimeInstallSecrets() second call error = %v", err)
	}
	if got2 != got {
		t.Fatalf("resolved secrets changed after env removal: first=%+v second=%+v", got, got2)
	}

	_, persisted, exists, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !exists {
		t.Fatalf("Load() exists = false, want true")
	}
	if persisted.PostgresPassword != "generated-postgres-password" {
		t.Fatalf("PostgresPassword = %q, want generated-postgres-password", persisted.PostgresPassword)
	}
}

func TestResolveRuntimeInstallSecretsAllowsEnvOverridesAndPersistsThem(t *testing.T) {
	persistedEncryptionKey := installStateTestEncryptionKey(0x41)
	overrideEncryptionKey := installStateTestEncryptionKey(0x42)
	store := installstate.New(filepath.Join(t.TempDir(), "install"))
	if err := store.Save(installstate.Metadata{}, installstate.Secrets{
		OwnerToken:       "persisted-owner-token",
		APIToken:         "persisted-api-token",
		EncryptionKey:    persistedEncryptionKey,
		PostgresPassword: "persisted-postgres-password",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	t.Setenv("LABTETHER_OWNER_TOKEN", "env-owner-token")
	t.Setenv("LABTETHER_API_TOKEN", "env-api-token")
	t.Setenv("LABTETHER_ENCRYPTION_KEY", overrideEncryptionKey)
	t.Setenv("POSTGRES_PASSWORD", "env-postgres-password")

	got, err := resolveRuntimeInstallSecrets(store)
	if err != nil {
		t.Fatalf("resolveRuntimeInstallSecrets() error = %v", err)
	}
	if got.OwnerToken != "env-owner-token" {
		t.Fatalf("OwnerToken = %q, want env-owner-token", got.OwnerToken)
	}
	if got.APIToken != "env-api-token" {
		t.Fatalf("APIToken = %q, want env-api-token", got.APIToken)
	}
	if got.EncryptionKey != overrideEncryptionKey {
		t.Fatalf("EncryptionKey = %q, want env override", got.EncryptionKey)
	}

	_, persisted, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if persisted.OwnerToken != "env-owner-token" {
		t.Fatalf("persisted OwnerToken = %q, want env-owner-token", persisted.OwnerToken)
	}
	if persisted.APIToken != "env-api-token" {
		t.Fatalf("persisted APIToken = %q, want env-api-token", persisted.APIToken)
	}
	if persisted.EncryptionKey != overrideEncryptionKey {
		t.Fatalf("persisted EncryptionKey = %q, want env override", persisted.EncryptionKey)
	}
	if persisted.PostgresPassword != "env-postgres-password" {
		t.Fatalf("persisted PostgresPassword = %q, want env-postgres-password", persisted.PostgresPassword)
	}
}

func TestResolveRuntimeInstallSecretsRejectsInvalidEncryptionKey(t *testing.T) {
	t.Setenv("LABTETHER_ENCRYPTION_KEY", "not-base64")
	store := installstate.New(filepath.Join(t.TempDir(), "install"))

	_, err := resolveRuntimeInstallSecrets(store)
	if err == nil {
		t.Fatalf("resolveRuntimeInstallSecrets() error = nil, want error")
	}
}

func TestWriteRuntimeAPITokenFileIsPrivateAndReplacesSymlink(t *testing.T) {
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	if err := os.WriteFile(victim, []byte("unchanged"), 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	tokenPath := filepath.Join(root, "api-token")
	if err := os.Symlink(victim, tokenPath); err != nil {
		t.Fatalf("symlink token path: %v", err)
	}
	t.Setenv("LABTETHER_API_TOKEN_FILE", tokenPath)

	if err := writeRuntimeAPITokenFile("  replacement-token\n"); err != nil {
		t.Fatalf("writeRuntimeAPITokenFile() error = %v", err)
	}

	victimData, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}
	if string(victimData) != "unchanged" {
		t.Fatalf("victim = %q, want unchanged", victimData)
	}
	info, err := os.Lstat(tokenPath)
	if err != nil {
		t.Fatalf("lstat token: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %v, want private regular file", info.Mode())
	}
	tokenData, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	if string(tokenData) != "replacement-token" {
		t.Fatalf("token = %q, want replacement-token", tokenData)
	}
}

func TestWriteRuntimeAPITokenFileRejectsSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatalf("mkdir real directory: %v", err)
	}
	linkDir := filepath.Join(root, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink directory: %v", err)
	}
	t.Setenv("LABTETHER_API_TOKEN_FILE", filepath.Join(linkDir, "api-token"))

	if err := writeRuntimeAPITokenFile("replacement-token"); err == nil {
		t.Fatal("writeRuntimeAPITokenFile() error = nil, want symlink-directory rejection")
	}
	if _, err := os.Stat(filepath.Join(realDir, "api-token")); !os.IsNotExist(err) {
		t.Fatalf("real directory token exists or stat failed unexpectedly: %v", err)
	}
}
