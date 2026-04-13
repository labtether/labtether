package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/credentials"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

type testFileConnectionStore struct {
	items map[string]*persistence.FileConnection
}

func newTestFileConnectionStore(connections ...*persistence.FileConnection) *testFileConnectionStore {
	items := make(map[string]*persistence.FileConnection, len(connections))
	for _, conn := range connections {
		cloned := *conn
		if conn.ExtraConfig != nil {
			cloned.ExtraConfig = cloneAnyMapForTest(conn.ExtraConfig)
		}
		items[conn.ID] = &cloned
	}
	return &testFileConnectionStore{items: items}
}

func (s *testFileConnectionStore) ListFileConnections(_ context.Context) ([]persistence.FileConnection, error) {
	out := make([]persistence.FileConnection, 0, len(s.items))
	for _, conn := range s.items {
		cloned := *conn
		if conn.ExtraConfig != nil {
			cloned.ExtraConfig = cloneAnyMapForTest(conn.ExtraConfig)
		}
		out = append(out, cloned)
	}
	return out, nil
}

func (s *testFileConnectionStore) GetFileConnection(_ context.Context, id string) (*persistence.FileConnection, error) {
	conn, ok := s.items[id]
	if !ok {
		return nil, persistence.ErrNotFound
	}
	cloned := *conn
	if conn.ExtraConfig != nil {
		cloned.ExtraConfig = cloneAnyMapForTest(conn.ExtraConfig)
	}
	return &cloned, nil
}

func (s *testFileConnectionStore) CreateFileConnection(_ context.Context, fc *persistence.FileConnection) error {
	cloned := *fc
	if fc.ExtraConfig != nil {
		cloned.ExtraConfig = cloneAnyMapForTest(fc.ExtraConfig)
	}
	s.items[fc.ID] = &cloned
	return nil
}

func (s *testFileConnectionStore) UpdateFileConnection(_ context.Context, fc *persistence.FileConnection) error {
	if _, ok := s.items[fc.ID]; !ok {
		return persistence.ErrNotFound
	}
	cloned := *fc
	if fc.ExtraConfig != nil {
		cloned.ExtraConfig = cloneAnyMapForTest(fc.ExtraConfig)
	}
	s.items[fc.ID] = &cloned
	return nil
}

func (s *testFileConnectionStore) DeleteFileConnection(_ context.Context, id string) error {
	if _, ok := s.items[id]; !ok {
		return persistence.ErrNotFound
	}
	delete(s.items, id)
	return nil
}

func cloneAnyMapForTest(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func buildTestFileConnectionDeps(t *testing.T, store persistence.FileConnectionStore, credentialStore persistence.CredentialStore, secretsManager *secrets.Manager) *respkg.Deps {
	t.Helper()
	return &respkg.Deps{
		FileConnectionStore: store,
		CredentialStore:     credentialStore,
		SecretsManager:      secretsManager,
		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return shared.DecodeJSONBody(w, r, dst)
		},
	}
}

func seedFileConnectionCredential(t *testing.T, sut *apiServer, username, kind, secret, passphrase string) credentials.Profile {
	t.Helper()

	profileID := "cred-file-test"
	secretCiphertext, err := sut.secretsManager.EncryptString(secret, profileID)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	passphraseCiphertext := ""
	if passphrase != "" {
		passphraseCiphertext, err = sut.secretsManager.EncryptString(passphrase, profileID)
		if err != nil {
			t.Fatalf("encrypt passphrase: %v", err)
		}
	}
	profile, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:                   profileID,
		Name:                 "File Connection — Primary",
		Kind:                 kind,
		Username:             username,
		Description:          "Auto-created for file connection (sftp)",
		Status:               "active",
		SecretCiphertext:     secretCiphertext,
		PassphraseCiphertext: passphraseCiphertext,
	})
	if err != nil {
		t.Fatalf("create credential profile: %v", err)
	}
	return profile
}

func TestHandleFileConnectionsUpdatePersistsCredentialUsername(t *testing.T) {
	sut := newTestAPIServer(t)
	profile := seedFileConnectionCredential(t, sut, "old-user", credentials.KindSSHPassword, "old-secret", "")

	store := newTestFileConnectionStore(&persistence.FileConnection{
		ID:           "fconn-1",
		Name:         "Primary",
		Protocol:     "sftp",
		Host:         "files.example.test",
		InitialPath:  "/",
		CredentialID: &profile.ID,
	})
	deps := buildTestFileConnectionDeps(t, store, sut.credentialStore, sut.secretsManager)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/file-connections/fconn-1", bytes.NewBufferString(`{"username":"new-user"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleFileConnections(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	updated, ok, err := sut.credentialStore.GetCredentialProfile(profile.ID)
	if err != nil {
		t.Fatalf("get credential profile: %v", err)
	}
	if !ok {
		t.Fatal("expected credential profile to exist")
	}
	if updated.Username != "new-user" {
		t.Fatalf("username=%q, want %q", updated.Username, "new-user")
	}
	decrypted, err := sut.secretsManager.DecryptString(updated.SecretCiphertext, updated.ID)
	if err != nil {
		t.Fatalf("decrypt rotated secret: %v", err)
	}
	if decrypted != "old-secret" {
		t.Fatalf("secret=%q, want %q", decrypted, "old-secret")
	}
}

