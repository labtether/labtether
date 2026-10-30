package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/audit"
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
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
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
