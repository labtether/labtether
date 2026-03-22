package proxmox

import (
	"fmt"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func (d *Deps) HandleProxmoxDesktopStream(w http.ResponseWriter, r *http.Request, session terminal.Session, target ProxmoxSessionTarget) {
	if target.Kind != "qemu" && target.Kind != "lxc" {
		servicehttp.WriteError(w, http.StatusBadRequest, "desktop proxy is only supported for Proxmox VM/CT assets")
		return
	}

	securityruntime.Logf("desktop-proxmox: starting VNC proxy for session=%s kind=%s node=%s vmid=%s", session.ID, target.Kind, target.Node, target.VMID)

	runtime, err := d.LoadProxmoxRuntime(target.CollectorID)
	if err != nil {
		securityruntime.Logf("desktop-proxmox: failed to load runtime collector=%s: %v", target.CollectorID, err)
		servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
		return
	}
	securityruntime.Logf("desktop-proxmox: runtime loaded collector=%s", runtime.collectorID)

	var ticket proxmox.ProxyTicket
	if target.Kind == "qemu" {
		ticket, err = runtime.client.OpenQemuVNCProxy(r.Context(), target.Node, target.VMID)
	} else {
		ticket, err = runtime.client.OpenLXCVNCProxy(r.Context(), target.Node, target.VMID)
	}
	if err != nil {
		securityruntime.Logf("desktop-proxmox: failed to get VNC ticket kind=%s node=%s vmid=%s: %v", target.Kind, target.Node, target.VMID, err)
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to open proxmox vnc proxy: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	securityruntime.Logf("desktop-proxmox: VNC ticket acquired port=%d user=%s password=%v", ticket.Port.Int(), ticket.User, ticket.Password != "")

	proxmoxConn, err := DialProxmoxProxySocket(runtime, target.Node, target.Kind, target.VMID, ticket)
	if err != nil {
		securityruntime.Logf("desktop-proxmox: failed to dial proxmox websocket: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to connect proxmox websocket: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	defer proxmoxConn.Close()
	securityruntime.Logf("desktop-proxmox: proxmox websocket connected")

	// Handle VNC RFB auth on the server side so the browser sees a clean
	// "no auth" connection. This avoids exposing the generated VNC password
	// to the frontend and eliminates the password prompt.
	rfbVersion, err := PerformProxmoxVNCAuth(proxmoxConn, ticket.Password)
	if err != nil {
		securityruntime.Logf("desktop-proxmox: VNC auth failed: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, "VNC authentication failed: "+err.Error())
		return
	}
	securityruntime.Logf("desktop-proxmox: VNC auth succeeded (version=%s)", rfbVersion)

	wsConn, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		securityruntime.Logf("desktop-proxmox: failed to upgrade browser websocket: %v", err)
		return
	}
	defer wsConn.Close()

	// Send a "no auth required" RFB handshake to the browser.
	if err := SendBrowserVNCNoAuth(wsConn, rfbVersion); err != nil {
		securityruntime.Logf("desktop-proxmox: browser VNC handshake failed: %v", err)
		return
	}
	securityruntime.Logf("desktop-proxmox: browser VNC handshake complete, starting bridge")

	BridgeWebSocketPair(wsConn, proxmoxConn)
	securityruntime.Logf("desktop-proxmox: bridge ended for session=%s", session.ID)
}
func PerformProxmoxVNCAuth(conn *websocket.Conn, password string) (rfbVersion string, err error) {
	_ = proxmoxCallSetReadDeadline(conn, time.Now().Add(10*time.Second))
	defer func() { _ = proxmoxCallSetReadDeadline(conn, time.Time{}) }()

	// Step 1: Read server RFB version (12 bytes, e.g. "RFB 003.008\n").
	_, versionMsg, err := conn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("read RFB version: %w", err)
	}
	rfbVersion = strings.TrimRight(string(versionMsg), "\n")

	// Step 2: Echo back the same version.
	if err := proxmoxCallWriteMessage(conn, websocket.BinaryMessage, versionMsg); err != nil {
		return "", fmt.Errorf("send RFB version: %w", err)
	}

	// Step 3: Read security types.
	_, secMsg, err := conn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("read security types: %w", err)
	}
	if len(secMsg) < 1 {
		return "", fmt.Errorf("empty security message")
	}

	numTypes := int(secMsg[0])
	if numTypes == 0 {
		return "", fmt.Errorf("VNC server rejected connection (0 security types)")
	}
	types := secMsg[1:]
	if len(types) < numTypes {
		return "", fmt.Errorf("incomplete security types (expected %d, got %d)", numTypes, len(types))
	}

	// Prefer "None" (type 1). Fall back to "VNC Auth" (type 2).
	hasNone := false
	hasVNCAuth := false
	for _, t := range types[:numTypes] {
		if t == 1 {
			hasNone = true
		}
		if t == 2 {
			hasVNCAuth = true
		}
	}

	if hasNone {
		if err := proxmoxCallWriteMessage(conn, websocket.BinaryMessage, []byte{1}); err != nil {
			return "", fmt.Errorf("send None selection: %w", err)
		}
		// RFB 3.8: SecurityResult is sent even for None.
		_, result, err := conn.ReadMessage()
		if err != nil {
			return rfbVersion, fmt.Errorf("read None auth result: %w", err)
		}
		if len(result) >= 4 && (result[0] != 0 || result[1] != 0 || result[2] != 0 || result[3] != 0) {
			return rfbVersion, fmt.Errorf("none auth unexpectedly rejected")
		}
		return rfbVersion, nil
	}

	if !hasVNCAuth {
		return "", fmt.Errorf("no supported security type (got %d types: %v)", numTypes, types[:numTypes])
	}

	// Select VNC Auth (type 2).
	if err := proxmoxCallWriteMessage(conn, websocket.BinaryMessage, []byte{2}); err != nil {
		return "", fmt.Errorf("send VNC Auth selection: %w", err)
	}

	// Step 4: Read 16-byte DES challenge.
	_, challenge, err := conn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("read VNC challenge: %w", err)
	}
	if len(challenge) != 16 {
		return "", fmt.Errorf("unexpected challenge length: %d", len(challenge))
	}

	// Step 5: DES-encrypt the challenge with the password.
	response := VNCEncryptChallenge(challenge, password)

	// Step 6: Send 16-byte response.
	if err := proxmoxCallWriteMessage(conn, websocket.BinaryMessage, response); err != nil {
		return "", fmt.Errorf("send VNC auth response: %w", err)
	}

	// Step 7: Read SecurityResult (4 bytes, 0 = OK).
	_, result, err := conn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("read VNC auth result: %w", err)
	}
	if len(result) < 4 {
		return "", fmt.Errorf("short auth result: %d bytes", len(result))
	}
	if result[0] != 0 || result[1] != 0 || result[2] != 0 || result[3] != 0 {
		return "", fmt.Errorf("VNC authentication failed (bad password?)")
	}
	return rfbVersion, nil
}