func TestHandleFileConnectionsUpdateRotatesCredentialKindAndSecret(t *testing.T) {
	sut := newTestAPIServer(t)
	profile := seedFileConnectionCredential(t, sut, "old-user", credentials.KindSSHPassword, "old-secret", "")

	store := newTestFileConnectionStore(&persistence.FileConnection{
		ID:           "fconn-1",
		Name:         "Primary",
		Protocol:     "sftp",
		Host:         "files.example.test",
		InitialPath:  "/",
		CredentialID: &profile.ID,
	})
	deps := buildTestFileConnectionDeps(t, store, sut.credentialStore, sut.secretsManager)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/file-connections/fconn-1", bytes.NewBufferString(`{"username":"deploy","auth_method":"private_key","secret":"PRIVATE-KEY","passphrase":"unlock-me"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleFileConnections(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	updated, ok, err := sut.credentialStore.GetCredentialProfile(profile.ID)
	if err != nil {
		t.Fatalf("get credential profile: %v", err)
	}
	if !ok {
		t.Fatal("expected credential profile to exist")
	}
	if updated.Kind != credentials.KindSSHPrivateKey {
		t.Fatalf("kind=%q, want %q", updated.Kind, credentials.KindSSHPrivateKey)
	}
	if updated.Username != "deploy" {
		t.Fatalf("username=%q, want %q", updated.Username, "deploy")
	}
	decryptedSecret, err := sut.secretsManager.DecryptString(updated.SecretCiphertext, updated.ID)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if decryptedSecret != "PRIVATE-KEY" {
		t.Fatalf("secret=%q, want %q", decryptedSecret, "PRIVATE-KEY")
	}
	decryptedPassphrase, err := sut.secretsManager.DecryptString(updated.PassphraseCiphertext, updated.ID)
	if err != nil {
		t.Fatalf("decrypt passphrase: %v", err)
	}
	if decryptedPassphrase != "unlock-me" {
		t.Fatalf("passphrase=%q, want %q", decryptedPassphrase, "unlock-me")
	}
}

func TestHandleFileConnectionsUpdateRejectsPrivateKeyTransitionWithoutSecret(t *testing.T) {
	sut := newTestAPIServer(t)
	profile := seedFileConnectionCredential(t, sut, "old-user", credentials.KindSSHPassword, "old-secret", "")

	store := newTestFileConnectionStore(&persistence.FileConnection{
		ID:           "fconn-1",
		Name:         "Primary",
		Protocol:     "sftp",
		Host:         "files.example.test",
		InitialPath:  "/",
		CredentialID: &profile.ID,
	})
	deps := buildTestFileConnectionDeps(t, store, sut.credentialStore, sut.secretsManager)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/file-connections/fconn-1", bytes.NewBufferString(`{"auth_method":"private_key"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleFileConnections(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	updated, ok, err := sut.credentialStore.GetCredentialProfile(profile.ID)
	if err != nil {
		t.Fatalf("get credential profile: %v", err)
	}
	if !ok {
		t.Fatal("expected credential profile to exist")
	}
	if updated.Kind != credentials.KindSSHPassword {
		t.Fatalf("kind=%q, want %q", updated.Kind, credentials.KindSSHPassword)
	}
	decrypted, err := sut.secretsManager.DecryptString(updated.SecretCiphertext, updated.ID)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if decrypted != "old-secret" {
		t.Fatalf("secret=%q, want %q", decrypted, "old-secret")
	}
}

func TestHandleFileConnectionsUpdateResponseIncludesUpdatedConnection(t *testing.T) {
	sut := newTestAPIServer(t)
	profile := seedFileConnectionCredential(t, sut, "old-user", credentials.KindSSHPassword, "old-secret", "")

	store := newTestFileConnectionStore(&persistence.FileConnection{
		ID:           "fconn-1",
		Name:         "Primary",
		Protocol:     "sftp",
		Host:         "files.example.test",
		InitialPath:  "/",
		CredentialID: &profile.ID,
	})
	deps := buildTestFileConnectionDeps(t, store, sut.credentialStore, sut.secretsManager)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/file-connections/fconn-1", bytes.NewBufferString(`{"host":"mirror.example.test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleFileConnections(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response struct {
		Connection persistence.FileConnection `json:"connection"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Connection.Host != "mirror.example.test" {
		t.Fatalf("host=%q, want %q", response.Connection.Host, "mirror.example.test")
	}
}
