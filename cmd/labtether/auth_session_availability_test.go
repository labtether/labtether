package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coreauth "github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
)

type authAvailabilityStore struct {
	persistence.AuthStore
	validateSession func(string) (coreauth.Session, bool, error)
	getUserByID     func(string) (coreauth.User, bool, error)
}

func (s authAvailabilityStore) ValidateSession(tokenHash string) (coreauth.Session, bool, error) {
	return s.validateSession(tokenHash)
}

func (s authAvailabilityStore) GetUserByID(id string) (coreauth.User, bool, error) {
	if s.getUserByID == nil {
		return coreauth.User{}, false, nil
	}
	return s.getUserByID(id)
}

func TestCookieAuthStoreFailuresReturnServiceUnavailable(t *testing.T) {
	validSession := coreauth.Session{
		ID:        "sess-test",
		UserID:    "user-test",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	tests := []struct {
		name  string
		store persistence.AuthStore
	}{
		{
			name: "session validation failure",
			store: authAvailabilityStore{
				validateSession: func(string) (coreauth.Session, bool, error) {
					return coreauth.Session{}, false, errors.New("database timeout")
				},
			},
		},
		{
			name: "session user lookup failure",
			store: authAvailabilityStore{
				validateSession: func(string) (coreauth.Session, bool, error) {
					return validSession, true, nil
				},
				getUserByID: func(string) (coreauth.User, bool, error) {
					return coreauth.User{}, false, errors.New("database timeout")
				},
			},
		},
	}

	wrappers := []struct {
		name string
		wrap func(*apiServer, http.HandlerFunc) http.HandlerFunc
	}{
		{name: "general auth", wrap: func(s *apiServer, next http.HandlerFunc) http.HandlerFunc {
			return s.withAuth(next)
		}},
		{name: "self service auth", wrap: func(s *apiServer, next http.HandlerFunc) http.HandlerFunc {
			return s.withSelfServiceAuth(next)
		}},
	}

	for _, test := range tests {
		for _, wrapper := range wrappers {
			t.Run(test.name+"/"+wrapper.name, func(t *testing.T) {
				sut := &apiServer{authStore: test.store}
				called := false
				handler := wrapper.wrap(sut, func(w http.ResponseWriter, _ *http.Request) {
					called = true
					w.WriteHeader(http.StatusNoContent)
				})
				req := httptest.NewRequest(http.MethodGet, "/assets", nil)
				req.AddCookie(&http.Cookie{Name: coreauth.SessionCookieName, Value: "opaque-session-token"})
				rec := httptest.NewRecorder()

				handler(rec, req)

				if called {
					t.Fatal("protected handler ran while the authentication store was unavailable")
				}
				if rec.Code != http.StatusServiceUnavailable {
					t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
				}
				if got := rec.Header().Get("Cache-Control"); got != "no-store" {
					t.Fatalf("Cache-Control = %q, want no-store", got)
				}
				if got := rec.Header().Get("Retry-After"); got != "1" {
					t.Fatalf("Retry-After = %q, want 1", got)
				}
				if got := rec.Body.String(); got != "{\"error\":\"An internal error occurred.\"}\n" {
					t.Fatalf("body = %q, want sanitized service error", got)
				}
			})
		}
	}
}

func TestCookieAuthOutageDoesNotChangePrincipalToBearer(t *testing.T) {
	const ownerToken = "owner-token-for-auth-precedence-test"
	sut := &apiServer{
		authStore: authAvailabilityStore{
			validateSession: func(string) (coreauth.Session, bool, error) {
				return coreauth.Session{}, false, errors.New("database timeout")
			},
		},
		authValidator: coreauth.NewTokenValidator(ownerToken),
	}
	handler := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/assets", nil)
	req.AddCookie(&http.Cookie{Name: coreauth.SessionCookieName, Value: "opaque-session-token"})
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want fail-closed %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestMissingAuthStoreOnlyBlocksRequestsThatPresentSessionCookie(t *testing.T) {
	const ownerToken = "owner-token-without-auth-store"
	sut := &apiServer{authValidator: coreauth.NewTokenValidator(ownerToken)}
	handler := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	withoutCookie := httptest.NewRequest(http.MethodGet, "/assets", nil)
	withoutCookie.Header.Set("Authorization", "Bearer "+ownerToken)
	withoutCookieRec := httptest.NewRecorder()
	handler(withoutCookieRec, withoutCookie)
	if withoutCookieRec.Code != http.StatusNoContent {
		t.Fatalf("bearer-only status = %d, want %d", withoutCookieRec.Code, http.StatusNoContent)
	}

	withCookie := httptest.NewRequest(http.MethodGet, "/assets", nil)
	withCookie.AddCookie(&http.Cookie{Name: coreauth.SessionCookieName, Value: "opaque-session-token"})
	withCookie.Header.Set("Authorization", "Bearer "+ownerToken)
	withCookieRec := httptest.NewRecorder()
	handler(withCookieRec, withCookie)
	if withCookieRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("cookie-plus-bearer status = %d, want %d", withCookieRec.Code, http.StatusServiceUnavailable)
	}
}

func TestInvalidCookieSessionStillReturnsUnauthorized(t *testing.T) {
	sut := &apiServer{authStore: authAvailabilityStore{
		validateSession: func(string) (coreauth.Session, bool, error) {
			return coreauth.Session{}, false, nil
		},
	}}
	handler := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/assets", nil)
	req.AddCookie(&http.Cookie{Name: coreauth.SessionCookieName, Value: "invalid-session-token"})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
