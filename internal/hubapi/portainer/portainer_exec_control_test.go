package portainer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The console's terminal sends {"type":"resize",…} as a text frame. The bridge
// passed it to Portainer's exec WebSocket, so the shell got it as typed text.
// It must become a resize, and only keystrokes may reach the shell.
func TestBridgePortainerExecResizesInsteadOfTypingControlMessages(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	typed := make(chan string, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			typed <- string(payload)
		}
	}))
	defer upstream.Close()
	upstreamConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(upstream.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial upstream: %v", err)
	}

	var mu sync.Mutex
	var sizes [][2]int
	deps := &Deps{
		StartBrowserWebSocketKeepalive:    func(*websocket.Conn, *sync.Mutex, string) func() { return func() {} },
		TouchBrowserWebSocketReadDeadline: func(*websocket.Conn) error { return nil },
	}
	browserServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		deps.bridgePortainerExec(conn, upstreamConn, func(cols, rows int) {
			mu.Lock()
			sizes = append(sizes, [2]int{cols, rows})
			mu.Unlock()
		})
	}))
	defer browserServer.Close()
	browser, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(browserServer.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial browser: %v", err)
	}
	defer browser.Close()

	_ = browser.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":132,"rows":41}`))
	_ = browser.WriteMessage(websocket.TextMessage, []byte("ls\r"))
	select {
	case got := <-typed:
		if got != "ls\r" {
			t.Fatalf("shell received %q first, want only the keystrokes", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("keystrokes did not reach the shell")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sizes) != 1 || sizes[0] != [2]int{132, 41} {
		t.Fatalf("resizes = %v, want one 132x41", sizes)
	}
}
