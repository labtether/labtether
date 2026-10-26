package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
)

// hostDataRouteCases pair a registered route key with a concrete request path.
// Every path reads host contents or opens an interactive session.
var hostDataRouteCases = []struct{ route, path string }{
	{"/files/", "/files/srv1/download?path=/etc/shadow"},
	{"/files/", "/files/srv1/list?path=/"},
	{"/terminal/sessions", "/terminal/sessions"},
	{"/terminal/sessions/", "/terminal/sessions/sess-1/stream"},
	{"/terminal/commands/recent", "/terminal/commands/recent"},
	{"/terminal/persistent-sessions", "/terminal/persistent-sessions"},
	{"/terminal/snippets", "/terminal/snippets"},
	{"/api/v2/terminal/history", "/api/v2/terminal/history"},
	{"/api/v2/terminal/sessions", "/api/v2/terminal/sessions"},
	{"/desktop/sessions", "/desktop/sessions"},
	{"/desktop/sessions/", "/desktop/sessions/sess-1/stream"},
	{"/desktop/sessions/", "/desktop/sessions/sess-1/audio"},
	{"/desktop/diagnose/", "/desktop/diagnose/srv1"},
	{"/recordings", "/recordings"},
	{"/recordings/", "/recordings/rec-1"},
	{"/recordings/", "/recordings/rec-1/download"},
	{"/api/v1/file-connections", "/api/v1/file-connections"},
	{"/api/v1/file-connections/", "/api/v1/file-connections/conn-1/download?path=/x"},
	{"/api/v1/file-transfers", "/api/v1/file-transfers"},
	{"/api/v2/file-transfers", "/api/v2/file-transfers"},
	{"/processes/", "/processes/srv1"},
	{"/cron/", "/cron/srv1"},
	{"/users/", "/users/srv1"},
	{"/logs/journal/", "/logs/journal/srv1"},
	{"/api/v2/assets/", "/api/v2/assets/srv1/files/download?path=/etc/shadow"},
	{"/api/v2/assets/", "/api/v2/assets/srv1/processes"},
	{"/api/v2/assets/", "/api/v2/assets/srv1/logs"},
	{"/api/v2/assets/", "/api/v2/assets/srv1/cron"},
	{"/api/v1/nodes/", "/api/v1/nodes/srv1/clipboard"},
	{"/api/v1/docker/containers/", "/api/v1/docker/containers/ct-1/logs"},
	{"/api/v2/docker/containers/", "/api/v2/docker/containers/ct-1/logs"},
	{"/portainer/assets/", "/portainer/assets/pt-1/containers/abc/exec"},
	{"/portainer/assets/", "/portainer/assets/pt-1/containers/abc/%20exec"},
	{"/portainer/assets/", "/portainer/assets/pt-1/containers/abc/inspect"},
	{"/portainer/assets/", "/portainer/assets/pt-1/stacks/3/compose"},
	{"/api/v2/portainer/assets/", "/api/v2/portainer/assets/pt-1/containers/abc/logs"},
}

func createRoleSessionToken(t *testing.T, sut *apiServer, username, role string) string {
	t.Helper()
	user, err := sut.authStore.CreateUserWithRole(username, "unused", role, "local", "")
	if err != nil {
		t.Fatalf("create %s user: %v", role, err)
	}
	raw := username + "-session-token"
	if _, err := sut.authStore.CreateAuthSession(user.ID, auth.HashToken(raw), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create %s session: %v", role, err)
	}
	return raw
}

func hostDataGET(path, sessionToken string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	// Streams are WebSocket upgrades; the gate must apply before any upgrade.
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	if sessionToken != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionToken})
	}
	return req
}

func TestViewerSessionIsDeniedEveryHostDataRoute(t *testing.T) {
	sut := newTestAPIServer(t)
	viewer := createRoleSessionToken(t, sut, "viewer-host-data", auth.RoleViewer)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, tc := range hostDataRouteCases {
		t.Run(tc.path, func(t *testing.T) {
			handler := handlers[tc.route]
			if handler == nil {
				t.Fatalf("route %s is not registered", tc.route)
			}
			rec := httptest.NewRecorder()
			handler(rec, hostDataGET(tc.path, viewer))
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), hostDataRoleDeniedMessage) {
				t.Fatalf("viewer GET %s status=%d body=%s, want 403 %q", tc.path, rec.Code, rec.Body.String(), hostDataRoleDeniedMessage)
			}
		})
	}
}

func TestViewerAPIKeyWithWildcardScopeIsDeniedHostFiles(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleViewer, []string{"*"}, nil)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	rec := invokeLegacyRoute(t, handlers["/files/"], http.MethodGet, "/files/srv1/download?path=/etc/shadow", key, "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), hostDataRoleDeniedMessage) {
		t.Fatalf("viewer API key file download status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOperatorSessionPassesHostDataRoleGate(t *testing.T) {
	sut := newTestAPIServer(t)
	operator := createRoleSessionToken(t, sut, "operator-host-data", auth.RoleOperator)

	for _, tc := range hostDataRouteCases {
		t.Run(tc.path, func(t *testing.T) {
			called := false
			handler := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			rec := httptest.NewRecorder()
			handler(rec, hostDataGET(tc.path, operator))
			if !called || rec.Code != http.StatusNoContent {
				t.Fatalf("operator GET %s status=%d called=%t body=%s", tc.path, rec.Code, called, rec.Body.String())
			}
		})
	}
}

func TestViewerKeepsInventoryAndStatusReads(t *testing.T) {
	sut := newTestAPIServer(t)
	viewer := createRoleSessionToken(t, sut, "viewer-inventory", auth.RoleViewer)
	for _, path := range []string{
		"/assets", "/api/v2/assets/srv1", "/api/v2/assets/srv1/services", "/api/v2/assets/srv1/packages",
		"/services/srv1", "/packages/srv1", "/disks/srv1", "/network/srv1", "/logs/query",
		"/api/v1/docker/containers/ct-1/stats", "/portainer/assets/pt-1/containers",
		"/api/v1/nodes/srv1/displays", "/alerts/instances", "/auth/users",
	} {
		called := false
		handler := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		})
		rec := httptest.NewRecorder()
		handler(rec, hostDataGET(path, viewer))
		if !called {
			t.Fatalf("viewer GET %s was blocked: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestViewerStreamTicketCannotOpenTerminalStream(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.streamTicketStore.Tickets = map[string]streamTicket{
		"viewer-ticket": {
			SessionID: "sess-1",
			ActorID:   "viewer-user",
			Role:      auth.RoleViewer,
			ExpiresAt: time.Now().UTC().Add(time.Minute),
		},
	}
	called := false
	handler := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) { called = true })
	rec := httptest.NewRecorder()
	handler(rec, hostDataGET("/terminal/sessions/sess-1/stream?ticket=viewer-ticket", ""))
	if called || rec.Code != http.StatusForbidden {
		t.Fatalf("viewer ticket opened terminal stream: status=%d called=%t", rec.Code, called)
	}
}

func TestMCPHostDataReadRequiresOperatorRole(t *testing.T) {
	viewerCtx := contextWithPrincipal(context.Background(), "viewer-user", auth.RoleViewer)
	if err := mcpAuthorizeHostDataRead(viewerCtx); err == nil {
		t.Fatal("viewer MCP principal was allowed to read host data")
	}
	operatorCtx := contextWithPrincipal(context.Background(), "operator-user", auth.RoleOperator)
	if err := mcpAuthorizeHostDataRead(operatorCtx); err != nil {
		t.Fatalf("operator MCP principal was denied host data: %v", err)
	}
}