// SendBrowserVNCNoAuth sends a "no authentication required" RFB handshake to
// the browser WebSocket. The browser's noVNC client sees a VNC server that
// does not require any credentials.
func SendBrowserVNCNoAuth(conn *websocket.Conn, rfbVersion string) error {
	// Send RFB version.
	if err := proxmoxCallWriteMessage(conn, websocket.BinaryMessage, []byte(rfbVersion+"\n")); err != nil {
		return fmt.Errorf("send RFB version: %w", err)
	}

	// Read browser's version response.
	_ = proxmoxCallSetReadDeadline(conn, time.Now().Add(10*time.Second))
	_, _, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read browser RFB version: %w", err)
	}
	_ = proxmoxCallSetReadDeadline(conn, time.Time{})

	// Send security types: 1 type available, type 1 (None).
	if err := proxmoxCallWriteMessage(conn, websocket.BinaryMessage, []byte{1, 1}); err != nil {
		return fmt.Errorf("send security types: %w", err)
	}

	// Read browser's selected security type.
	_ = proxmoxCallSetReadDeadline(conn, time.Now().Add(10*time.Second))
	_, _, err = conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read browser security selection: %w", err)
	}
	_ = proxmoxCallSetReadDeadline(conn, time.Time{})

	// Send SecurityResult: OK.
	if err := proxmoxCallWriteMessage(conn, websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
		return fmt.Errorf("send auth result: %w", err)
	}
	return nil
}

// VNCEncryptChallenge DES-encrypts a 16-byte VNC challenge using the password.
func VNCEncryptChallenge(challenge []byte, password string) []byte {
	key := VNCDESKey(password)
	block, err := proxmoxCallDESNewCipher(key[:])
	if err != nil {
		return make([]byte, 16)
	}
	response := make([]byte, 16)
	block.Encrypt(response[0:8], challenge[0:8])
	block.Encrypt(response[8:16], challenge[8:16])
	return response
}

// VNCDESKey converts a VNC password into a DES key. VNC reverses the bits in
// each byte before using it as the key. The key is always 8 bytes (zero-padded
// if the password is shorter, truncated if longer).
func VNCDESKey(password string) [8]byte {
	var key [8]byte
	pw := []byte(password)
	for i := 0; i < 8 && i < len(pw); i++ {
		key[i] = VNCReverseBits(pw[i])
	}
	return key
}

func VNCReverseBits(b byte) byte {
	var r byte
	for i := 0; i < 8; i++ {
		r = (r << 1) | (b & 1)
		b >>= 1
	}
	return r
}

// BridgeWebSocketPair bridges two WebSocket connections with passthrough.
// Each connection has exactly one writer (the goroutine for upstream→browser,
// and the main loop for browser→upstream), so no write mutex is needed per
// connection. Write deadlines are applied on every write to prevent blocking
// indefinitely on a slow or unresponsive peer.
func BridgeWebSocketPair(browserConn, upstreamConn *websocket.Conn) {
	done := make(chan struct{})
	var browserWriteMu sync.Mutex
	stopKeepalive := shared.StartBrowserWebSocketKeepalive(browserConn, &browserWriteMu, "desktop-proxmox-bridge")
	defer stopKeepalive()

	go func() {
		defer close(done)
		for {
			// Rolling read deadline: detect half-open upstream connections.
			_ = upstreamConn.SetReadDeadline(time.Now().Add(90 * time.Second))
			msgType, payload, err := proxmoxCallReadMessage(upstreamConn)
			if err != nil {
				return
			}
			browserWriteMu.Lock()
			_ = browserConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err = proxmoxCallWriteMessage(browserConn, msgType, payload)
			browserWriteMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
		}

		msgType, payload, err := proxmoxCallReadMessage(browserConn)
		if err != nil {
			return
		}
		_ = shared.TouchBrowserWebSocketReadDeadline(browserConn)
		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			continue
		}
		_ = upstreamConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := proxmoxCallWriteMessage(upstreamConn, msgType, payload); err != nil {
			return
		}
	}
}
