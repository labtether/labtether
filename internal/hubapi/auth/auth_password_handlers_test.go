package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coreauth "github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
)

type failingSessionListAuthStore struct {
	*memAuthStore
}

func (s failingSessionListAuthStore) ListSessionsByUserID(string) ([]coreauth.Session, error) {
	return nil, errors.New("injected session-list failure")
}

type failingSessionDeleteAuthStore struct {
	*memAuthStore
}

func (s failingSessionDeleteAuthStore) DeleteSession(string) error {
	return errors.New("injected session-delete failure")
}

type failingAllSessionsDeleteAuthStore struct {
	*memAuthStore
}

func (s failingAllSessionsDeleteAuthStore) DeleteSessionsByUserID(string) error {
	return errors.New("injected all-session-delete failure")
}

func TestChangePasswordPreservesExactBytesAndRetainsOnlyCurrentSession(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	const currentPassword = "  current password  "
	const newPassword = "  replacement password  "
	hash, err := coreauth.HashPassword(currentPassword)
	if err != nil {
		t.Fatalf("hash current password: %v", err)
	}
	user, err := store.CreateUserWithRole("password-user", hash, coreauth.RoleViewer, "local", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	deps.UserIDFromContext = func(context.Context) string { return user.ID }

	const currentToken = "current-session-token"
	if _, err := store.CreateAuthSession(user.ID, coreauth.HashToken(currentToken), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create current session: %v", err)
	}
	if _, err := store.CreateAuthSession(user.ID, coreauth.HashToken("other-session-token"), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create other session: %v", err)
	}

	rec := passwordChangeRequest(t, deps, currentToken, currentPassword, newPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("password change status = %d, body = %s", rec.Code, rec.Body.String())
	}
	updated, ok, err := store.GetUserByID(user.ID)
	if err != nil || !ok {
		t.Fatalf("load updated user: ok=%t err=%v", ok, err)
	}
	if !coreauth.CheckPassword(newPassword, updated.PasswordHash) {
		t.Fatal("exact replacement password was not stored")
	}
	if coreauth.CheckPassword("replacement password", updated.PasswordHash) {
		t.Fatal("replacement password was silently trimmed")
	}
	sessions, err := store.ListSessionsByUserID(user.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].TokenHash != coreauth.HashToken(currentToken) {
		t.Fatalf("retained sessions = %#v, want only current session", sessions)
	}
}

func TestChangePasswordDoesNotChangeCredentialWhenSessionRevocationCannotBeProven(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		wrap  func(*memAuthStore) persistence.AuthStore
		other bool
	}{
		{
			name: "list failure",
			wrap: func(store *memAuthStore) persistence.AuthStore {
				return failingSessionListAuthStore{memAuthStore: store}
			},
		},
		{
			name: "delete failure",
			wrap: func(store *memAuthStore) persistence.AuthStore {
				return failingSessionDeleteAuthStore{memAuthStore: store}
			},
			other: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			deps, store := newTestAuthDeps(t)
			const currentPassword = "current-password"
			const newPassword = "replacement-password"
			hash, err := coreauth.HashPassword(currentPassword)
			if err != nil {
				t.Fatalf("hash password: %v", err)
			}
			user, err := store.CreateUserWithRole("password-failure-user", hash, coreauth.RoleViewer, "local", "")
			if err != nil {
				t.Fatalf("create user: %v", err)
			}
			deps.UserIDFromContext = func(context.Context) string { return user.ID }
			const currentToken = "current-session-token"
			if _, err := store.CreateAuthSession(user.ID, coreauth.HashToken(currentToken), time.Now().UTC().Add(time.Hour)); err != nil {
				t.Fatalf("create current session: %v", err)
			}
			if testCase.other {
				if _, err := store.CreateAuthSession(user.ID, coreauth.HashToken("other-session-token"), time.Now().UTC().Add(time.Hour)); err != nil {
					t.Fatalf("create other session: %v", err)
				}
			}
			deps.AuthStore = testCase.wrap(store)

			rec := passwordChangeRequest(t, deps, currentToken, currentPassword, newPassword)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("password change status = %d, want 500", rec.Code)
			}
			unchanged, ok, err := store.GetUserByID(user.ID)
			if err != nil || !ok {
				t.Fatalf("load unchanged user: ok=%t err=%v", ok, err)
			}
			if !coreauth.CheckPassword(currentPassword, unchanged.PasswordHash) || coreauth.CheckPassword(newPassword, unchanged.PasswordHash) {
				t.Fatal("credential changed despite an unproven session revocation")
			}
		})
	}
}

