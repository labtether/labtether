package proxmox_test

import (
	"crypto/cipher"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
)

// Test helpers for the Proxmox stream, VNC and proxy-dial tests in this
// package. cmd/labtether keeps its own copies of allowInsecureTransport,
// withProxmoxStreamHooks and newWebSocketPair for the tests that stay there.

func allowInsecureTransportForConnectorTests(t *testing.T) {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
}

func withProxmoxStreamHooks(
	t *testing.T,
	write func(*websocket.Conn, int, []byte) error,
	read func(*websocket.Conn) (int, []byte, error),
	setReadDeadline func(*websocket.Conn, time.Time) error,
	newCipher func([]byte) (cipher.Block, error),
) {
	t.Helper()

	proxmoxpkg.ProxmoxStreamHooksMu.Lock()
	originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
	originalRead := proxmoxpkg.ProxmoxWSReadMessage
	originalSetReadDeadline := proxmoxpkg.ProxmoxWSSetReadDeadline
	originalNewCipher := proxmoxpkg.ProxmoxDESNewCipher

	if write != nil {
		proxmoxpkg.ProxmoxWSWriteMessage = write
	}
	if read != nil {
		proxmoxpkg.ProxmoxWSReadMessage = read
	}
	if setReadDeadline != nil {
		proxmoxpkg.ProxmoxWSSetReadDeadline = setReadDeadline
	}
	if newCipher != nil {
		proxmoxpkg.ProxmoxDESNewCipher = newCipher
	}
	proxmoxpkg.ProxmoxStreamHooksMu.Unlock()

	t.Cleanup(func() {
		proxmoxpkg.ProxmoxStreamHooksMu.Lock()
		proxmoxpkg.ProxmoxWSWriteMessage = originalWrite
		proxmoxpkg.ProxmoxWSReadMessage = originalRead
		proxmoxpkg.ProxmoxWSSetReadDeadline = originalSetReadDeadline
		proxmoxpkg.ProxmoxDESNewCipher = originalNewCipher
		proxmoxpkg.ProxmoxStreamHooksMu.Unlock()
	})
}

func setProxmoxTerminalKeepaliveIntervalForTest(t *testing.T, interval time.Duration) {
	t.Helper()

	proxmoxpkg.ProxmoxStreamHooksMu.Lock()
	original := proxmoxpkg.ProxmoxTerminalKeepaliveInterval
	proxmoxpkg.ProxmoxTerminalKeepaliveInterval = interval
	proxmoxpkg.ProxmoxStreamHooksMu.Unlock()

	t.Cleanup(func() {
		proxmoxpkg.ProxmoxStreamHooksMu.Lock()
		proxmoxpkg.ProxmoxTerminalKeepaliveInterval = original
		proxmoxpkg.ProxmoxStreamHooksMu.Unlock()
	})
}

func newWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	serverConnCh := make(chan *websocket.Conn, 1)
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("websocket upgrade failed: %v", err)
		}
		serverConnCh <- conn
		<-done
		_ = conn.Close()
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		close(done)
		server.Close()
		t.Fatalf("websocket dial failed: %v", err)
	}

	serverConn := <-serverConnCh
	cleanup := func() {
		_ = clientConn.Close()
		close(done)
		server.Close()
	}
	return serverConn, clientConn, cleanup
}
