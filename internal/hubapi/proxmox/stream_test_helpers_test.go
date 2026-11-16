package proxmox_test

import (
	"crypto/cipher"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
)

// Hooks for the Proxmox stream, VNC and proxy-dial tests in this package.
// They swap the package's exported stream variables under
// ProxmoxStreamHooksMu and restore them when the test ends.

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
