package persistence

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/secrets"
)

func oidcMigrationTestManager(t *testing.T) *secrets.Manager {
	t.Helper()
	manager, err := secrets.NewManagerFromEncodedKey(base64.StdEncoding.EncodeToString([]byte("0123456789ABCDEF0123456789ABCDEF")))
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestPostgresMigrateLegacyOIDCClientSecretEncryptsAtomicallyAndIsIdempotent(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	original, existed, err := store.GetSystemSetting(ctx, OIDCSystemSettingsKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = store.PutSystemSetting(ctx, OIDCSystemSettingsKey, original)
		} else {
			_, _ = store.pool.Exec(ctx, `DELETE FROM system_settings WHERE key = $1`, OIDCSystemSettingsKey)
		}
	})

	const legacySecret = "legacy-database-oidc-secret"
	legacy := json.RawMessage(`{"enabled":true,"client_id":"client-1","client_secret":"` + legacySecret + `"}`)
	if err := store.PutSystemSetting(ctx, OIDCSystemSettingsKey, legacy); err != nil {
		t.Fatal(err)
	}
	manager := oidcMigrationTestManager(t)
	if err := store.MigrateLegacyOIDCClientSecret(ctx, manager); err != nil {
		t.Fatal(err)
	}
	migrated, found, err := store.GetSystemSetting(ctx, OIDCSystemSettingsKey)
	if err != nil || !found {
		t.Fatalf("load migrated setting found=%v err=%v", found, err)
	}
	if strings.Contains(string(migrated), legacySecret) || strings.Contains(string(migrated), `"client_secret"`) {
		t.Fatalf("plaintext survived migration: %s", migrated)
	}
	var stored struct {
		Ciphertext string `json:"client_secret_ciphertext"`
	}
	if err := json.Unmarshal(migrated, &stored); err != nil {
		t.Fatal(err)
	}
	plain, err := manager.DecryptString(stored.Ciphertext, OIDCClientSecretAAD)
	if err != nil || plain != legacySecret {
		t.Fatalf("decrypted secret = %q, %v", plain, err)
	}

	if err := store.MigrateLegacyOIDCClientSecret(ctx, manager); err != nil {
		t.Fatal(err)
	}
	again, _, err := store.GetSystemSetting(ctx, OIDCSystemSettingsKey)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(migrated) {
		t.Fatalf("idempotent migration rewrote ciphertext\nfirst: %s\nagain: %s", migrated, again)
	}

	const latestLegacySecret = "latest-old-writer-secret"
	both := json.RawMessage(`{"client_secret":"` + latestLegacySecret + `","client_secret_ciphertext":"` + stored.Ciphertext + `"}`)
	if err := store.PutSystemSetting(ctx, OIDCSystemSettingsKey, both); err != nil {
		t.Fatal(err)
	}
	if err := store.MigrateLegacyOIDCClientSecret(ctx, manager); err != nil {
		t.Fatal(err)
	}
	replaced, _, err := store.GetSystemSetting(ctx, OIDCSystemSettingsKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(replaced, &stored); err != nil {
		t.Fatal(err)
	}
	plain, err = manager.DecryptString(stored.Ciphertext, OIDCClientSecretAAD)
	if err != nil || plain != latestLegacySecret {
		t.Fatalf("latest plaintext did not replace old ciphertext: %q, %v", plain, err)
	}

	if err := store.PutSystemSetting(ctx, OIDCSystemSettingsKey, legacy); err != nil {
		t.Fatal(err)
	}
	if err := store.MigrateLegacyOIDCClientSecret(ctx, nil); err == nil {
		t.Fatal("legacy migration succeeded without an encryption manager")
	}
	unchanged, _, err := store.GetSystemSetting(ctx, OIDCSystemSettingsKey)
	if err != nil {
		t.Fatal(err)
	}
	var unchangedFields map[string]any
	if err := json.Unmarshal(unchanged, &unchangedFields); err != nil {
		t.Fatal(err)
	}
	if unchangedFields["client_secret"] != legacySecret || unchangedFields["client_secret_ciphertext"] != nil {
		t.Fatalf("failed migration changed secret fields: %s", unchanged)
	}
}
