package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/auth"
)

// The console's shared token must not be an owner credential: a compromised
// console process may read routing settings and nothing else.
func TestConsoleServiceTokenOnlyReadsRuntimeSettings(t *testing.T) {
	sut := newTestAPIServer(t)
	token := consoleServiceToken("install-api-token")
	if token == "" || token == "install-api-token" {
		t.Fatalf("console token = %q, want a derived value", token)
	}
	sut.consoleServiceValidator = auth.NewTokenValidator(token)

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/settings/runtime", http.StatusNoContent},
		{http.MethodPatch, "/settings/runtime", http.StatusForbidden},
		{http.MethodGet, "/assets", http.StatusForbidden},
		{http.MethodPost, "/admin/reset", http.StatusForbidden},
	} {
		called := false
		handler := sut.withAuth(func(w http.ResponseWriter, r *http.Request) {
			called = true
			if userRoleFromContext(r.Context()) == auth.RoleOwner {
				t.Fatal("console token reached a handler as owner")
			}
			w.WriteHeader(http.StatusNoContent)
		})
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != tc.want || called != (tc.want == http.StatusNoContent) {
			t.Fatalf("%s %s: status=%d called=%t, want %d", tc.method, tc.path, rec.Code, called, tc.want)
		}
	}
}
