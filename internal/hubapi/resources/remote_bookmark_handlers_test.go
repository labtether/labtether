package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
)

type memoryRemoteBookmarkStore struct {
	mu        sync.Mutex
	bookmarks map[string]persistence.RemoteBookmark
	nextID    int
}

func newMemoryRemoteBookmarkStore() *memoryRemoteBookmarkStore {
	return &memoryRemoteBookmarkStore{bookmarks: make(map[string]persistence.RemoteBookmark)}
}

func cloneRemoteBookmark(value persistence.RemoteBookmark) persistence.RemoteBookmark {
	if value.CredentialID != nil {
		id := *value.CredentialID
		value.CredentialID = &id
	}
	value.HasCredentials = value.CredentialID != nil && strings.TrimSpace(*value.CredentialID) != ""
	return value
}

func (s *memoryRemoteBookmarkStore) ListRemoteBookmarks(context.Context) ([]persistence.RemoteBookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]persistence.RemoteBookmark, 0, len(s.bookmarks))
	for _, value := range s.bookmarks {
		out = append(out, cloneRemoteBookmark(value))
	}
	return out, nil
}

func (s *memoryRemoteBookmarkStore) GetRemoteBookmark(_ context.Context, id string) (*persistence.RemoteBookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.bookmarks[id]
	if !ok {
		return nil, persistence.ErrNotFound
	}
	cloned := cloneRemoteBookmark(value)
	return &cloned, nil
}

func (s *memoryRemoteBookmarkStore) CreateRemoteBookmark(_ context.Context, bookmark *persistence.RemoteBookmark) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	bookmark.ID = "bookmark-" + string(rune('0'+s.nextID))
	now := time.Now().UTC()
	bookmark.CreatedAt = now
	bookmark.UpdatedAt = now
	s.bookmarks[bookmark.ID] = cloneRemoteBookmark(*bookmark)
	return nil
}

func (s *memoryRemoteBookmarkStore) UpdateRemoteBookmark(_ context.Context, bookmark persistence.RemoteBookmark) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bookmarks[bookmark.ID]; !ok {
		return persistence.ErrNotFound
	}
	bookmark.UpdatedAt = time.Now().UTC()
	s.bookmarks[bookmark.ID] = cloneRemoteBookmark(bookmark)
	return nil
}

func (s *memoryRemoteBookmarkStore) DeleteRemoteBookmark(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bookmarks[id]; !ok {
		return persistence.ErrNotFound
	}
	delete(s.bookmarks, id)
	return nil
}

func newRemoteBookmarkTestDeps(t *testing.T) (*Deps, *memoryRemoteBookmarkStore, *persistence.MemoryCredentialStore, *[]audit.Event) {
	t.Helper()
	bookmarks := newMemoryRemoteBookmarkStore()
	credentialStore := persistence.NewMemoryCredentialStore()
	auditEvents := make([]audit.Event, 0, 4)
	deps := &Deps{
		RemoteBookmarkStore: bookmarks,
		CredentialStore:     credentialStore,
		SecretsManager:      testutil.TestSecretsManager(t),
		DecodeJSONBody:      shared.DecodeJSONBody,
		PrincipalActorID: func(ctx context.Context) string {
			return apiv2.PrincipalActorID(ctx)
		},
		AppendAuditEventBestEffort: func(event audit.Event, _ string) {
			auditEvents = append(auditEvents, event)
		},
	}
	return deps, bookmarks, credentialStore, &auditEvents
}

func remoteBookmarkRequest(t *testing.T, deps *Deps, method, path, payload string, scopes []string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithPrincipal(req.Context(), "user-remote", "owner")
	if scopes != nil {
		ctx = apiv2.ContextWithScopes(ctx, scopes)
	}
	req = req.WithContext(ctx)
	recorder := httptest.NewRecorder()
	deps.HandleRemoteBookmarks(recorder, req)
	return recorder
}

