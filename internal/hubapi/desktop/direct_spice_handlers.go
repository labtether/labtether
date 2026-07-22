package desktop

import (
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

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

// HandleDirectSPICEStream bridges a browser WebSocket to a conventional
// cleartext SPICE endpoint (normally port 5930). TLS SPICE endpoints require a
// managed connector that can provide the expected CA and verification policy.
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

	spiceConn, err := securityruntime.DialOutboundTCPContext(r.Context(), opts.DirectHost, opts.DirectPort, 10*time.Second)
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
