package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
)

type fakeBootstrapAuthStore struct {
	*fakeAdminBootstrapStore
	sessionsCreated int
}

func (f *fakeBootstrapAuthStore) GetUserByID(id string) (auth.User, bool, error) {
	if f.fakeAdminBootstrapStore.user.ID == id {
		return f.fakeAdminBootstrapStore.user, true, nil
	}
	return auth.User{}, false, nil
}

func (f *fakeBootstrapAuthStore) GetUserByOIDCSubject(provider, subject string) (auth.User, bool, error) {
	return auth.User{}, false, nil
}

func (f *fakeBootstrapAuthStore) CreateUserWithRole(username, passwordHash, role, authProvider, oidcSubject string) (auth.User, error) {
	return f.CreateUser(username, passwordHash)
}

func (f *fakeBootstrapAuthStore) SetUserTOTPSecret(id, encryptedSecret string) error {
	return nil
}

func (f *fakeBootstrapAuthStore) ConfirmUserTOTP(id, recoveryCodes string) error {
	return nil
}

func (f *fakeBootstrapAuthStore) ClearUserTOTP(id string) error {
	return nil
}

func (f *fakeBootstrapAuthStore) UpdateUserRecoveryCodes(id, recoveryCodes string) error {
	return nil
}

func (f *fakeBootstrapAuthStore) UpdateUserRole(id, role string) error {
	return nil
}

func (f *fakeBootstrapAuthStore) ConsumeRecoveryCode(userID, code string) (bool, error) {
	return false, nil
}

func (f *fakeBootstrapAuthStore) CreateAuthSession(userID, tokenHash string, expiresAt time.Time) (auth.Session, error) {
	f.sessionsCreated++
	return auth.Session{ID: "sess-1", UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt, CreatedAt: time.Now().UTC()}, nil
}

func (f *fakeBootstrapAuthStore) ValidateSession(tokenHash string) (auth.Session, bool, error) {
	return auth.Session{}, false, nil
}

func (f *fakeBootstrapAuthStore) DeleteSession(id string) error {
	return nil
}

func (f *fakeBootstrapAuthStore) DeleteSessionsByUserID(userID string) error {
	return nil
}

func (f *fakeBootstrapAuthStore) DeleteExpiredSessions() (int64, error) {
	return 0, nil
}

func (f *fakeBootstrapAuthStore) DeleteUser(id string) error {
	return nil
}

func (f *fakeBootstrapAuthStore) ListSessionsByUserID(userID string) ([]auth.Session, error) {
	return nil, nil
}

func TestHandleAuthBootstrapStatusReportsSetupRequired(t *testing.T) {
	sut := apiServer{authStore: &fakeBootstrapAuthStore{fakeAdminBootstrapStore: &fakeAdminBootstrapStore{}}}
	req := httptest.NewRequest(http.MethodGet, "/auth/bootstrap/status", nil)
	rec := httptest.NewRecorder()

	sut.handleAuthBootstrapStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		SetupRequired bool   `json:"setup_required"`
		Suggested     string `json:"suggested_username"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.SetupRequired {
		t.Fatalf("expected setup_required=true")
	}
	if payload.Suggested == "" {
		t.Fatalf("expected suggested username")
	}
}

func TestHandleAuthBootstrapSetupCreatesUserAndSession(t *testing.T) {
	store := &fakeBootstrapAuthStore{fakeAdminBootstrapStore: &fakeAdminBootstrapStore{}}
	sut := apiServer{authStore: store, authValidator: auth.NewTokenValidator("bootstrap-token")}
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", strings.NewReader(`{"username":"Owner.One","password":"correct-horse-battery"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer bootstrap-token")
	rec := httptest.NewRecorder()

	sut.handleAuthBootstrapSetup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if store.createCount != 1 {
		t.Fatalf("expected createCount=1, got %d", store.createCount)
	}
	if store.createUser != "owner.one" {
		t.Fatalf("expected normalized username owner.one, got %q", store.createUser)
	}
	if store.sessionsCreated != 1 {
		t.Fatalf("expected one created session, got %d", store.sessionsCreated)
	}
	if store.bootstrapCount != 1 {
		t.Fatalf("expected one bootstrap create, got %d", store.bootstrapCount)
	}
	if setCookie := rec.Header().Get("Set-Cookie"); !strings.Contains(setCookie, "labtether_session=") {
		t.Fatalf("expected session cookie, got %q", setCookie)
	}
}

func TestHandleAuthBootstrapSetupRejectsWhenAlreadyConfigured(t *testing.T) {
	store := &fakeBootstrapAuthStore{fakeAdminBootstrapStore: &fakeAdminBootstrapStore{list: []auth.User{{ID: "usr-1", Username: "owner"}}}}
	sut := apiServer{authStore: store, authValidator: auth.NewTokenValidator("bootstrap-token")}
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", strings.NewReader(`{"username":"owner","password":"correct-horse-battery"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer bootstrap-token")
	rec := httptest.NewRecorder()

	sut.handleAuthBootstrapSetup(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthBootstrapSetupRequiresServiceAuthorization(t *testing.T) {
	store := &fakeBootstrapAuthStore{fakeAdminBootstrapStore: &fakeAdminBootstrapStore{}}
	sut := apiServer{authStore: store, authValidator: auth.NewTokenValidator("bootstrap-token")}
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", strings.NewReader(`{"username":"owner","password":"correct-horse-battery"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	sut.handleAuthBootstrapSetup(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}
