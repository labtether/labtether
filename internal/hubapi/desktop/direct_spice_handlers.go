package desktop

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

const (
	SPICESecurityTLS       = "tls"
	SPICESecurityCleartext = "cleartext"
	maxSPICECAPEMBytes     = 16 * 1024
)

// ValidateDirectSPICESecurityOptions applies safe defaults before a direct
// SPICE session is created. Cleartext requires a per-session choice and the
// process-wide unsafe transport gate.
func ValidateDirectSPICESecurityOptions(protocol, rawMode, rawCAPEM string) (string, string, error) {
	mode := strings.ToLower(strings.TrimSpace(rawMode))
	caPEM := strings.TrimSpace(rawCAPEM)
	if NormalizeDesktopProtocol(protocol) != "spice" {
		if mode != "" || caPEM != "" {
			return "", "", errors.New("SPICE security options are only valid for SPICE")
		}
		return "", "", nil
	}
	if mode == "" {
		mode = SPICESecurityTLS
	}
	if mode != SPICESecurityTLS && mode != SPICESecurityCleartext {
		return "", "", errors.New("spice_security_mode must be tls or cleartext")
	}
	if len(caPEM) > maxSPICECAPEMBytes {
		return "", "", fmt.Errorf("spice_ca_pem too long (max %d bytes)", maxSPICECAPEMBytes)
	}
	if mode == SPICESecurityCleartext {
		if caPEM != "" {
			return "", "", errors.New("spice_ca_pem cannot be used with cleartext SPICE")
		}
		if !securityruntime.InsecureTransportAllowed() {
			return "", "", errors.New("cleartext SPICE requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
		}
		return mode, "", nil
	}
	if _, err := NewDirectSPICETLSConfig("localhost", caPEM); err != nil {
		return "", "", err
	}
	return mode, caPEM, nil
}

// NewDirectSPICETLSConfig builds a verified TLS client configuration. The
// target host is always used as ServerName, including IP literals, so IP SANs
// are checked instead of silently skipping peer identity verification.
func NewDirectSPICETLSConfig(host, caPEM string) (*tls.Config, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, errors.New("SPICE TLS host is required")
	}
	var roots *x509.CertPool
	if strings.TrimSpace(caPEM) != "" {
		var err error
		roots, err = x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if ok := roots.AppendCertsFromPEM([]byte(caPEM)); !ok {
			return nil, errors.New("invalid spice_ca_pem certificate bundle")
		}
	}
	return &tls.Config{ // #nosec G402 -- certificate and host verification remain enabled.
		MinVersion: tls.VersionTLS12,
		ServerName: host,
		RootCAs:    roots,
	}, nil
}

func connectDirectSPICE(r *http.Request, opts DesktopSessionOptions) (net.Conn, error) {
	mode := strings.ToLower(strings.TrimSpace(opts.SPICESecurityMode))
	if mode == "" {
		mode = SPICESecurityTLS
	}
	if mode == SPICESecurityCleartext {
		if !securityruntime.InsecureTransportAllowed() {
			return nil, errors.New("cleartext SPICE is disabled")
		}
		return securityruntime.DialOutboundTCPContext(r.Context(), opts.DirectHost, opts.DirectPort, 10*time.Second)
	}
	if mode != SPICESecurityTLS {
		return nil, errors.New("invalid SPICE security mode")
	}
	tlsConfig, err := NewDirectSPICETLSConfig(opts.DirectHost, opts.SPICECAPEM)
	if err != nil {
		return nil, err
	}
	rawConn, err := securityruntime.DialOutboundTCPContext(r.Context(), opts.DirectHost, opts.DirectPort, 10*time.Second)
	if err != nil {
		return nil, err
	}
	return secureDirectSPICEConnection(r.Context(), rawConn, tlsConfig)
}

func secureDirectSPICEConnection(ctx context.Context, rawConn net.Conn, tlsConfig *tls.Config) (net.Conn, error) {
	if rawConn == nil {
		return nil, errors.New("SPICE connection is required")
	}
	if tlsConfig == nil {
		_ = rawConn.Close()
		return nil, errors.New("SPICE TLS configuration is required")
	}
	tlsConn := tls.Client(rawConn, tlsConfig)
	if err := rawConn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("SPICE TLS handshake failed: %w", err)
	}
	if err := rawConn.SetDeadline(time.Time{}); err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	return tlsConn, nil
}

