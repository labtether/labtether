package persistence

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/credentials"
	"golang.org/x/crypto/ssh"
)

func TestPostgresPinSFTPHostKeyIsAtomic(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping SFTP host key persistence integration test")
	}

	store := mustOpenPostgresStoreForMigrationRecovery(t, dbURL)
	defer store.Close()
	profile, err := store.CreateCredentialProfile(credentials.Profile{
		ID:               "cred-sftp-host-key-race",
		Name:             "SFTP host key race",
		Kind:             credentials.KindSSHPassword,
		Username:         "operator",
		Status:           "active",
		SecretCiphertext: "original-ciphertext",
	})
	if err != nil {
		t.Fatalf("create credential profile: %v", err)
	}
	defer store.DeleteCredentialProfile(profile.ID)
	zeroPort := 0
	connection := &FileConnection{
		Name:         "SFTP host key race",
		Protocol:     "sftp",
		Host:         "sftp-race.example.invalid",
		Port:         &zeroPort,
		InitialPath:  "/",
		ExtraConfig:  map[string]any{"future_option": "preserved"},
		CredentialID: &profile.ID,
	}
	if err := store.CreateFileConnection(context.Background(), connection); err != nil {
		t.Fatalf("create file connection: %v", err)
	}
	defer store.DeleteFileConnection(context.Background(), connection.ID)
	staleUpdate, err := store.GetFileConnection(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("load stale update snapshot: %v", err)
	}
	staleExplicitKey, err := store.GetFileConnection(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("load stale explicit-key snapshot: %v", err)
	}
	staleAtomicUpdate, err := store.GetFileConnection(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("load stale atomic update snapshot: %v", err)
	}

	keys := []string{newTestAuthorizedHostKey(t), newTestAuthorizedHostKey(t)}
	errs := make([]error, len(keys))
	var wg sync.WaitGroup
	for index, hostKey := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[index] = store.PinSFTPHostKey(
				context.Background(),
				connection.ID,
				connection.Host,
				22,
				hostKey,
			)
		}()
	}
	wg.Wait()

	winnerCount := 0
	for _, err := range errs {
		switch {
		case err == nil:
			winnerCount++
		case errors.Is(err, ErrSFTPHostKeyMismatch):
		default:
			t.Fatalf("unexpected concurrent pin error: %v", err)
		}
	}
	if winnerCount != 1 {
		t.Fatalf("concurrent different host keys produced %d winners, want 1", winnerCount)
	}

	stored, err := store.GetFileConnection(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("reload file connection: %v", err)
	}
	pinnedKey, _ := stored.ExtraConfig["host_key"].(string)
	if pinnedKey != keys[0] && pinnedKey != keys[1] {
		t.Fatalf("stored an unexpected host key")
	}
	if stored.ExtraConfig["future_option"] != "preserved" {
		t.Fatalf("pinning removed unrelated extra_config: %#v", stored.ExtraConfig)
	}
	staleUpdate.Name = "ordinary edit after concurrent pin"
	if err := store.UpdateFileConnection(context.Background(), staleUpdate); !errors.Is(err, ErrFileConnectionChanged) {
		t.Fatalf("stale ordinary update was not rejected: %v", err)
	}
	staleExplicitKey.ExtraConfig["host_key"] = newTestAuthorizedHostKey(t)
	if err := store.UpdateFileConnection(context.Background(), staleExplicitKey); !errors.Is(err, ErrFileConnectionChanged) {
		t.Fatalf("pre-pin stale key replacement was not rejected: %v", err)
	}
	staleProfile := profile
	staleProfile.SecretCiphertext = "must-roll-back"
	if _, err := store.UpdateFileConnectionWithCredential(context.Background(), staleAtomicUpdate, staleProfile, false); !errors.Is(err, ErrFileConnectionChanged) {
		t.Fatalf("stale atomic connection and credential update was not rejected: %v", err)
	}
	rolledBackProfile, ok, err := store.GetCredentialProfile(profile.ID)
	if err != nil || !ok {
		t.Fatalf("reload rolled-back credential profile: ok=%v err=%v", ok, err)
	}
	if rolledBackProfile.SecretCiphertext != profile.SecretCiphertext {
		t.Fatalf("failed connection edit changed credential ciphertext")
	}

	stored, err = store.GetFileConnection(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("reload after stale ordinary update: %v", err)
	}
	if stored.ExtraConfig["host_key"] != pinnedKey {
		t.Fatalf("ordinary update erased concurrently stored host key: %#v", stored.ExtraConfig)
	}
	stored.Name = "fresh ordinary edit"
	if err := store.UpdateFileConnection(context.Background(), stored); err != nil {
		t.Fatalf("apply fresh ordinary update: %v", err)
	}
	if stored.ExtraConfig["host_key"] != pinnedKey {
		t.Fatalf("fresh ordinary update erased stored host key: %#v", stored.ExtraConfig)
	}
	staleKeyReplacement, err := store.GetFileConnection(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("load stale key replacement: %v", err)
	}
	stored.Name = "newer ordinary edit"
	if err := store.UpdateFileConnection(context.Background(), stored); err != nil {
		t.Fatalf("apply newer ordinary edit: %v", err)
	}
	staleKeyReplacement.ExtraConfig["host_key"] = newTestAuthorizedHostKey(t)
	if err := store.UpdateFileConnection(context.Background(), staleKeyReplacement); !errors.Is(err, ErrFileConnectionChanged) {
		t.Fatalf("stale key replacement was not rejected: %v", err)
	}
	stored, err = store.GetFileConnection(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("reload after stale key replacement: %v", err)
	}
	if stored.ExtraConfig["host_key"] != pinnedKey {
		t.Fatalf("stale update replaced the trusted host key: %#v", stored.ExtraConfig)
	}
	if err := store.PinSFTPHostKey(context.Background(), connection.ID, connection.Host, 22, pinnedKey); err != nil {
		t.Fatalf("same host key was not idempotent: %v", err)
	}
	if err := store.PinSFTPHostKey(context.Background(), connection.ID, connection.Host, 22, newTestAuthorizedHostKey(t)); !errors.Is(err, ErrSFTPHostKeyMismatch) {
		t.Fatalf("different host key was not rejected: %v", err)
	}

	stored.Host = "sftp-edited.example.invalid"
	if err := store.UpdateFileConnection(context.Background(), stored); err != nil {
		t.Fatalf("edit file connection endpoint: %v", err)
	}
	if err := store.PinSFTPHostKey(context.Background(), connection.ID, connection.Host, 22, pinnedKey); !errors.Is(err, ErrFileConnectionChanged) {
		t.Fatalf("stale endpoint pin was not rejected: %v", err)
	}
}

func newTestAuthorizedHostKey(t *testing.T) string {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("create host signer: %v", err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
}
