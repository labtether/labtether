package shared

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/secrets"
)

const runtimeSettingsTestKey = "MDEyMzQ1Njc4OUFCQ0RFRjAxMjM0NTY3ODlBQkNERUY=" // gitleaks:allow -- deterministic test-only encryption key

func newRuntimeSettingsTestSecrets(t *testing.T) *secrets.Manager {
	t.Helper()
	manager, err := secrets.NewManagerFromEncodedKey(runtimeSettingsTestKey)
	if err != nil {
		t.Fatalf("NewManagerFromEncodedKey: %v", err)
	}
	return manager
}

func TestBuildRuntimeSettingsPayloadMigratesAndRedactsSensitiveOverride(t *testing.T) {
	const secret = "remote-write-super-secret"
	store := persistence.NewMemoryRuntimeSettingsStore()
	if _, err := store.SaveRuntimeSettingOverrides(map[string]string{
		runtimesettings.KeyPrometheusRemoteWritePassword: secret,
	}); err != nil {
		t.Fatalf("seed legacy override: %v", err)
	}
	manager := newRuntimeSettingsTestSecrets(t)

	payload, err := BuildRuntimeSettingsPayload(store, manager)
	if err != nil {
		t.Fatalf("BuildRuntimeSettingsPayload: %v", err)
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(rawPayload), secret) || strings.Contains(string(rawPayload), "v2:") {
		t.Fatalf("public payload leaked sensitive material: %s", rawPayload)
	}
	if _, ok := payload.Overrides[runtimesettings.KeyPrometheusRemoteWritePassword]; ok {
		t.Fatal("sensitive override must not appear in public overrides map")
	}

	var passwordEntry *RuntimeSettingEntry
	for i := range payload.Settings {
		if payload.Settings[i].Key == runtimesettings.KeyPrometheusRemoteWritePassword {
			passwordEntry = &payload.Settings[i]
			break
		}
	}
	if passwordEntry == nil {
		t.Fatal("password definition missing")
	}
	if !passwordEntry.Sensitive || !passwordEntry.Configured || passwordEntry.Source != string(runtimesettings.SourceUI) {
		t.Fatalf("unexpected password metadata: %+v", *passwordEntry)
	}
	if passwordEntry.EnvValue != "" || passwordEntry.OverrideValue != "" || passwordEntry.EffectiveValue != "" {
		t.Fatalf("password values were not redacted: %+v", *passwordEntry)
	}

	stored, err := store.ListRuntimeSettingOverrides()
	if err != nil {
		t.Fatalf("ListRuntimeSettingOverrides: %v", err)
	}
	ciphertext := stored[runtimesettings.KeyPrometheusRemoteWritePassword]
	if !strings.HasPrefix(ciphertext, "v2:") || strings.Contains(ciphertext, secret) {
		t.Fatalf("legacy plaintext was not migrated: %q", ciphertext)
	}
	plain, err := manager.DecryptString(ciphertext, runtimeSettingSecretAAD(runtimesettings.KeyPrometheusRemoteWritePassword))
	if err != nil || plain != secret {
		t.Fatalf("decrypt migrated value = %q, %v", plain, err)
	}
	if _, err := manager.DecryptString(ciphertext, runtimeSettingSecretAAD(runtimesettings.KeyPrometheusRemoteWriteUsername)); err == nil {
		t.Fatal("ciphertext transplant to another setting key must fail")
	}
}

func TestBuildRuntimeSettingsPayloadRedactsSensitiveEnvironmentValue(t *testing.T) {
	const secret = "environment-only-secret"
	t.Setenv("LABTETHER_PROMETHEUS_REMOTE_WRITE_PASSWORD", secret)
	payload, err := BuildRuntimeSettingsPayload(persistence.NewMemoryRuntimeSettingsStore(), nil)
	if err != nil {
		t.Fatalf("BuildRuntimeSettingsPayload: %v", err)
	}
	raw, _ := json.Marshal(payload)
	if strings.Contains(string(raw), secret) {
		t.Fatalf("environment secret leaked: %s", raw)
	}
	for _, entry := range payload.Settings {
		if entry.Key == runtimesettings.KeyPrometheusRemoteWritePassword {
			if !entry.Sensitive || !entry.Configured || entry.Source != string(runtimesettings.SourceDocker) {
				t.Fatalf("unexpected environment metadata: %+v", entry)
			}
			return
		}
	}
	t.Fatal("password definition missing")
}

func TestPrepareRuntimeOverridesForStorageRequiresSecretsManager(t *testing.T) {
	_, err := PrepareRuntimeOverridesForStorage(map[string]string{
		runtimesettings.KeyPrometheusRemoteWritePassword: "do-not-persist",
	}, nil)
	if err == nil {
		t.Fatal("expected sensitive write to fail without secrets manager")
	}

	stored, err := PrepareRuntimeOverridesForStorage(map[string]string{
		runtimesettings.KeyPrometheusRemoteWritePassword: "new-secret",
		runtimesettings.KeyPrometheusRemoteWriteUsername: "alice",
	}, newRuntimeSettingsTestSecrets(t))
	if err != nil {
		t.Fatalf("PrepareRuntimeOverridesForStorage: %v", err)
	}
	if stored[runtimesettings.KeyPrometheusRemoteWritePassword] == "new-secret" ||
		!strings.HasPrefix(stored[runtimesettings.KeyPrometheusRemoteWritePassword], "v2:") {
		t.Fatalf("password was not encrypted: %#v", stored)
	}
	if stored[runtimesettings.KeyPrometheusRemoteWriteUsername] != "alice" {
		t.Fatalf("non-sensitive value unexpectedly changed: %#v", stored)
	}
}

func TestBuildRuntimeSettingsPayloadRejectsCiphertextTransplant(t *testing.T) {
	manager := newRuntimeSettingsTestSecrets(t)
	ciphertext, err := manager.EncryptString("secret", runtimeSettingSecretAAD(runtimesettings.KeyPrometheusRemoteWriteUsername))
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	store := persistence.NewMemoryRuntimeSettingsStore()
	_, _ = store.SaveRuntimeSettingOverrides(map[string]string{
		runtimesettings.KeyPrometheusRemoteWritePassword: ciphertext,
	})
	if _, err := BuildRuntimeSettingsPayload(store, manager); err == nil {
		t.Fatal("expected wrong-AAD ciphertext to be rejected")
	}
}
