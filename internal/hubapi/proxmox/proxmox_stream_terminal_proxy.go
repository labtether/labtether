package proxmox

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/terminal"
)

func (d *Deps) TryProxmoxTerminalStream(w http.ResponseWriter, r *http.Request, session terminal.Session, target ProxmoxSessionTarget) error {
	securityruntime.Logf("terminal-proxmox: starting for session=%s kind=%s node=%s vmid=%s", session.ID, target.Kind, target.Node, target.VMID)

	runtime, err := d.LoadProxmoxRuntime(target.CollectorID)
	if err != nil {
		securityruntime.Logf("terminal-proxmox: failed to load runtime: %v", err)
		return fmt.Errorf("load runtime: %w", err)
	}

	ticket, err := d.OpenProxmoxTerminalTicket(r.Context(), runtime, target)
	if err != nil {
		securityruntime.Logf("terminal-proxmox: failed to get terminal ticket: %v", err)
		return fmt.Errorf("open terminal ticket: %w", err)
	}
	securityruntime.Logf("terminal-proxmox: ticket acquired port=%d user=%s", ticket.Port.Int(), ticket.User)

	// The vncwebsocket URL must match the path where the termproxy ticket
	// was issued. For QEMU VMs this is /nodes/{node}/qemu/{vmid}/vncwebsocket,
	// for LXC /nodes/{node}/lxc/{vmid}/vncwebsocket, for nodes /nodes/{node}/vncwebsocket.
	proxmoxConn, err := DialProxmoxProxySocket(runtime, target.Node, target.Kind, target.VMID, ticket)
	if err != nil {
		securityruntime.Logf("terminal-proxmox: failed to dial websocket: %v", err)
		return fmt.Errorf("dial websocket: %w", err)
	}
	defer proxmoxConn.Close()
	securityruntime.Logf("terminal-proxmox: websocket connected")

	// Proxmox termproxy protocol: send "user:ticket\n" as first message.
	// Use the full user from the ticket (API token format).
	authMsg := ticket.User + ":" + ticket.Ticket + "\n"
	securityruntime.Logf("terminal-proxmox: sending auth (user=%s, ticket_len=%d)", ticket.User, len(ticket.Ticket))
	if err := proxmoxCallWriteMessage(proxmoxConn, websocket.TextMessage, []byte(authMsg)); err != nil {
		securityruntime.Logf("terminal-proxmox: failed to send auth message: %v", err)
		return fmt.Errorf("send auth: %w", err)
	}

	// Read the auth response — Proxmox sends "OK" on success.
	if err := proxmoxCallSetReadDeadline(proxmoxConn, time.Now().Add(10*time.Second)); err != nil {
		return fmt.Errorf("set read deadline: %w", err)
	}
	_, authResp, err := proxmoxConn.ReadMessage()
	if err != nil {
		securityruntime.Logf("terminal-proxmox: auth read failed: %v", err)
		return fmt.Errorf("auth read: %w", err)
	}
	_ = proxmoxCallSetReadDeadline(proxmoxConn, time.Time{})

	if len(authResp) < 2 || authResp[0] != 'O' || authResp[1] != 'K' {
		securityruntime.Logf("terminal-proxmox: auth rejected (response=%q)", string(authResp))
		return fmt.Errorf("auth rejected: %q", string(authResp))
	}
	securityruntime.Logf("terminal-proxmox: auth OK, upgrading browser websocket")

	// Past this point we own the HTTP response — return nil so the caller
	// does not attempt SSH fallback.
	wsConn, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		securityruntime.Logf("terminal-proxmox: browser websocket upgrade failed: %v", err)
		return nil // response already hijacked by upgrade attempt
	}
	defer wsConn.Close()

	// Forward any data that came after the "OK" (e.g. initial prompt).
	if len(authResp) > 2 {
		_ = proxmoxCallWriteMessage(wsConn, websocket.BinaryMessage, authResp[2:])
	}

	securityruntime.Logf("terminal-proxmox: bridge started for session=%s", session.ID)
	BridgeProxmoxTerminal(wsConn, proxmoxConn)
	securityruntime.Logf("terminal-proxmox: bridge ended for session=%s", session.ID)
	return nil
}
func (d *Deps) OpenProxmoxTerminalTicket(ctx context.Context, runtime *ProxmoxRuntime, target ProxmoxSessionTarget) (proxmox.ProxyTicket, error) {
	switch target.Kind {
	case "node":
		return runtime.client.OpenNodeTermProxy(ctx, target.Node)
	case "qemu":
		return runtime.client.OpenQemuTermProxy(ctx, target.Node, target.VMID)
	case "lxc":
		return runtime.client.OpenLXCTermProxy(ctx, target.Node, target.VMID)
	default:
		return proxmox.ProxyTicket{}, fmt.Errorf("unsupported proxmox terminal kind: %s", target.Kind)
	}
}
func BridgeProxmoxTerminal(browserConn, proxmoxConn *websocket.Conn) {
	done := make(chan struct{})
	var browserWriteMu sync.Mutex
	stopKeepalive := shared.StartBrowserWebSocketKeepalive(browserConn, &browserWriteMu, "terminal-proxmox-bridge")
	defer stopKeepalive()

	// Proxmox → Browser: forward raw terminal output directly.
	go func() {
		defer close(done)
		for {
			// Rolling read deadline: if Proxmox sends nothing for 90s,
			// the connection is likely half-open; close to free goroutines.
			_ = proxmoxConn.SetReadDeadline(time.Now().Add(90 * time.Second))
			msgType, payload, err := proxmoxCallReadMessage(proxmoxConn)
			if err != nil {
				return
			}
			browserWriteMu.Lock()
			err = proxmoxCallWriteMessage(browserConn, msgType, payload)
			browserWriteMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	// Keepalive: send "2" to Proxmox every 30 seconds.
	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(proxmoxCurrentKeepaliveInterval())
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-pingDone:
				return
			case <-ticker.C:
				if err := proxmoxCallWriteMessage(proxmoxConn, websocket.BinaryMessage, []byte("2")); err != nil {
					return
				}
			}
		}
	}()
	defer close(pingDone)

	// Browser → Proxmox: translate browser messages to Proxmox framing.
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

		proxmoxMsg := TranslateBrowserToProxmoxTerm(payload)
		if proxmoxMsg == nil {
			continue
		}
		if err := proxmoxCallWriteMessage(proxmoxConn, websocket.BinaryMessage, proxmoxMsg); err != nil {
			return
		}
	}
}

