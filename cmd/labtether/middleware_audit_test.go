package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
)

func TestAuditMiddlewarePreservesWebSocketHijacker(t *testing.T) {
	sut := newTestAPIServer(t)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	handler := sut.auditMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		if err := conn.WriteMessage(websocket.TextMessage, []byte("ready")); err != nil {
			t.Errorf("write websocket message: %v", err)
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	conn, response, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:]+"/ws/events", nil)
	if err != nil {
		if response != nil {
			t.Fatalf("websocket dial failed with HTTP %d: %v", response.StatusCode, err)
		}
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.Close()

	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket message: %v", err)
	}
	if messageType != websocket.TextMessage || string(payload) != "ready" {
		t.Fatalf("unexpected websocket message type=%d payload=%q", messageType, payload)
	}
}

// Requests through the console must record the user's address, not the
// console's; a client that is not a trusted proxy cannot choose its address.
func TestAuditMiddlewareRecordsForwardedClientOnlyFromTrustedPeer(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.auditEventCh = make(chan audit.Event, 2)
	handler := sut.auditMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	record := func(peer string) map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		req.RemoteAddr = peer + ":40000"
		req.Header.Set("X-Forwarded-For", "198.51.100.7")
		handler.ServeHTTP(httptest.NewRecorder(), req)
		return (<-sut.auditEventCh).Details
	}

	if got := record("127.0.0.1"); got["client_ip"] != "198.51.100.7" || got["peer_ip"] != "127.0.0.1" {
		t.Fatalf("trusted proxy request recorded client=%v peer=%v", got["client_ip"], got["peer_ip"])
	}
	if got := record("192.0.2.44"); got["client_ip"] != "192.0.2.44" || got["peer_ip"] != "192.0.2.44" {
		t.Fatalf("untrusted peer chose its recorded address: client=%v peer=%v", got["client_ip"], got["peer_ip"])
	}
}

// The console polls several reads every few seconds. Storing each one buried
// people's actions under "API request GET" rows, so only changes are kept:
// refused ones read "denied", and machine traffic stays in the hub log.
func TestAuditMiddlewareKeepsChangesNotReads(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.auditEventCh = make(chan audit.Event, 1)
	cases := []struct {
		method, path string
		status       int
		readOnly     bool
		kept         bool
		decision     string
	}{
		{http.MethodGet, "/status/aggregate/live", http.StatusOK, false, false, ""},
		{http.MethodGet, "/auth/users", http.StatusForbidden, false, false, ""},
		{http.MethodPost, "/ws/events/ticket", http.StatusOK, true, false, ""},
		{http.MethodPost, "/assets/heartbeat", http.StatusUnauthorized, false, false, ""},
		{http.MethodPost, "/api/v2/webhooks", http.StatusCreated, false, true, ""},
		{http.MethodDelete, "/api/v2/webhooks/wh-1", http.StatusForbidden, false, true, "denied"},
		{http.MethodPost, "/auth/login", http.StatusTooManyRequests, false, true, "denied"},
		{http.MethodPut, "/settings/runtime", http.StatusInternalServerError, false, true, "failed"},
	}
	for _, tc := range cases {
		handler := sut.auditMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.readOnly {
				noteAuditReadOnly(r.Context())
			}
			w.WriteHeader(tc.status)
		}))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tc.method, tc.path, nil))
		select {
		case event := <-sut.auditEventCh:
			if !tc.kept || event.Decision != tc.decision {
				t.Errorf("%s %s %d: kept with decision %q, want kept=%v decision %q", tc.method, tc.path, tc.status, event.Decision, tc.kept, tc.decision)
			}
		default:
			if tc.kept {
				t.Errorf("%s %s %d: no audit row", tc.method, tc.path, tc.status)
			}
		}
	}
}

// The Person column must name who made a change, not the client address the
// request came from. Without a sign-in the address is all there is.
func TestAuditMiddlewareRecordsSignedInActor(t *testing.T) {
	sut := newTestAPIServer(t)
	store := persistence.NewMemoryAuthStore()
	sut.authStore = store
	sut.auditEventCh = make(chan audit.Event, 1)
	user, err := store.CreateUserWithRole("demo-admin", "unused", auth.RoleAdmin, "local", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := store.CreateAuthSession(user.ID, auth.HashToken("session-token"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	handler := sut.auditMiddleware(sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	send := func(cookie bool) audit.Event {
		req := httptest.NewRequest(http.MethodPost, "/api/v2/webhooks", nil)
		req.RemoteAddr = "192.0.2.44:40000"
		if cookie {
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "session-token"})
		}
		handler.ServeHTTP(httptest.NewRecorder(), req)
		return <-sut.auditEventCh
	}

	if got := send(true); got.ActorID != user.ID || got.Decision != "" {
		t.Fatalf("signed-in change recorded actor=%q decision=%q, want %q", got.ActorID, got.Decision, user.ID)
	}
	if got := send(false); got.ActorID != "192.0.2.44" || got.Decision != "denied" {
		t.Fatalf("refused change recorded actor=%q decision=%q, want the client address and denied", got.ActorID, got.Decision)
	}
}