// HandleDirectSPICETicket issues the browser stream ticket for an ad-hoc
// SPICE target. Unlike Proxmox SPICE, the password was supplied by the caller
// and remains only in the session's in-memory options.
func (d *Deps) HandleDirectSPICETicket(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.EnforceRateLimit(w, r, "desktop.spice_ticket.create", 240, time.Minute) {
		return
	}
	opts := d.GetDesktopSessionOptions(session.ID)
	if !opts.Direct || NormalizeDesktopProtocol(opts.Protocol) != "spice" {
		servicehttp.WriteError(w, http.StatusBadRequest, "direct SPICE session required")
		return
	}
	if _, _, err := securityruntime.ValidateOutboundEndpoint(opts.DirectHost, opts.DirectPort); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid direct SPICE target")
		return
	}
	if strings.EqualFold(strings.TrimSpace(opts.SPICESecurityMode), SPICESecurityCleartext) && !securityruntime.InsecureTransportAllowed() {
		servicehttp.WriteError(w, http.StatusBadRequest, "cleartext SPICE is disabled")
		return
	}

	ticket, expiresAt, err := d.IssueStreamTicket(r.Context(), session.ID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to issue stream ticket")
		return
	}
	streamPath := fmt.Sprintf(
		"/desktop/sessions/%s/stream?ticket=%s&protocol=spice",
		neturl.PathEscape(session.ID),
		neturl.QueryEscape(ticket),
	)
	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
		"session_id":  session.ID,
		"ticket":      ticket,
		"expires_at":  expiresAt,
		"stream_path": streamPath,
		"password":    opts.DirectPassword,
		"type":        "spice",
	})
}

// HandleDirectSPICEStream bridges a browser WebSocket to a conventional SPICE
// endpoint. Direct sessions use verified TLS unless cleartext was explicitly
// selected and the process-wide unsafe transport gate remains enabled.
func (d *Deps) HandleDirectSPICEStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	opts := d.GetDesktopSessionOptions(session.ID)
	if !opts.Direct || NormalizeDesktopProtocol(opts.Protocol) != "spice" {
		servicehttp.WriteError(w, http.StatusBadRequest, "direct SPICE session required")
		return
	}

	if _, _, err := securityruntime.ValidateOutboundEndpoint(opts.DirectHost, opts.DirectPort); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid direct SPICE target")
		return
	}
	spiceConn, err := connectDirectSPICE(r, opts)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to connect to SPICE: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	defer spiceConn.Close()

	wsConn, err := shared.UpgradeWebSocket(d.TerminalWebSocketUpgrader, w, r, nil)
	if err != nil {
		return
	}
	shared.LimitBrowserInteractiveMessages(wsConn)
	defer wsConn.Close()

	log.Printf("desktop: direct SPICE proxy for %s -> %s", session.ID, spiceConn.RemoteAddr())
	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }
	var writeMu sync.Mutex
	stopKeepalive := d.StartBrowserWSKeepalive(wsConn, &writeMu, "desktop-spice:"+session.ID)
	defer stopKeepalive()

	go func() {
		defer closeDone()
		buf := make([]byte, 16384)
		for {
			n, readErr := spiceConn.Read(buf)
			if n > 0 {
				writeMu.Lock()
				_ = wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				writeErr := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n])
				writeMu.Unlock()
				if writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	stopCloser := make(chan struct{})
	defer close(stopCloser)
	go func() {
		select {
		case <-done:
			_ = wsConn.SetReadDeadline(time.Now())
			_ = wsConn.Close()
		case <-stopCloser:
		}
	}()

	for {
		messageType, payload, readErr := wsConn.ReadMessage()
		if readErr != nil {
			return
		}
		_ = d.TouchBrowserWSReadDeadline(wsConn)
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		if _, writeErr := spiceConn.Write(payload); writeErr != nil {
			return
		}
	}
}
