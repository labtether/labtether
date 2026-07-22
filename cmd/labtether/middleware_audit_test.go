package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
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
