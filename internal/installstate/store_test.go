package installstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreSaveAndLoadRoundTrip(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "install")
	store := New(root)

	createdAt := time.Date(2026, time.March, 11, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2 * time.Minute)
	meta := Metadata{
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	secrets := Secrets{
		OwnerToken:       "owner-token",
		APIToken:         "api-token",
		EncryptionKey:    "MDEyMzQ1Njc4OUFCQ0RFRjAxMjM0NTY3ODlBQkNERUY=",
		PostgresPassword: "postgres-password",
	}

	if err := store.Save(meta, secrets); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	gotMeta, gotSecrets, exists, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !exists {
		t.Fatalf("Load() exists = false, want true")
	}
	if gotMeta.SchemaVersion != currentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", gotMeta.SchemaVersion, currentSchemaVersion)
	}
	if !gotMeta.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", gotMeta.CreatedAt, createdAt)
	}
	if !gotMeta.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("UpdatedAt = %v, want %v", gotMeta.UpdatedAt, updatedAt)
	}
	if gotSecrets != secrets {
		t.Fatalf("Secrets = %+v, want %+v", gotSecrets, secrets)
	}
}

func TestStoreLoadMissingReturnsFalse(t *testing.T) {
	t.Parallel()

	store := New(filepath.Join(t.TempDir(), "missing"))
	_, _, exists, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if exists {
		t.Fatalf("Load() exists = true, want false")
	}
}

func TestStoreLoadInvalidJSONReturnsError(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "install")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := New(root)
	_, _, _, err := store.Load()
	if err == nil {
		t.Fatalf("Load() error = nil, want error")
	}
	if !errors.Is(err, os.ErrNotExist) && err.Error() == "" {
		t.Fatalf("Load() error should describe decode failure")
	}
}
