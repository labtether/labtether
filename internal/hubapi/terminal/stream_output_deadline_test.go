package terminal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestTerminalOutputRefreshesExpiredStatusDeadline(t *testing.T) {
	result := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		// Model the status/ready deadline expiring while a shell is idle.
		_ = conn.SetWriteDeadline(time.Now().Add(-time.Second))
		var mu sync.Mutex
		result <- writeTerminalOutput(conn, &mu, []byte("remote output after idle\r\n"))
	}))
	defer server.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	kind, payload, err := client.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocket.BinaryMessage || string(payload) != "remote output after idle\r\n" {
		t.Fatalf("unexpected output kind=%d payload=%q", kind, payload)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
