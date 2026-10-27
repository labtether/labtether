package shared

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/apiv2"
)

// Logging out, deleting or demoting a user, or revoking a key must end the
// root terminals and remote views opened with that credential, not leave them
// running until the client closes them.
func TestRevokePrincipalsClosesOnlyMatchingStreams(t *testing.T) {
	var registry SessionStreamRegistry
	open := func(userID, authSessionID string) *http.Request {
		ctx := ContextWithAuthSessionID(apiv2.ContextWithPrincipal(context.Background(), userID, "operator"), authSessionID)
		req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/s/stream", nil).WithContext(ctx)
		_, opened, cleanup, err := registry.Open(httptest.NewRecorder(), req, "term-"+userID, func() (bool, error) { return true, nil })
		if err != nil {
			t.Fatalf("open stream: %v", err)
		}
		t.Cleanup(cleanup)
		return opened
	}
	alice := open("usr-alice", "sess-alice")
	bob := open("usr-bob", "sess-bob")

	if closed := registry.RevokePrincipals(StreamSessionKey("sess-alice")); closed != 1 {
		t.Fatalf("revoked %d streams, want 1", closed)
	}
	if alice.Context().Err() == nil {
		t.Fatal("logged-out session's stream is still open")
	}
	if bob.Context().Err() != nil {
		t.Fatal("another user's stream was closed")
	}
	registry.RevokePrincipals(StreamActorKey("usr-bob"))
	if bob.Context().Err() == nil {
		t.Fatal("deleted user's stream is still open")
	}
}

// /api/v2/events/stream must accept the API key its route already checked,
// and revoking that key must end the socket.
func TestServeAuthenticatedBrowserEventsAcceptsKeyAndEndsOnRevoke(t *testing.T) {
	broadcaster := NewEventBroadcaster()
	deps := &BrowserEventsDeps{Broadcaster: broadcaster, CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := apiv2.ContextWithAPIKeyID(apiv2.ContextWithPrincipal(r.Context(), "apikey:key-dash", "viewer"), "key-dash")
		if r.URL.Query().Get("restricted") == "1" {
			ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"srv1"})
		}
		deps.ServeAuthenticatedBrowserEvents(w, r.WithContext(ctx))
	}))
	defer server.Close()

	if _, resp, err := websocket.DefaultDialer.Dial(browserEventsWebSocketURL(server.URL, "/?restricted=1"), nil); err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("asset-restricted key: err=%v resp=%v, want 403", err, resp)
	}

	conn, _, err := websocket.DefaultDialer.Dial(browserEventsWebSocketURL(server.URL, "/"), nil)
	if err != nil {
		t.Fatalf("API key subscription failed: %v", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for broadcaster.Count() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := broadcaster.DisconnectPrincipals(StreamActorKey("apikey:key-dash")); got != 1 {
		t.Fatalf("disconnected %d sockets, want 1", got)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("event socket stayed open after its key was revoked")
	}
}
