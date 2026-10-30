package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
	sharedpkg "github.com/labtether/labtether/internal/hubapi/shared"
)

// signedInStreamContext returns a request context for a real user with a live
// sign-in session, as the session middleware would build it.
func signedInStreamContext(t *testing.T, sut *apiServer, username, role string) context.Context {
	t.Helper()
	user, err := sut.authStore.CreateUserWithRole(username, "unused", role, "local", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	session, err := sut.authStore.CreateAuthSession(user.ID, auth.HashToken(username+"-session"), time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return sharedpkg.ContextWithAuthSessionID(contextWithPrincipal(context.Background(), user.ID, role), session.ID)
}

// A stream ticket issued just before logout or demotion must not open a root
// terminal afterwards, because nothing would ever close that stream.
func TestStreamTicketRefusedAfterLogoutOrDemotion(t *testing.T) {
	sut := newTestAPIServer(t)
	redeem := func(ticket string) (int, bool) {
		called := false
		handler := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess-1/stream?ticket="+ticket, nil))
		return rec.Code, called
	}

	loggedOut := signedInStreamContext(t, sut, "logout-user", auth.RoleOperator)
	ticket, _, err := sut.issueStreamTicket(loggedOut, "sess-1")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	if err := sut.authStore.DeleteSession(sharedpkg.AuthSessionIDFromContext(loggedOut)); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if code, called := redeem(ticket); called || code != http.StatusUnauthorized {
		t.Fatalf("ticket after logout: status=%d called=%t, want 401", code, called)
	}

	demoted := signedInStreamContext(t, sut, "demoted-user", auth.RoleAdmin)
	ticket, _, err = sut.issueStreamTicket(demoted, "sess-1")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	if err := sut.authStore.UpdateUserRole(userIDFromContext(demoted), auth.RoleOperator); err != nil {
		t.Fatalf("demote user: %v", err)
	}
	if code, called := redeem(ticket); called || code != http.StatusUnauthorized {
		t.Fatalf("ticket after demotion: status=%d called=%t, want 401", code, called)
	}

	active := signedInStreamContext(t, sut, "active-user", auth.RoleOperator)
	ticket, _, err = sut.issueStreamTicket(active, "sess-1")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	if code, called := redeem(ticket); !called || code != http.StatusNoContent {
		t.Fatalf("ticket for an active session: status=%d called=%t, want 204", code, called)
	}
}
