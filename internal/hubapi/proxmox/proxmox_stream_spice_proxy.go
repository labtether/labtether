package proxmox

import (
	"crypto/tls"
	"fmt"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"log"
	"net"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

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

	ticket, err := runtime.client.OpenQemuSPICEProxy(r.Context(), target.Node, target.VMID)
	if err != nil {
		status, message := ProxmoxSPICEOpenErrorResponse(err)
		servicehttp.WriteError(w, status, message)
		return
	}
	trimmedHost := strings.TrimSpace(ticket.Host)
	effectiveCA := strings.TrimSpace(ticket.CA)
	if effectiveCA == "" {
		effectiveCA = strings.TrimSpace(runtime.caPEM)
	}
	if trimmedHost == "" || ticket.TLSPort <= 0 || ticket.TLSPort > 65535 {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox SPICE target unavailable")
		return
	}

	streamTicket, expiresAt, err := d.IssueStreamTicket(r.Context(), session.ID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to issue stream ticket")
		return
	}

	d.SetDesktopSPICEProxyTarget(session.ID, DesktopSPICEProxyTarget{
		Host:       trimmedHost,
		TLSPort:    ticket.TLSPort,
		Password:   ticket.Password,
		Type:       ticket.Type,
		CA:         effectiveCA,
		Proxy:      ticket.Proxy,
		SkipVerify: runtime.skipVerify,
	})

	streamPath := fmt.Sprintf(
		"/desktop/sessions/%s/stream?ticket=%s&protocol=spice",
		neturl.PathEscape(session.ID),
		neturl.QueryEscape(streamTicket),
	)

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
		"session_id":  session.ID,
		"ticket":      streamTicket,
		"expires_at":  expiresAt,
		"stream_path": streamPath,
		"password":    ticket.Password,
		"type":        ticket.Type,
		"ca":          effectiveCA,
		"proxy":       ticket.Proxy,
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
	if host == "" || port <= 0 || port > 65535 {
		servicehttp.WriteError(w, http.StatusBadGateway, "invalid SPICE target")
		return
	}

	validatedHost, validatedPort, validateErr := securityruntime.ValidateOutboundHostPort(host, strconv.Itoa(port), port)
	if validateErr != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid SPICE target: "+shared.SanitizeUpstreamError(validateErr.Error()))
		return
	}

	addr := net.JoinHostPort(validatedHost, strconv.Itoa(validatedPort))
	tlsConfig, err := NewProxmoxTLSConfig(spiceTarget.SkipVerify, spiceTarget.CA)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to prepare SPICE TLS: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	if net.ParseIP(validatedHost) == nil {
		tlsConfig.ServerName = validatedHost
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	spiceConn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to connect to SPICE: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	defer spiceConn.Close()

	wsConn, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer wsConn.Close()
	wsConn.SetReadLimit(d.MaxDesktopInputReadBytes)

	log.Printf("desktop: proxied SPICE stream for %s -> %s", session.ID, addr)

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
