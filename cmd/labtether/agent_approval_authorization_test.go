package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/auth"
)

// Operators could approve pending agents although only admins can mint the
// enrollment tokens that admit a host. Approval is now admin-only on both APIs.
func TestPendingAgentApprovalRequiresAdmin(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()
	handlers := sut.buildHTTPHandlers(nil, nil, nil)
	operator := createRoleSessionToken(t, sut, "operator-approval", auth.RoleOperator)
	admin := createRoleSessionToken(t, sut, "admin-approval", auth.RoleAdmin)

	for _, route := range []string{"/api/v1/agents/approve", "/api/v2/agents/pending/approve"} {
		post := func(session string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"asset_id":"pending-unknown"}`))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session})
			rec := httptest.NewRecorder()
			handlers[route](rec, req)
			return rec
		}
		if rec := post(operator); rec.Code != http.StatusForbidden {
			t.Fatalf("%s operator status=%d body=%s, want 403", route, rec.Code, rec.Body.String())
		}
		// The admin passes the role gate; the unknown pending agent is then 404.
		if rec := post(admin); rec.Code == http.StatusForbidden {
			t.Fatalf("%s admin was forbidden: %s", route, rec.Body.String())
		}
	}
}