// TranslateBrowserToProxmoxTerm converts a browser WebSocket message into
// Proxmox termproxy protocol format.
func TranslateBrowserToProxmoxTerm(payload []byte) []byte {
	// Check if the message is a JSON control message from our XTerminal.
	if isControlMessage(payload) {
		var control terminalControlMessage
		if err := json.Unmarshal(payload, &control); err == nil {
			switch strings.ToLower(strings.TrimSpace(control.Type)) {
			case "resize":
				if control.Cols > 0 && control.Rows > 0 {
					return []byte(fmt.Sprintf("1:%d:%d:", control.Cols, control.Rows))
				}
				return nil
			case "ping":
				return []byte("2")
			case "input":
				if control.Data == "" {
					return nil
				}
				return wrapProxmoxTermData(control.Data)
			default:
				return nil
			}
		}
	}

	// Raw terminal data (keystrokes from xterm.js onData/onBinary).
	if len(payload) == 0 {
		return nil
	}
	return wrapProxmoxTermData(string(payload))
}

// wrapProxmoxTermData wraps terminal input in Proxmox "0:LENGTH:DATA" format.
// LENGTH is the byte length of the UTF-8 encoded data.
func wrapProxmoxTermData(data string) []byte {
	byteLen := len(data)
	return []byte(fmt.Sprintf("0:%d:%s", byteLen, data))
}

// performProxmoxVNCAuth handles the RFB security handshake with the Proxmox
// VNC WebSocket, authenticating with the generated password. After this
// function returns, the connection is past the security stage and ready for
