package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/secrets"
)

const prometheusSecurityTestKey = "MDEyMzQ1Njc4OUFCQ0RFRjAxMjM0NTY3ODlBQkNERUY=" // gitleaks:allow -- deterministic test-only encryption key

func newPrometheusSecurityTestDeps(t *testing.T) (*Deps, *persistence.MemoryRuntimeSettingsStore) {
	t.Helper()
	manager, err := secrets.NewManagerFromEncodedKey(prometheusSecurityTestKey)
	if err != nil {
		t.Fatalf("NewManagerFromEncodedKey: %v", err)
	}
	store := persistence.NewMemoryRuntimeSettingsStore()
	return &Deps{
		RuntimeStore:   store,
		SecretsManager: manager,
		DecodeJSONBody: func(_ http.ResponseWriter, r *http.Request, dst any) error {
			return json.NewDecoder(r.Body).Decode(dst)
		},
	}, store
}

func seedPrometheusSecuritySettings(t *testing.T, d *Deps, store *persistence.MemoryRuntimeSettingsStore) {
	t.Helper()
	normalized, err := shared.NormalizeRuntimeOverrides(map[string]string{
		runtimesettings.KeyPrometheusRemoteWriteURL:      "https://metrics.example.test/api/v1/write",
		runtimesettings.KeyPrometheusRemoteWriteUsername: "alice",
		runtimesettings.KeyPrometheusRemoteWritePassword: "stored-test-secret",
	})
	if err != nil {
		t.Fatalf("NormalizeRuntimeOverrides: %v", err)
	}
	stored, err := shared.PrepareRuntimeOverridesForStorage(normalized, d.SecretsManager)
	if err != nil {
		t.Fatalf("PrepareRuntimeOverridesForStorage: %v", err)
	}
	if _, err := store.SaveRuntimeSettingOverrides(stored); err != nil {
		t.Fatalf("SaveRuntimeSettingOverrides: %v", err)
	}
}

func prometheusTestRequest(t *testing.T, body string, scopes []string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, prometheusTestConnectionRoute, strings.NewReader(body))
	return req.WithContext(apiv2.ContextWithScopes(req.Context(), scopes))
}

func TestPrometheusStoredPasswordReuseRequiresScopeAndExactTarget(t *testing.T) {
	d, store := newPrometheusSecurityTestDeps(t)
	seedPrometheusSecuritySettings(t, d, store)

	var called bool
	var gotURL, gotUsername, gotPassword string
	d.PrometheusRemoteWriteTester = func(_ context.Context, url, username, password string) PrometheusTestConnectionResponse {
		called = true
		gotURL, gotUsername, gotPassword = url, username, password
		return PrometheusTestConnectionResponse{Success: true}
	}

	body := `{"url":"https://metrics.example.test/api/v1/write","username":"alice","use_stored_password":true}`
	rec := httptest.NewRecorder()
	d.HandlePrometheusTestConnection(rec, prometheusTestRequest(t, body, []string{"settings:write"}))
	if rec.Code != http.StatusForbidden || called {
		t.Fatalf("stored reuse without credentials:use = %d, called=%v", rec.Code, called)
	}

	called = false
	rec = httptest.NewRecorder()
	mismatch := `{"url":"https://attacker.example.test/write","username":"alice","use_stored_password":true}`
	d.HandlePrometheusTestConnection(rec, prometheusTestRequest(t, mismatch, []string{"settings:write", "credentials:use"}))
	if rec.Code != http.StatusBadRequest || called {
		t.Fatalf("mismatched target = %d, called=%v", rec.Code, called)
	}

	called = false
	rec = httptest.NewRecorder()
	d.HandlePrometheusTestConnection(rec, prometheusTestRequest(t, body, []string{"settings:write", "credentials:use"}))
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("exact scoped reuse = %d, called=%v, body=%s", rec.Code, called, rec.Body.String())
	}
	if gotURL != "https://metrics.example.test/api/v1/write" || gotUsername != "alice" || gotPassword != "stored-test-secret" {
		t.Fatal("connection tester did not receive the exact stored credential tuple")
	}
}

func TestRuntimeSettingsSecretUpdateEncryptedRedactedAndOmittedUpdatePreserved(t *testing.T) {
	d, store := newPrometheusSecurityTestDeps(t)
	first := httptest.NewRequest(http.MethodPatch, runtimeSettingsRoute, bytes.NewBufferString(
		`{"values":{"prometheus.remote_write_password":"first-secret"}}`,
	))
	firstRec := httptest.NewRecorder()
	d.HandleRuntimeSettings(firstRec, first)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first update = %d: %s", firstRec.Code, firstRec.Body.String())
	}
	if strings.Contains(firstRec.Body.String(), "first-secret") || strings.Contains(firstRec.Body.String(), "v2:") {
		t.Fatalf("secret update response leaked material: %s", firstRec.Body.String())
	}
	raw, _ := store.ListRuntimeSettingOverrides()
	firstCiphertext := raw[runtimesettings.KeyPrometheusRemoteWritePassword]
	if !strings.HasPrefix(firstCiphertext, "v2:") || strings.Contains(firstCiphertext, "first-secret") {
		t.Fatal("secret override was not encrypted at rest")
	}

	second := httptest.NewRequest(http.MethodPatch, runtimeSettingsRoute, bytes.NewBufferString(
		`{"values":{"prometheus.remote_write_username":"bob"}}`,
	))
	secondRec := httptest.NewRecorder()
	d.HandleRuntimeSettings(secondRec, second)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second update = %d: %s", secondRec.Code, secondRec.Body.String())
	}
	raw, _ = store.ListRuntimeSettingOverrides()
	if raw[runtimesettings.KeyPrometheusRemoteWritePassword] != firstCiphertext {
		t.Fatal("omitting the password changed its stored ciphertext")
	}
}

