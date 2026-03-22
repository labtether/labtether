package shared

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	serverConnCh := make(chan *websocket.Conn, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConnCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial websocket failed: %v", err)
	}

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(2 * time.Second):
		_ = clientConn.Close()
		server.Close()
		t.Fatal("timed out waiting for server websocket connection")
	}

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		server.Close()
	}

	return serverConn, clientConn, cleanup
}

func TestStartBrowserWebSocketKeepaliveSendsPingAndStopsCleanly(t *testing.T) {
	serverConn, clientConn, cleanup := newWebSocketPair(t)
	defer cleanup()

	originalPingInterval := StreamBrowserPingInterval
	originalReadDeadline := StreamBrowserReadDeadline
	originalWriteTimeout := StreamBrowserPingWriteTimeout
	StreamBrowserPingInterval = 20 * time.Millisecond
	StreamBrowserReadDeadline = 2 * time.Second
	StreamBrowserPingWriteTimeout = 250 * time.Millisecond
	defer func() {
		StreamBrowserPingInterval = originalPingInterval
		StreamBrowserReadDeadline = originalReadDeadline
		StreamBrowserPingWriteTimeout = originalWriteTimeout
	}()

	var pingCount int32
	pingSeen := make(chan struct{}, 1)
	clientConn.SetPingHandler(func(appData string) error {
		if atomic.AddInt32(&pingCount, 1) == 1 {
			select {
			case pingSeen <- struct{}{}:
			default:
			}
		}
		return clientConn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(250*time.Millisecond))
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_ = clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			if _, _, err := clientConn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	var writeMu sync.Mutex
	stop := StartBrowserWebSocketKeepalive(serverConn, &writeMu, "test-keepalive")
	defer stop()

	select {
	case <-pingSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("expected keepalive ping to reach client")
	}

	if atomic.LoadInt32(&pingCount) == 0 {
		t.Fatal("expected ping handler to be invoked at least once")
	}

	stop()
	stop()

	_ = clientConn.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("client read loop did not stop after close")
	}
}

func TestTouchBrowserWebSocketReadDeadlineExtendsDeadline(t *testing.T) {
	serverConn, clientConn, cleanup := newWebSocketPair(t)
	defer cleanup()

	originalReadDeadline := StreamBrowserReadDeadline
	StreamBrowserReadDeadline = 80 * time.Millisecond
	defer func() {
		StreamBrowserReadDeadline = originalReadDeadline
	}()

	if err := TouchBrowserWebSocketReadDeadline(serverConn); err != nil {
		t.Fatalf("set initial read deadline: %v", err)
	}

	// Refresh before the first deadline expires.
	time.Sleep(45 * time.Millisecond)
	if err := TouchBrowserWebSocketReadDeadline(serverConn); err != nil {
		t.Fatalf("refresh read deadline: %v", err)
	}

	// Past the original deadline, still before the refreshed deadline.
	time.Sleep(45 * time.Millisecond)

	if err := clientConn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("write client message: %v", err)
	}

	_, payload, err := serverConn.ReadMessage()
	if err != nil {
		t.Fatalf("read server message after refresh: %v", err)
	}
	if string(payload) != "ping" {
		t.Fatalf("unexpected payload %q", string(payload))
	}
}
