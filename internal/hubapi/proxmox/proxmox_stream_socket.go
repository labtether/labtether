package proxmox

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/securityruntime"
)

func NewProxmoxTLSConfig(skipVerify bool, caPEM string) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// #nosec G402 -- runtime-configurable for self-signed homelab certs.
		InsecureSkipVerify: skipVerify, //nolint:gosec // #nosec G402 -- runtime-configurable for self-signed homelab certs
	}
	if strings.TrimSpace(caPEM) != "" {
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM([]byte(caPEM)); !ok {
			return nil, fmt.Errorf("invalid proxmox ca_pem")
		}
		tlsConfig.RootCAs = pool
	}
	return tlsConfig, nil
}

func DialProxmoxProxySocket(runtime *ProxmoxRuntime, node, kind, vmid string, ticket proxmox.ProxyTicket) (*websocket.Conn, error) {
	if runtime == nil || runtime.client == nil {
		return nil, fmt.Errorf("proxmox runtime unavailable")
	}
	if ticket.Port.Int() <= 0 || strings.TrimSpace(ticket.Ticket) == "" {
		return nil, fmt.Errorf("invalid proxmox ticket payload")
	}

	wsURL, err := runtime.client.BuildVNCWebSocketURL(node, kind, vmid, ticket.Port.Int(), ticket.Ticket)
	if err != nil {
		return nil, err
	}
	if _, err := securityruntime.ValidateOutboundURL(wsURL); err != nil {
		return nil, err
	}

	dialer, err := NewProxmoxDialer(runtime.skipVerify, runtime.caPEM)
	if err != nil {
		return nil, err
	}

	// Authenticate the WebSocket upgrade request.
	headers := http.Header{}
	if runtime.authMode == proxmox.AuthModePassword {
		// Password mode: use session ticket cookie for WebSocket auth.
		sessionTicket, _, err := runtime.client.GetTicket(context.Background())
		if err != nil {
			return nil, fmt.Errorf("acquire session ticket for websocket: %w", err)
		}
		headers.Set("Cookie", "PVEAuthCookie="+sessionTicket)
	} else {
		headers.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", runtime.tokenID, runtime.tokenSecret))
	}

	logHost := ""
	if parsed, parseErr := neturl.Parse(wsURL); parseErr == nil {
		logHost = parsed.Host
	}
	if strings.TrimSpace(logHost) == "" {
		logHost = "unknown"
	}
	securityruntime.Logf("desktop-proxmox: dialing websocket host=%s (kind=%s, vmid=%s)", logHost, kind, vmid)
	conn, httpResp, err := dialer.Dial(wsURL, headers)
	if err != nil {
		detail := err.Error()
		if httpResp != nil {
			body := make([]byte, 1024)
			n, _ := httpResp.Body.Read(body)
			_ = httpResp.Body.Close()
			if n > 0 {
				detail = fmt.Sprintf("%s (HTTP %d: %s)", detail, httpResp.StatusCode, strings.TrimSpace(string(body[:n])))
			} else {
				detail = fmt.Sprintf("%s (HTTP %d)", detail, httpResp.StatusCode)
			}
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return conn, nil
}

func NewProxmoxDialer(skipVerify bool, caPEM string) (*websocket.Dialer, error) {
	tlsConfig, err := NewProxmoxTLSConfig(skipVerify, caPEM)
	if err != nil {
		return nil, err
	}
	return &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig:  tlsConfig,
		Subprotocols:     []string{"binary"},
	}, nil
}

// bridgeProxmoxTerminal bridges a browser WebSocket to a Proxmox termproxy
// WebSocket using the Proxmox terminal framing protocol:
//   - Client→Proxmox data:   "0:LENGTH:DATA"
//   - Client→Proxmox resize: "1:COLS:ROWS:"
//   - Client→Proxmox ping:   "2"
