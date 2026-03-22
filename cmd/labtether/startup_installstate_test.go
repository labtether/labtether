package main

import (
	"path/filepath"
	"testing"

	"github.com/labtether/labtether/internal/installstate"
)

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
	t.Setenv("LABTETHER_OWNER_TOKEN", "legacy-owner-token")
	t.Setenv("LABTETHER_API_TOKEN", "legacy-api-token")
	t.Setenv("LABTETHER_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
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
	if got.EncryptionKey != "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" {
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
	store := installstate.New(filepath.Join(t.TempDir(), "install"))
	if err := store.Save(installstate.Metadata{}, installstate.Secrets{
		OwnerToken:       "persisted-owner-token",
		APIToken:         "persisted-api-token",
		EncryptionKey:    "MDEyMzQ1Njc4OUFCQ0RFRjAxMjM0NTY3ODlBQkNERUY=",
		PostgresPassword: "persisted-postgres-password",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	t.Setenv("LABTETHER_OWNER_TOKEN", "env-owner-token")
	t.Setenv("LABTETHER_API_TOKEN", "env-api-token")
	t.Setenv("LABTETHER_ENCRYPTION_KEY", "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVoxMjM0NTY=")
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
	if got.EncryptionKey != "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVoxMjM0NTY=" {
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
	if persisted.EncryptionKey != "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVoxMjM0NTY=" {
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
