package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/secrets"
)

type memorySystemSettings map[string]json.RawMessage

func (m memorySystemSettings) GetSystemSetting(_ context.Context, key string) (json.RawMessage, bool, error) {
	value, ok := m[key]
	return value, ok, nil
}

func (m memorySystemSettings) PutSystemSetting(_ context.Context, key string, value json.RawMessage) error {
	m[key] = value
	return nil
}

// A database restored onto a Hub with a different encryption key must stop
// the start, not fail later on every saved secret.
func TestCheckEncryptionKeyMatchesDataRefusesRestoredDatabaseWithOtherKey(t *testing.T) {
	keyA := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	keyB := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("b", 32)))
	managerFor := func(key string) *secrets.Manager {
		manager, err := secrets.NewManagerFromEncodedKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return manager
	}
	ctx := context.Background()
	store := memorySystemSettings{}

	if err := checkEncryptionKeyMatchesData(ctx, store, managerFor(keyA), keyA); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if err := checkEncryptionKeyMatchesData(ctx, store, managerFor(keyA), keyA); err != nil {
		t.Fatalf("same key: %v", err)
	}

	t.Setenv(encryptionKeyReplaceEnv, "")
	err := checkEncryptionKeyMatchesData(ctx, store, managerFor(keyB), keyB)
	if !errors.Is(err, errEncryptionKeyWrongForData) {
		t.Fatalf("other key: error = %v, want errEncryptionKeyWrongForData", err)
	}
	if !strings.Contains(err.Error(), encryptionKeyID(keyA)) {
		t.Fatalf("error %q does not name the database's key ID", err)
	}

	t.Setenv(encryptionKeyReplaceEnv, "true")
	if err := checkEncryptionKeyMatchesData(ctx, store, managerFor(keyB), keyB); err != nil {
		t.Fatalf("replace: %v", err)
	}
	t.Setenv(encryptionKeyReplaceEnv, "")
	if err := checkEncryptionKeyMatchesData(ctx, store, managerFor(keyB), keyB); err != nil {
		t.Fatalf("after replace: %v", err)
	}
}