func TestAdminPasswordResetPreservesExactBytesAndFailsBeforeCredentialChange(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		storeFails bool
		wantStatus int
	}{
		{name: "success", wantStatus: http.StatusOK},
		{name: "session revocation failure", storeFails: true, wantStatus: http.StatusInternalServerError},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			deps, store := newTestAuthDeps(t)
			const currentPassword = "current-password"
			const newPassword = "  exact admin reset  "
			hash, err := coreauth.HashPassword(currentPassword)
			if err != nil {
				t.Fatalf("hash password: %v", err)
			}
			user, err := store.CreateUserWithRole("admin-reset-user", hash, coreauth.RoleViewer, "local", "")
			if err != nil {
				t.Fatalf("create user: %v", err)
			}
			if _, err := store.CreateAuthSession(user.ID, coreauth.HashToken("active"), time.Now().UTC().Add(time.Hour)); err != nil {
				t.Fatalf("create session: %v", err)
			}
			if testCase.storeFails {
				deps.AuthStore = failingAllSessionsDeleteAuthStore{memAuthStore: store}
			}

			rec := performJSONRequest(t, http.MethodPatch, "/auth/users/"+user.ID, map[string]any{
				"password": newPassword,
			}, func(w http.ResponseWriter, r *http.Request) {
				deps.HandleAuthUserActions(w, r)
			})
			if rec.Code != testCase.wantStatus {
				t.Fatalf("reset status = %d, want %d; body=%s", rec.Code, testCase.wantStatus, rec.Body.String())
			}
			updated, ok, err := store.GetUserByID(user.ID)
			if err != nil || !ok {
				t.Fatalf("load user: ok=%t err=%v", ok, err)
			}
			if testCase.storeFails {
				if !coreauth.CheckPassword(currentPassword, updated.PasswordHash) || coreauth.CheckPassword(newPassword, updated.PasswordHash) {
					t.Fatal("admin reset changed credential after revocation failure")
				}
				return
			}
			if !coreauth.CheckPassword(newPassword, updated.PasswordHash) || coreauth.CheckPassword("exact admin reset", updated.PasswordHash) {
				t.Fatal("admin reset did not preserve exact password bytes")
			}
			sessions, err := store.ListSessionsByUserID(user.ID)
			if err != nil || len(sessions) != 0 {
				t.Fatalf("admin reset sessions = %d, err=%v", len(sessions), err)
			}
		})
	}
}

func TestBootstrapPasswordPreservesExactBytes(t *testing.T) {
	t.Setenv("LABTETHER_SETUP_TOKEN", "test-bootstrap-token")
	deps, store := newTestAuthDeps(t)
	const password = "  exact bootstrap password  "
	body, err := json.Marshal(BootstrapSetupRequest{Username: "bootstrap-user", Password: password})
	if err != nil {
		t.Fatalf("encode bootstrap request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(BootstrapSetupTokenHeader(), "test-bootstrap-token")
	rec := httptest.NewRecorder()
	deps.HandleAuthBootstrapSetup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d, body = %s", rec.Code, rec.Body.String())
	}
	user, ok, err := store.GetUserByUsername("bootstrap-user")
	if err != nil || !ok {
		t.Fatalf("load bootstrap user: ok=%t err=%v", ok, err)
	}
	if !coreauth.CheckPassword(password, user.PasswordHash) {
		t.Fatal("exact bootstrap password was not stored")
	}
	if coreauth.CheckPassword("exact bootstrap password", user.PasswordHash) {
		t.Fatal("bootstrap password was silently trimmed")
	}
}

func passwordChangeRequest(
	t *testing.T,
	deps *Deps,
	rawSessionToken string,
	currentPassword string,
	newPassword string,
) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"current_password": currentPassword,
		"new_password":     newPassword,
	})
	if err != nil {
		t.Fatalf("encode password change: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/me/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: coreauth.SessionCookieName, Value: rawSessionToken})
	rec := httptest.NewRecorder()
	deps.HandleChangePassword(rec, req)
	return rec
}
