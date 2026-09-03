package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

type oidcSettingsTestStore struct {
	raw      json.RawMessage
	found    bool
	putCalls int
}

func (s *oidcSettingsTestStore) GetSystemSetting(_ context.Context, key string) (json.RawMessage, bool, error) {
	if key != oidcSettingsKey || !s.found {
		return nil, false, nil
	}
	return append(json.RawMessage(nil), s.raw...), true, nil
}

func (s *oidcSettingsTestStore) PutSystemSetting(_ context.Context, key string, value json.RawMessage) error {
	if key != oidcSettingsKey {
		return nil
	}
	s.putCalls++
	s.raw = append(json.RawMessage(nil), value...)
	s.found = true
	return nil
}

func putOIDCSettings(t *testing.T, deps *Deps, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/settings/oidc", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	deps.HandleOIDCSettingsPut(rec, req)
	return rec
}

func encryptedOIDCTestSettings(t *testing.T, manager CredentialSecretsManager, secret string) json.RawMessage {
	t.Helper()
	ciphertext, err := manager.EncryptString(secret, persistence.OIDCClientSecretAAD)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(oidcDBSettings{ClientSecretCiphertext: ciphertext})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOIDCSettingsPutEncryptsClientSecretAndNeverReturnsIt(t *testing.T) {
	const knownSecret = "oidc-known-secret-never-store-plain"
	manager := testutil.TestSecretsManager(t)
	store := &oidcSettingsTestStore{}
	deps := &Deps{SettingsStore: store, SecretsManager: manager}

	rec := putOIDCSettings(t, deps, `{"enabled":false,"client_secret":"`+knownSecret+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if store.putCalls != 1 {
		t.Fatalf("put calls = %d, want 1", store.putCalls)
	}
	if strings.Contains(string(store.raw), knownSecret) || strings.Contains(rec.Body.String(), knownSecret) {
		t.Fatal("OIDC client secret escaped into storage or response")
	}
	if strings.Contains(rec.Body.String(), "client_secret\"") || strings.Contains(rec.Body.String(), "client_secret_ciphertext") {
		t.Fatalf("response exposed a secret field: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"client_secret_configured":true`) {
		t.Fatalf("response omitted configured state: %s", rec.Body.String())
	}

	var stored oidcDBSettings
	if err := json.Unmarshal(store.raw, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.LegacyClientSecret != "" || !strings.HasPrefix(stored.ClientSecretCiphertext, "v2:") {
		t.Fatalf("unexpected stored secret envelope: %#v", stored)
	}
	plain, err := manager.DecryptString(stored.ClientSecretCiphertext, persistence.OIDCClientSecretAAD)
	if err != nil || plain != knownSecret {
		t.Fatalf("decrypt stored secret = %q, %v", plain, err)
	}
}

func TestOIDCSettingsPutPreservesExistingSecretForCompatibleBlankInputs(t *testing.T) {
	const knownSecret = "preserved-oidc-secret"
	for _, body := range []string{
		`{"enabled":false}`,
		`{"enabled":false,"client_secret":""}`,
		`{"enabled":false,"client_secret":"` + oidcSecretMask + `"}`,
	} {
		t.Run(body, func(t *testing.T) {
			manager := testutil.TestSecretsManager(t)
			store := &oidcSettingsTestStore{raw: encryptedOIDCTestSettings(t, manager, knownSecret), found: true}
			deps := &Deps{SettingsStore: store, SecretsManager: manager}
			rec := putOIDCSettings(t, deps, body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
			}
			var stored oidcDBSettings
			if err := json.Unmarshal(store.raw, &stored); err != nil {
				t.Fatal(err)
			}
			plain, err := manager.DecryptString(stored.ClientSecretCiphertext, persistence.OIDCClientSecretAAD)
			if err != nil || plain != knownSecret {
				t.Fatalf("preserved secret = %q, %v", plain, err)
			}
		})
	}
}

func TestOIDCSettingsPutMigratesLegacyPlaintextAndExplicitClearRemovesSecret(t *testing.T) {
	const legacySecret = "legacy-oidc-plaintext"
	manager := testutil.TestSecretsManager(t)
	legacyRaw, err := json.Marshal(map[string]any{"enabled": false, "client_secret": legacySecret})
	if err != nil {
		t.Fatal(err)
	}
	store := &oidcSettingsTestStore{raw: legacyRaw, found: true}
	deps := &Deps{SettingsStore: store, SecretsManager: manager}

	rec := putOIDCSettings(t, deps, `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("migration status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(string(store.raw), legacySecret) || strings.Contains(string(store.raw), `"client_secret"`) {
		t.Fatalf("legacy plaintext survived authorized save: %s", store.raw)
	}

	rec = putOIDCSettings(t, deps, `{"enabled":false,"clear_client_secret":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(string(store.raw), "client_secret") {
		t.Fatalf("clear left secret fields in storage: %s", store.raw)
	}
	if !strings.Contains(rec.Body.String(), `"client_secret_configured":false`) {
		t.Fatalf("clear response did not report removal: %s", rec.Body.String())
	}
}

func TestOIDCSettingsSecretWritesFailClosedWithoutEncryption(t *testing.T) {
	store := &oidcSettingsTestStore{}
	deps := &Deps{SettingsStore: store}
	rec := putOIDCSettings(t, deps, `{"enabled":false,"client_secret":"must-not-store"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if store.putCalls != 0 || strings.Contains(string(store.raw), "must-not-store") {
		t.Fatal("plaintext write was not stopped")
	}
}

func TestOIDCSettingsGetReportsPresenceWithoutReturningSecretFields(t *testing.T) {
	manager := testutil.TestSecretsManager(t)
	store := &oidcSettingsTestStore{raw: encryptedOIDCTestSettings(t, manager, "hidden"), found: true}
	deps := &Deps{SettingsStore: store, OIDCRef: NewOIDCProviderRef(nil, false)}
	req := httptest.NewRequest(http.MethodGet, "/settings/oidc", nil)
	rec := httptest.NewRecorder()
	deps.HandleOIDCSettingsGet(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	configured, _ := payload["client_secret_configured"].(bool)
	_, exposedPlain := payload["client_secret"]
	_, exposedCiphertext := payload["client_secret_ciphertext"]
	if !configured || exposedPlain || exposedCiphertext {
		t.Fatalf("unsafe GET response: %s", rec.Body.String())
	}
}

func TestOIDCSettingsApplyWrongKeyFailsWithoutReplacingProviderState(t *testing.T) {
	manager := testutil.TestSecretsManager(t)
	wrongManager, err := secrets.NewManagerFromEncodedKey(base64.StdEncoding.EncodeToString([]byte("abcdefghijklmnopqrstuvwxyzABCDEF")))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := manager.EncryptString("hidden", persistence.OIDCClientSecretAAD)
	if err != nil {
		t.Fatal(err)
	}
	storeRaw, err := json.Marshal(oidcDBSettings{
		Enabled:                boolPointer(true),
		ClientSecretCiphertext: ciphertext,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &oidcSettingsTestStore{raw: storeRaw, found: true}
	ref := NewOIDCProviderRef(nil, true)
	deps := &Deps{
		SettingsStore:  store,
		SecretsManager: wrongManager,
		OIDCRef:        ref,
		EnforceRateLimit: func(http.ResponseWriter, *http.Request, string, int, time.Duration) bool {
			return true
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/oidc/apply", nil)
	rec := httptest.NewRecorder()
	deps.HandleOIDCSettingsApply(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if _, autoProvision := ref.Get(); !autoProvision {
		t.Fatal("failed apply replaced the prior provider state")
	}
	if strings.Contains(rec.Body.String(), string(store.raw)) || strings.Contains(rec.Body.String(), "hidden") {
		t.Fatal("apply error exposed secret material")
	}
}

func TestOIDCSettingsApplyCanDisableWithUnreadableDBSecret(t *testing.T) {
	activeProvider, _ := newMobileOIDCTestProvider(t)
	manager := testutil.TestSecretsManager(t)
	wrongManager, err := secrets.NewManagerFromEncodedKey(base64.StdEncoding.EncodeToString([]byte("abcdefghijklmnopqrstuvwxyzABCDEF")))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := manager.EncryptString("unreadable", persistence.OIDCClientSecretAAD)
	if err != nil {
		t.Fatal(err)
	}
	storeRaw, err := json.Marshal(oidcDBSettings{
		Enabled:                boolPointer(false),
		ClientSecretCiphertext: ciphertext,
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := NewOIDCProviderRef(activeProvider, true)
	deps := &Deps{
		SettingsStore:    &oidcSettingsTestStore{raw: storeRaw, found: true},
		SecretsManager:   wrongManager,
		OIDCRef:          ref,
		EnforceRateLimit: testutil.NoopRateLimit,
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/oidc/apply", nil)
	rec := httptest.NewRecorder()
	deps.HandleOIDCSettingsApply(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if provider, _ := ref.Get(); provider != nil {
		t.Fatal("disabled OIDC retained the old active provider")
	}
}

func TestOIDCSettingsApplyEnvironmentSecretOverridesUnreadableDBSecret(t *testing.T) {
	activeProvider, _ := newMobileOIDCTestProvider(t)
	_, replacement := newMobileOIDCTestProvider(t)
	t.Setenv("LABTETHER_OIDC_CLIENT_SECRET", "valid-env-override")
	t.Setenv("LABTETHER_OIDC_ALLOW_LOOPBACK", "true")
	t.Setenv("LABTETHER_OIDC_ALLOWED_ENDPOINT_ORIGINS", "")

	manager := testutil.TestSecretsManager(t)
	wrongManager, err := secrets.NewManagerFromEncodedKey(base64.StdEncoding.EncodeToString([]byte("abcdefghijklmnopqrstuvwxyzABCDEF")))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := manager.EncryptString("unreadable", persistence.OIDCClientSecretAAD)
	if err != nil {
		t.Fatal(err)
	}
	storeRaw, err := json.Marshal(oidcDBSettings{
		Enabled:                boolPointer(true),
		IssuerURL:              replacement.server.URL,
		ClientID:               "replacement-client",
		ClientSecretCiphertext: ciphertext,
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := NewOIDCProviderRef(activeProvider, false)
	deps := &Deps{
		SettingsStore:    &oidcSettingsTestStore{raw: storeRaw, found: true},
		SecretsManager:   wrongManager,
		OIDCRef:          ref,
		EnforceRateLimit: testutil.NoopRateLimit,
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/oidc/apply", nil)
	rec := httptest.NewRecorder()
	deps.HandleOIDCSettingsApply(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	provider, _ := ref.Get()
	if provider == nil || provider == activeProvider || provider.IssuerURL() != replacement.server.URL {
		t.Fatal("environment override did not replace the active provider")
	}
}

func TestOIDCSettingsEnvironmentSecretIsNotCopiedIntoStorage(t *testing.T) {
	const envSecret = "env-only-oidc-secret"
	t.Setenv("LABTETHER_OIDC_CLIENT_SECRET", envSecret)
	store := &oidcSettingsTestStore{}
	deps := &Deps{SettingsStore: store, SecretsManager: testutil.TestSecretsManager(t)}
	rec := putOIDCSettings(t, deps, `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(string(store.raw), envSecret) || strings.Contains(string(store.raw), "client_secret") {
		t.Fatalf("environment secret was copied into DB settings: %s", store.raw)
	}
	if !strings.Contains(rec.Body.String(), `"client_secret_configured":true`) {
		t.Fatalf("response did not report env secret: %s", rec.Body.String())
	}
}