func TestRemoteBookmarkCredentialsEncryptedRedactedAndAuditedOnReveal(t *testing.T) {
	deps, bookmarks, credentialStore, auditEvents := newRemoteBookmarkTestDeps(t)
	secret := "synthetic-remote-secret"
	created := remoteBookmarkRequest(t, deps, http.MethodPost, remoteBookmarkAPIPrefix,
		`{"label":"QA RDP","protocol":"rdp","host":"192.0.2.10","port":3389,"username":"qa-user","password":"`+secret+`"}`, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), secret) || strings.Contains(created.Body.String(), "credential_id") {
		t.Fatalf("create response exposed credential material or profile reference: %s", created.Body.String())
	}
	var response remoteBookmarkResponse
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if !response.HasCredentials {
		t.Fatal("create response did not report configured credentials")
	}

	bookmark, err := bookmarks.GetRemoteBookmark(context.Background(), response.ID)
	if err != nil || bookmark.CredentialID == nil {
		t.Fatalf("stored bookmark credential link: bookmark=%+v err=%v", bookmark, err)
	}
	profile, ok, err := credentialStore.GetCredentialProfile(*bookmark.CredentialID)
	if err != nil || !ok {
		t.Fatalf("stored credential profile: ok=%v err=%v", ok, err)
	}
	if profile.SecretCiphertext == secret || strings.Contains(profile.SecretCiphertext, secret) {
		t.Fatal("credential secret was not encrypted at rest")
	}
	if !isOwnedRemoteBookmarkCredential(profile, response.ID) || profile.Kind != credentials.KindRDPPassword {
		t.Fatalf("credential ownership/kind mismatch: %+v", profile)
	}

	revealed := remoteBookmarkRequest(t, deps, http.MethodGet,
		remoteBookmarkAPIPrefix+"/"+response.ID+"/credentials", "", []string{"credentials:use"})
	if revealed.Code != http.StatusOK {
		t.Fatalf("reveal status=%d body=%s", revealed.Code, revealed.Body.String())
	}
	var revealPayload map[string]any
	if err := json.Unmarshal(revealed.Body.Bytes(), &revealPayload); err != nil {
		t.Fatalf("decode reveal: %v", err)
	}
	if revealPayload["username"] != "qa-user" || revealPayload["password"] != secret {
		t.Fatalf("reveal payload mismatch: %+v", revealPayload)
	}
	if got := revealed.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("credential reveal response is cacheable: Cache-Control=%q", got)
	}
	if len(*auditEvents) < 2 || (*auditEvents)[len(*auditEvents)-1].Type != "remote_bookmark.credential.revealed" {
		t.Fatalf("expected credential reveal audit event, got %+v", *auditEvents)
	}
}

func TestRemoteBookmarkCredentialScopeRequiredForWriteAndReveal(t *testing.T) {
	deps, bookmarks, _, auditEvents := newRemoteBookmarkTestDeps(t)
	deniedCreate := remoteBookmarkRequest(t, deps, http.MethodPost, remoteBookmarkAPIPrefix,
		`{"label":"Denied","protocol":"vnc","host":"192.0.2.20","port":5900,"password":"secret"}`,
		[]string{"assets:read"})
	if deniedCreate.Code != http.StatusForbidden {
		t.Fatalf("denied create status=%d body=%s", deniedCreate.Code, deniedCreate.Body.String())
	}
	listed, _ := bookmarks.ListRemoteBookmarks(context.Background())
	if len(listed) != 0 {
		t.Fatalf("unauthorized create persisted %d bookmarks", len(listed))
	}

	created := remoteBookmarkRequest(t, deps, http.MethodPost, remoteBookmarkAPIPrefix,
		`{"label":"Allowed","protocol":"vnc","host":"192.0.2.21","port":5900,"password":"secret"}`, nil)
	var response remoteBookmarkResponse
	_ = json.Unmarshal(created.Body.Bytes(), &response)
	deniedReveal := remoteBookmarkRequest(t, deps, http.MethodGet,
		remoteBookmarkAPIPrefix+"/"+response.ID+"/credentials", "", []string{"assets:read"})
	if deniedReveal.Code != http.StatusForbidden {
		t.Fatalf("denied reveal status=%d body=%s", deniedReveal.Code, deniedReveal.Body.String())
	}
	last := (*auditEvents)[len(*auditEvents)-1]
	if last.Decision != "denied" || last.Reason != "insufficient_scope" {
		t.Fatalf("denied reveal audit mismatch: %+v", last)
	}
}

