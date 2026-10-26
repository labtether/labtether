package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/labtether/labtether/internal/installstate"
)

func saveInstallStateWithKey(t *testing.T, key string) *installstate.Store {
	t.Helper()
	store := installstate.New(filepath.Join(t.TempDir(), "install"))
	if err := store.Save(installstate.Metadata{SchemaVersion: 1}, installstate.Secrets{
		OwnerToken:    "persisted-owner-token",
		APIToken:      "persisted-api-token",
		EncryptionKey: key,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return store
}

func clearInstallSecretEnv(t *testing.T) {
	t.Setenv("LABTETHER_OWNER_TOKEN", "")
	t.Setenv("LABTETHER_API_TOKEN", "")
	t.Setenv("POSTGRES_PASSWORD", "")
	t.Setenv(encryptionKeyReplaceEnv, "")
}

// Setting LABTETHER_ENCRYPTION_KEY after first boot used to replace the stored
// key silently, leaving every saved credential undecryptable.
func TestEncryptionKeyChangeIsRefusedAndStoredKeyKept(t *testing.T) {
	clearInstallSecretEnv(t)
	storedKey := installStateTestEncryptionKey(0x41)
	store := saveInstallStateWithKey(t, storedKey)
	t.Setenv("LABTETHER_ENCRYPTION_KEY", installStateTestEncryptionKey(0x42))

	_, err := resolveRuntimeInstallSecrets(store)
	if !errors.Is(err, errEncryptionKeyMismatch) {
		t.Fatalf("resolveRuntimeInstallSecrets() error = %v, want key mismatch", err)
	}
	_, persisted, _, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if persisted.EncryptionKey != storedKey {
		t.Fatal("refused start still overwrote the stored encryption key")
	}
}

func TestEncryptionKeyMatchingStoredKeyStarts(t *testing.T) {
	clearInstallSecretEnv(t)
	storedKey := installStateTestEncryptionKey(0x41)
	store := saveInstallStateWithKey(t, storedKey)
	t.Setenv("LABTETHER_ENCRYPTION_KEY", storedKey)

	got, err := resolveRuntimeInstallSecrets(store)
	if err != nil {
		t.Fatalf("resolveRuntimeInstallSecrets() error = %v", err)
	}
	if got.EncryptionKey != storedKey {
		t.Fatalf("EncryptionKey changed unexpectedly")
	}
}

func TestEncryptionKeyReplacementRequiresExplicitConfirmation(t *testing.T) {
	clearInstallSecretEnv(t)
	store := saveInstallStateWithKey(t, installStateTestEncryptionKey(0x41))
	newKey := installStateTestEncryptionKey(0x43)
	t.Setenv("LABTETHER_ENCRYPTION_KEY", newKey)
	t.Setenv(encryptionKeyReplaceEnv, "true")

	got, err := resolveRuntimeInstallSecrets(store)
	if err != nil {
		t.Fatalf("resolveRuntimeInstallSecrets() error = %v", err)
	}
	if got.EncryptionKey != newKey {
		t.Fatal("confirmed replacement did not take the new key")
	}
	_, persisted, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.EncryptionKey != newKey {
		t.Fatal("confirmed replacement was not persisted")
	}
}
