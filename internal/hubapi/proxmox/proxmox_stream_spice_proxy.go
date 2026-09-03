package proxmox

import (
	"crypto/tls"
	"fmt"
	"log"
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

func ProxmoxSPICEOpenErrorResponse(err error) (status int, message string) {
	if err == nil {
		return http.StatusBadGateway, "failed to open SPICE proxy"
	}

	normalized := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(normalized, "no spice port"):
		return http.StatusConflict, "proxmox VM is not configured for SPICE; enable a SPICE display adapter in Proxmox or use VNC"
	case strings.Contains(normalized, "not running"):
		return http.StatusConflict, "proxmox VM must be running before a SPICE session can start"
	default:
		return http.StatusBadGateway, "failed to open SPICE proxy: " + shared.SanitizeUpstreamError(err.Error())
	}
}

func (d *Deps) HandleDesktopSPICETicket(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.EnforceRateLimit(w, r, "desktop.spice_ticket.create", 240, time.Minute) {
		return
	}

	target, ok, err := d.ResolveProxmoxSessionTarget(session.Target)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusBadRequest, "SPICE is only available for proxmox assets")
		return
	}
	if target.Kind != "qemu" {
		servicehttp.WriteError(w, http.StatusBadRequest, "SPICE is only available for Proxmox QEMU VMs")
		return
	}

	runtime, err := d.LoadProxmoxRuntime(target.CollectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox runtime unavailable")
		return
	}
	if err := validateProxmoxSPICEVerificationPolicy(runtime.skipVerify, runtime.spiceSkipVerify); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if runtime.spiceSkipVerify && !securityruntime.InsecureTransportAllowed() {
		servicehttp.WriteError(w, http.StatusBadRequest, "Proxmox SPICE certificate checks can only be disabled when LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
		return
	}

	ticket, err := runtime.client.OpenQemuSPICEProxy(r.Context(), target.Node, target.VMID)
	if err != nil {
		status, message := ProxmoxSPICEOpenErrorResponse(err)
		servicehttp.WriteError(w, status, message)
		return
	}
	trimmedHost := strings.TrimSpace(ticket.Host)
	trimmedProxy := strings.TrimSpace(ticket.Proxy)
	hostSubject := strings.TrimSpace(ticket.HostSubject)
	effectiveCA := strings.TrimSpace(ticket.CA)
	if effectiveCA == "" {
		effectiveCA = strings.TrimSpace(runtime.caPEM)
	}
	if _, err := validateProxmoxSPICEProxyTicket(trimmedHost, ticket.TLSPort); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox SPICE target unavailable")
		return
	}
	if _, _, err := parseProxmoxSPICEProxyEndpoint(trimmedProxy); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox SPICE proxy unavailable")
		return
	}
	if _, err := NewProxmoxSPICETLSConfig(runtime.spiceSkipVerify, effectiveCA, hostSubject); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox SPICE certificate data unavailable")
		return
	}

	streamTicket, expiresAt, err := d.IssueStreamTicket(r.Context(), session.ID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to issue stream ticket")
		return
	}

	d.SetDesktopSPICEProxyTarget(session.ID, DesktopSPICEProxyTarget{
		Host:        trimmedHost,
		TLSPort:     ticket.TLSPort,
		Password:    ticket.Password,
		Type:        ticket.Type,
		CA:          effectiveCA,
		Proxy:       trimmedProxy,
		HostSubject: hostSubject,
		SkipVerify:  runtime.spiceSkipVerify,
	})

	streamPath := fmt.Sprintf(
		"/desktop/sessions/%s/stream?ticket=%s&protocol=spice",
		neturl.PathEscape(session.ID),
		neturl.QueryEscape(streamTicket),
	)

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
		"session_id":   session.ID,
		"ticket":       streamTicket,
		"expires_at":   expiresAt,
		"stream_path":  streamPath,
		"password":     ticket.Password,
		"type":         ticket.Type,
		"ca":           effectiveCA,
		"proxy":        trimmedProxy,
		"host-subject": hostSubject,
	})
}

func (d *Deps) HandleDesktopSPICEStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	spiceTarget, ok := d.TakeDesktopSPICEProxyTarget(session.ID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadRequest, "SPICE ticket required before stream")
		return
	}

	host := strings.TrimSpace(spiceTarget.Host)
	port := spiceTarget.TLSPort
	if spiceTarget.SkipVerify && !securityruntime.InsecureTransportAllowed() {
		servicehttp.WriteError(w, http.StatusBadRequest, "Proxmox SPICE certificate checks are disabled")
		return
	}
	if _, err := validateProxmoxSPICEProxyTicket(host, port); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "invalid SPICE target")
		return
	}

	proxyHost, proxyPort, err := parseProxmoxSPICEProxyEndpoint(spiceTarget.Proxy)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid SPICE proxy")
		return
	}

	tlsConfig, err := NewProxmoxSPICETLSConfig(spiceTarget.SkipVerify, spiceTarget.CA, spiceTarget.HostSubject)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to prepare SPICE TLS")
		return
	}
	rawConn, err := dialProxmoxSPICEProxyTunnel(r.Context(), spiceTarget.Proxy, host, port)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to connect to SPICE proxy: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	spiceConn := tls.Client(rawConn, tlsConfig)
	if err := spiceConn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		_ = rawConn.Close()
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to prepare SPICE connection")
		return
	}
	if err := spiceConn.HandshakeContext(r.Context()); err != nil {
		_ = spiceConn.Close()
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to connect to SPICE: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	_ = spiceConn.SetDeadline(time.Time{})
	defer spiceConn.Close()

	wsConn, err := shared.UpgradeWebSocket(d.TerminalWebSocketUpgrader, w, r, nil)
	if err != nil {
		return
	}
	wsConn.SetReadLimit(d.MaxDesktopInputReadBytes)
	defer wsConn.Close()

	log.Printf("desktop: proxied SPICE stream for %s via %s:%d", session.ID, proxyHost, proxyPort)

	// Bidirectional bridge: browser WS ↔ SPICE TCP.
	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }

	// Guard all websocket writes; gorilla/websocket only allows one concurrent writer.
	var writeMu sync.Mutex
	stopKeepalive := shared.StartBrowserWebSocketKeepalive(wsConn, &writeMu, "desktop-spice:"+session.ID)
	defer stopKeepalive()

	// SPICE → Browser
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

	// Browser → SPICE
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

	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("desktop: SPICE browser input panic for %s: %v", session.ID, recovered)
		}
	}()

	for {
		messageType, payload, readErr := wsConn.ReadMessage()
		if readErr != nil {
			return
		}
		_ = shared.TouchBrowserWebSocketReadDeadline(wsConn)
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		if _, writeErr := spiceConn.Write(payload); writeErr != nil {
			return
		}
	}
}