func TestRuntimeSettingsConcurrentSecretReadsAndWrites(t *testing.T) {
	d, _ := newPrometheusSecurityTestDeps(t)
	const workers = 24
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var req *http.Request
			if i%3 == 0 {
				req = httptest.NewRequest(http.MethodGet, runtimeSettingsRoute, nil)
			} else {
				body := `{"values":{"prometheus.remote_write_password":"concurrent-secret"}}`
				if i%3 == 2 {
					body = `{"values":{"prometheus.remote_write_username":"alice"}}`
				}
				req = httptest.NewRequest(http.MethodPatch, runtimeSettingsRoute, strings.NewReader(body))
			}
			rec := httptest.NewRecorder()
			d.HandleRuntimeSettings(rec, req)
			if rec.Code != http.StatusOK {
				errs <- rec.Body.String()
			}
			if strings.Contains(rec.Body.String(), "concurrent-secret") || strings.Contains(rec.Body.String(), "v2:") {
				errs <- "response leaked secret material"
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestRuntimeSettingsPrometheusMutationValidatesBeforeSaveAndReloadsLiveRuntime(t *testing.T) {
	d, store := newPrometheusSecurityTestDeps(t)
	var reloads atomic.Int32
	d.ApplyPrometheusRemoteWriteSettings = func() error {
		reloads.Add(1)
		return nil
	}
	valid := httptest.NewRequest(http.MethodPatch, runtimeSettingsRoute, strings.NewReader(
		`{"values":{"prometheus.remote_write_enabled":"true","prometheus.remote_write_url":"https://metrics.example.test/api/v1/write","prometheus.remote_write_interval":"10s"}}`,
	))
	validRec := httptest.NewRecorder()
	d.HandleRuntimeSettings(validRec, valid)
	if validRec.Code != http.StatusOK || reloads.Load() != 1 {
		t.Fatalf("valid update status=%d reloads=%d body=%s", validRec.Code, reloads.Load(), validRec.Body.String())
	}

	invalid := httptest.NewRequest(http.MethodPatch, runtimeSettingsRoute, strings.NewReader(
		`{"values":{"prometheus.remote_write_url":"https://metrics.example.test/write?token=must-not-persist"}}`,
	))
	invalidRec := httptest.NewRecorder()
	d.HandleRuntimeSettings(invalidRec, invalid)
	if invalidRec.Code != http.StatusBadRequest || reloads.Load() != 1 || strings.Contains(invalidRec.Body.String(), "must-not-persist") {
		t.Fatalf("invalid update status=%d reloads=%d body=%s", invalidRec.Code, reloads.Load(), invalidRec.Body.String())
	}
	overrides, _ := store.ListRuntimeSettingOverrides()
	if strings.Contains(overrides[runtimesettings.KeyPrometheusRemoteWriteURL], "must-not-persist") {
		t.Fatal("invalid endpoint was persisted")
	}

	d.ApplyPrometheusRemoteWriteSettings = func() error { return errors.New("reload failed") }
	failing := httptest.NewRequest(http.MethodPatch, runtimeSettingsRoute, strings.NewReader(
		`{"values":{"prometheus.remote_write_interval":"30s"}}`,
	))
	failingRec := httptest.NewRecorder()
	d.HandleRuntimeSettings(failingRec, failing)
	if failingRec.Code != http.StatusServiceUnavailable || strings.Contains(failingRec.Body.String(), "reload failed") {
		t.Fatalf("reload failure status=%d body=%s", failingRec.Code, failingRec.Body.String())
	}
	overrides, _ = store.ListRuntimeSettingOverrides()
	if overrides[runtimesettings.KeyPrometheusRemoteWriteInterval] != "30s" {
		t.Fatal("honestly failed runtime reload did not retain the durable setting for startup retry")
	}
}

func TestRuntimeSettingsPrometheusPasswordPreservesExactBytes(t *testing.T) {
	d, store := newPrometheusSecurityTestDeps(t)
	req := httptest.NewRequest(http.MethodPatch, runtimeSettingsRoute, strings.NewReader(
		`{"values":{"prometheus.remote_write_password":"  exact secret bytes  "}}`,
	))
	rec := httptest.NewRecorder()
	d.HandleRuntimeSettings(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "exact secret bytes") {
		t.Fatalf("password update status=%d body=%s", rec.Code, rec.Body.String())
	}
	raw, _ := store.ListRuntimeSettingOverrides()
	plain, err := d.SecretsManager.DecryptString(raw[runtimesettings.KeyPrometheusRemoteWritePassword], "labtether:runtime-setting:"+runtimesettings.KeyPrometheusRemoteWritePassword)
	if err != nil {
		t.Fatalf("DecryptString: %v", err)
	}
	if plain != "  exact secret bytes  " {
		t.Fatalf("stored password bytes = %q", plain)
	}
}
