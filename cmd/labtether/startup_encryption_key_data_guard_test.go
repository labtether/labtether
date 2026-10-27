package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/labtether/labtether/internal/installstate"
	"github.com/labtether/labtether/internal/persistence"
)

type fakeEncryptedSecretsState persistence.EncryptedSecretsState

func (f fakeEncryptedSecretsState) LoadEncryptedSecretsState(context.Context) (persistence.EncryptedSecretsState, error) {
	return persistence.EncryptedSecretsState(f), nil
}

func clearEncryptionKeyEnv(t *testing.T) {
	t.Helper()
	clearInstallSecretEnv(t)
	t.Setenv("LABTETHER_ENCRYPTION_KEY", "")
	t.Setenv("LABTETHER_TOTP_KEY", "")
}

// A database kept or restored without its data volume used to start with a
// newly generated key, silently leaving every saved secret unreadable.
func TestMissingEncryptionKeyWithEncryptedDataRefusesStart(t *testing.T) {
	clearEncryptionKeyEnv(t)
	store := installstate.New(filepath.Join(t.TempDir(), "install"))

	err := checkEncryptionKeyPresentForData(context.Background(), store, fakeEncryptedSecretsState{Credentials: true})
	if !errors.Is(err, errEncryptionKeyMissing) {
		t.Fatalf("checkEncryptionKeyPresentForData() error = %v, want missing key", err)
	}
	if _, _, exists, loadErr := store.Load(); loadErr != nil || exists {
		t.Fatalf("refused start wrote install state (exists=%v, err=%v)", exists, loadErr)
	}
}

// Fresh installs, normal restarts and a confirmed replacement must still start.
func TestEncryptionKeyDataCheckAllowsNormalStarts(t *testing.T) {
	cases := []struct {
		name      string
		storedKey string
		replace   string
		state     fakeEncryptedSecretsState
	}{
		{name: "fresh install", state: fakeEncryptedSecretsState{}},
		{name: "stored key", storedKey: installStateTestEncryptionKey(0x41), state: fakeEncryptedSecretsState{Credentials: true, TOTP: true}},
		{name: "replace confirmed", replace: "true", state: fakeEncryptedSecretsState{Credentials: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearEncryptionKeyEnv(t)
			t.Setenv(encryptionKeyReplaceEnv, tc.replace)
			store := installstate.New(filepath.Join(t.TempDir(), "install"))
			if tc.storedKey != "" {
				store = saveInstallStateWithKey(t, tc.storedKey)
			}
			if err := checkEncryptionKeyPresentForData(context.Background(), store, tc.state); err != nil {
				t.Fatalf("checkEncryptionKeyPresentForData() error = %v", err)
			}
		})
	}
}
