package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
)

func TestCredentialProfileCreateAndRotatePreserveExactSecretBytes(t *testing.T) {
	store := persistence.NewMemoryCredentialStore()
	manager := testutil.TestSecretsManager(t)
	deps := &Deps{
		CredentialStore:   store,
		SecretsManager:    manager,
		EnforceRateLimit:  testutil.NoopRateLimit,
		UserIDFromContext: apiv2.PrincipalActorID,
	}
	ctx := apiv2.ContextWithPrincipal(context.Background(), "usr_exact_bytes", "admin")

	createSecret := " \tsecret with edges\n "
	createPassphrase := "  passphrase\t"
	createPayload, err := json.Marshal(credentials.CreateProfileRequest{
		Name:       "Exact bytes",
		Kind:       credentials.KindSSHPrivateKey,
		Secret:     createSecret,
		Passphrase: createPassphrase,
	})
	if err != nil {
		t.Fatal(err)
	}
	createReq := httptest.NewRequest(http.MethodPost, "/credentials/profiles", strings.NewReader(string(createPayload))).WithContext(ctx)
	createRec := httptest.NewRecorder()
	deps.HandleCredentialProfiles(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResponse struct {
		Profile credentials.Profile `json:"profile"`
	}
	if err = json.Unmarshal(createRec.Body.Bytes(), &createResponse); err != nil {
		t.Fatal(err)
	}
	stored, ok, err := store.GetCredentialProfile(createResponse.Profile.ID)
	if err != nil || !ok {
		t.Fatalf("load created profile: ok=%v err=%v", ok, err)
	}
	if stored.CreatedBy != "usr_exact_bytes" {
		t.Fatalf("created_by=%q", stored.CreatedBy)
	}
	assertDecryptedCredentialValue(t, manager, stored.SecretCiphertext, stored.ID, createSecret)
	assertDecryptedCredentialValue(t, manager, stored.PassphraseCiphertext, stored.ID, createPassphrase)
	if strings.Contains(createRec.Body.String(), "usr_exact_bytes") || strings.Contains(createRec.Body.String(), "v2:") {
		t.Fatalf("create response exposed storage-only data: %s", createRec.Body.String())
	}

	rotateSecret := "\n rotated secret \t"
	rotatePassphrase := " rotated passphrase "
	rotatePayload, err := json.Marshal(credentials.RotateProfileRequest{
		Secret:     rotateSecret,
		Passphrase: rotatePassphrase,
		Reason:     "scheduled exact-byte rotation",
	})
	if err != nil {
		t.Fatal(err)
	}
	rotateReq := httptest.NewRequest(
		http.MethodPost,
		"/credentials/profiles/"+stored.ID+"/rotate",
		strings.NewReader(string(rotatePayload)),
	).WithContext(ctx)
	rotateRec := httptest.NewRecorder()
	deps.HandleCredentialProfileActions(rotateRec, rotateReq)
	if rotateRec.Code != http.StatusOK {
		t.Fatalf("rotate status=%d body=%s", rotateRec.Code, rotateRec.Body.String())
	}
	stored, ok, err = store.GetCredentialProfile(stored.ID)
	if err != nil || !ok {
		t.Fatalf("load rotated profile: ok=%v err=%v", ok, err)
	}
	assertDecryptedCredentialValue(t, manager, stored.SecretCiphertext, stored.ID, rotateSecret)
	assertDecryptedCredentialValue(t, manager, stored.PassphraseCiphertext, stored.ID, rotatePassphrase)
}

func TestCredentialProfileDeleteRejectsReferencesAndHubIdentity(t *testing.T) {
	store := persistence.NewMemoryCredentialStore()
	created, err := store.CreateCredentialProfile(credentials.Profile{ID: "cred_in_use", Name: "In use", Kind: credentials.KindSSHPassword})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveAssetTerminalConfig(credentials.AssetTerminalConfig{
		AssetID:             "asset-secret-canary",
		CredentialProfileID: created.ID,
	}); err != nil {
		t.Fatal(err)
	}
	deps := &Deps{CredentialStore: store}
	rec := httptest.NewRecorder()
	deps.HandleCredentialProfileActions(rec, httptest.NewRequest(http.MethodDelete, "/credentials/profiles/"+created.ID, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "asset_terminal_configs") || strings.Contains(rec.Body.String(), "asset-secret-canary") {
		t.Fatalf("reference response was not redacted and useful: %s", rec.Body.String())
	}
	if _, ok, _ := store.GetCredentialProfile(created.ID); !ok {
		t.Fatal("referenced profile was deleted")
	}

	if err = store.DeleteAssetTerminalConfig("asset-secret-canary"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	deps.HandleCredentialProfileActions(rec, httptest.NewRequest(http.MethodDelete, "/credentials/profiles/"+created.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unreferenced delete status=%d body=%s", rec.Code, rec.Body.String())
	}

	identity, err := store.CreateCredentialProfile(credentials.Profile{ID: "cred_hub_identity", Name: "Hub", Kind: credentials.KindHubSSHIdentity})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	deps.HandleCredentialProfileActions(rec, httptest.NewRequest(http.MethodDelete, "/credentials/profiles/"+identity.ID, nil))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "protected") {
		t.Fatalf("hub identity delete status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCredentialProfilePayloadAndPathBounds(t *testing.T) {
	deps := &Deps{
		CredentialStore:  persistence.NewMemoryCredentialStore(),
		SecretsManager:   testutil.TestSecretsManager(t),
		EnforceRateLimit: testutil.NoopRateLimit,
	}

	oversized := `{"name":"large","kind":"ssh_password","secret":"` + strings.Repeat("x", maxCredentialBodyBytes) + `"}`
	rec := httptest.NewRecorder()
	deps.HandleCredentialProfiles(rec, httptest.NewRequest(http.MethodPost, "/credentials/profiles", strings.NewReader(oversized)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized create status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	deps.HandleCredentialProfileActions(rec, httptest.NewRequest(http.MethodGet, "/credentials/profiles/"+strings.Repeat("x", maxCredentialProfileIDLen+1), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("oversized path status=%d body=%s", rec.Code, rec.Body.String())
	}

	if err := validateCreateProfileRequest(credentials.CreateProfileRequest{
		Name:   "metadata",
		Kind:   credentials.KindSSHPassword,
		Secret: "secret",
		Metadata: map[string]string{
			"oversized": strings.Repeat("x", maxCredentialMetadataValueLen+1),
		},
	}); err == nil {
		t.Fatal("oversized metadata value was accepted")
	}
}

func assertDecryptedCredentialValue(t *testing.T, manager CredentialSecretsManager, ciphertext, aad, want string) {
	t.Helper()
	got, err := manager.DecryptString(ciphertext, aad)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("decrypted value=%q want=%q", got, want)
	}
}