func TestRemoteBookmarkRejectsCrossBookmarkOwnedCredentialReference(t *testing.T) {
	deps, bookmarks, credentialStore, _ := newRemoteBookmarkTestDeps(t)
	first := remoteBookmarkRequest(t, deps, http.MethodPost, remoteBookmarkAPIPrefix,
		`{"label":"Owner","protocol":"rdp","host":"192.0.2.30","port":3389,"password":"secret"}`, nil)
	var firstResponse remoteBookmarkResponse
	_ = json.Unmarshal(first.Body.Bytes(), &firstResponse)
	firstBookmark, _ := bookmarks.GetRemoteBookmark(context.Background(), firstResponse.ID)
	if firstBookmark.CredentialID == nil {
		t.Fatal("owner bookmark missing credential")
	}

	second := remoteBookmarkRequest(t, deps, http.MethodPost, remoteBookmarkAPIPrefix,
		`{"label":"Reference","protocol":"rdp","host":"192.0.2.31","port":3389,"credential_id":"`+*firstBookmark.CredentialID+`"}`,
		[]string{"credentials:use"})
	if second.Code != http.StatusBadRequest {
		t.Fatalf("cross-bookmark reference status=%d body=%s", second.Code, second.Body.String())
	}
	if _, ok, err := credentialStore.GetCredentialProfile(*firstBookmark.CredentialID); err != nil || !ok {
		t.Fatalf("rejected cross-bookmark reference affected owner credential: ok=%v err=%v", ok, err)
	}

	deletedOwner := remoteBookmarkRequest(t, deps, http.MethodDelete,
		remoteBookmarkAPIPrefix+"/"+firstResponse.ID, "", []string{"credentials:use"})
	if deletedOwner.Code != http.StatusNoContent {
		t.Fatalf("owner delete status=%d body=%s", deletedOwner.Code, deletedOwner.Body.String())
	}
	if _, ok, _ := credentialStore.GetCredentialProfile(*firstBookmark.CredentialID); ok {
		t.Fatal("owner bookmark deletion did not remove its managed credential")
	}
}

func TestRemoteBookmarkRelinkingSameOwnedCredentialDoesNotDeleteIt(t *testing.T) {
	deps, bookmarks, credentialStore, _ := newRemoteBookmarkTestDeps(t)
	created := remoteBookmarkRequest(t, deps, http.MethodPost, remoteBookmarkAPIPrefix,
		`{"label":"Owner","protocol":"rdp","host":"192.0.2.32","port":3389,"password":"secret"}`, nil)
	var response remoteBookmarkResponse
	_ = json.Unmarshal(created.Body.Bytes(), &response)
	bookmark, _ := bookmarks.GetRemoteBookmark(context.Background(), response.ID)
	if bookmark.CredentialID == nil {
		t.Fatal("bookmark missing credential")
	}

	updated := remoteBookmarkRequest(t, deps, http.MethodPut, remoteBookmarkAPIPrefix+"/"+response.ID,
		`{"credential_id":"`+*bookmark.CredentialID+`"}`, []string{"credentials:use"})
	if updated.Code != http.StatusOK {
		t.Fatalf("same credential relink status=%d body=%s", updated.Code, updated.Body.String())
	}
	if _, ok, err := credentialStore.GetCredentialProfile(*bookmark.CredentialID); err != nil || !ok {
		t.Fatalf("same credential relink deleted active profile: ok=%v err=%v", ok, err)
	}
}

func TestRemoteBookmarkManagedCredentialNameStaysWithinInventoryLimit(t *testing.T) {
	deps, bookmarks, credentialStore, _ := newRemoteBookmarkTestDeps(t)
	label := strings.Repeat("a", remoteBookmarkMaxLabelLength)
	created := remoteBookmarkRequest(t, deps, http.MethodPost, remoteBookmarkAPIPrefix,
		`{"label":"`+label+`","protocol":"vnc","host":"192.0.2.33","port":5900,"password":"secret"}`, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("long-label create status=%d body=%s", created.Code, created.Body.String())
	}
	var response remoteBookmarkResponse
	_ = json.Unmarshal(created.Body.Bytes(), &response)
	bookmark, _ := bookmarks.GetRemoteBookmark(context.Background(), response.ID)
	profile, ok, err := credentialStore.GetCredentialProfile(*bookmark.CredentialID)
	if err != nil || !ok {
		t.Fatalf("load managed credential: ok=%v err=%v", ok, err)
	}
	if len(profile.Name) > remoteBookmarkProfileNameMax {
		t.Fatalf("managed credential name length=%d, max=%d", len(profile.Name), remoteBookmarkProfileNameMax)
	}
}

func TestRemoteBookmarkRejectsCredentialKindMismatch(t *testing.T) {
	deps, _, credentialStore, _ := newRemoteBookmarkTestDeps(t)
	profile, err := credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               "cred-rdp-only",
		Name:             "RDP only",
		Kind:             credentials.KindRDPPassword,
		Status:           "active",
		SecretCiphertext: "not-read-in-this-test",
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	response := remoteBookmarkRequest(t, deps, http.MethodPost, remoteBookmarkAPIPrefix,
		`{"label":"Wrong kind","protocol":"vnc","host":"192.0.2.40","port":5900,"credential_id":"`+profile.ID+`"}`,
		[]string{"credentials:use"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("kind mismatch status=%d body=%s", response.Code, response.Body.String())
	}
}
