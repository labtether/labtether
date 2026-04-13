package desktop

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func (d *Deps) desktopDirectProxyEnabled() bool {
	return d.EnvOrDefaultBool("LABTETHER_DESKTOP_DIRECT_PROXY_ENABLED", false)
}

// ShouldUseWebRTC checks if an asset's agent supports WebRTC.
func (d *Deps) ShouldUseWebRTC(assetID string) bool {
	if d == nil || d.AgentMgr == nil {
		return false
	}
	conn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(conn.Meta("webrtc_available")), "true")
}

// CanFallbackWebRTCToVNC checks if a WebRTC-unavailable asset can fall back to VNC.
func (d *Deps) CanFallbackWebRTCToVNC(assetID string) bool {
	if d == nil || d.AgentMgr == nil {
		return true
	}
	conn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return true
	}
	sessionType := strings.TrimSpace(strings.ToLower(conn.Meta("desktop_session_type")))
	if sessionType != "wayland" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(conn.Meta("desktop_vnc_real_desktop_supported")), "true")
}

// HandleDesktopStream routes a desktop WebSocket connection through an agent or direct VNC proxy.
func (d *Deps) HandleDesktopStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	protocol := d.ResolveDesktopProtocol(session, r)
	if protocol == "webrtc" {
		if d.ShouldUseWebRTC(session.Target) {
			d.HandleWebRTCStream(w, r, session)
			return
		}
		reason := "agent does not support webrtc"
		if d.AgentMgr != nil {
			if conn, ok := d.AgentMgr.Get(session.Target); ok {
				if metaReason := strings.TrimSpace(conn.Meta("webrtc_unavailable_reason")); metaReason != "" {
					reason = metaReason
				}
			}
		}
		if !d.CanFallbackWebRTCToVNC(session.Target) {
			servicehttp.WriteError(w, http.StatusBadGateway, "webrtc unavailable for current desktop backend: "+d.SanitizeUpstreamError(reason))
			return
		}
		log.Printf("desktop: webrtc requested for %s but unavailable (%s), falling back to vnc", session.ID, reason) // #nosec G706 -- Session IDs and fallback reasons are bounded runtime values.
		opts := d.GetDesktopSessionOptions(session.ID)
		opts.FallbackReason = reason
		d.SetDesktopSessionOptions(session.ID, opts)
		protocol = "vnc"
	}
	if protocol == "rdp" {
		d.HandleGuacdDesktopStream(w, r, session)
		return
	}
	if protocol == "spice" {
		d.HandleDesktopSPICEStream(w, r, session)
		return
	}

	// Priority 1: Route through connected agent.
	if d.AgentMgr != nil && d.AgentMgr.IsConnected(session.Target) {
		d.HandleAgentDesktopStream(w, r, session)
		return
	}

	// Priority 2: Proxmox API websocket bridge for Proxmox-backed assets.
	if proxmoxTarget, ok, err := d.ResolveProxmoxSessionTarget(session.Target); err == nil && ok {
		d.HandleProxmoxDesktopStream(w, r, session, proxmoxTarget)
		return
	} else if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox desktop stream unavailable: "+d.SanitizeUpstreamError(err.Error()))
		return
	}

	// Priority 3: Manual device protocol routing (VNC/ARD/RDP via asset_protocol_configs).
	if d.GetProtocolConfig != nil {
		assetSource := ""
		if d.AssetStore != nil {
			if assetEntry, ok, err := d.AssetStore.GetAsset(session.Target); err == nil && ok {
				assetSource = strings.ToLower(strings.TrimSpace(assetEntry.Source))
			}
		}
		if assetSource == "manual" {
			// Check VNC protocol config.
			vncCfg, vncErr := d.GetProtocolConfig(r.Context(), session.Target, protocols.ProtocolVNC)
			if vncErr != nil {
				log.Printf("desktop: vnc protocol config lookup failed session=%s target=%s err=%v", session.ID, session.Target, vncErr)
			}
			if vncCfg == nil || vncErr != nil {
				// Also check ARD as a VNC-compatible protocol.
				vncCfg, vncErr = d.GetProtocolConfig(r.Context(), session.Target, protocols.ProtocolARD)
				if vncErr != nil {
					log.Printf("desktop: ard protocol config lookup failed session=%s target=%s err=%v", session.ID, session.Target, vncErr)
					vncCfg = nil
				}
			}
			if vncCfg != nil && vncCfg.Enabled {
				host := strings.TrimSpace(vncCfg.Host)
				if host == "" && d.AssetStore != nil {
					if assetEntry, ok, err := d.AssetStore.GetAsset(session.Target); err == nil && ok {
						host = strings.TrimSpace(assetEntry.Host)
					}
				}
				if host == "" {
					host = session.Target
				}
				port := vncCfg.Port
				if port <= 0 {
					port = protocols.DefaultPort(protocols.ProtocolVNC)
				}
				// Manual device VNC/ARD: always use direct proxy, bypassing the env gate.
				// The host was already validated at creation time via ValidateManualDeviceHost
				// which permits LAN/private IPs — skip the outbound security validation.
				d.handleDirectVNCProxyWithConfig(w, r, session, host, port, true)
				return
			}

			// Check RDP protocol config.
			rdpCfg, rdpErr := d.GetProtocolConfig(r.Context(), session.Target, protocols.ProtocolRDP)
			if rdpErr != nil {
				log.Printf("desktop: rdp protocol config lookup failed session=%s target=%s err=%v", session.ID, session.Target, rdpErr)
			}
			if rdpCfg != nil && rdpCfg.Enabled && rdpErr == nil {
				// Route through Guacamole for RDP.
				d.HandleGuacdDesktopStream(w, r, session)
				return
			}
		}
	}

	// Priority 4: Direct VNC proxy (agentless fallback, env-gated).
	if !d.desktopDirectProxyEnabled() {
		servicehttp.WriteError(w, http.StatusBadGateway, "direct VNC proxy is disabled")
		return
	}
	d.handleDirectVNCProxy(w, r, session)
}

// ResolveDesktopProtocol determines the effective desktop protocol for a session.
func (d *Deps) ResolveDesktopProtocol(session terminal.Session, r *http.Request) string {
	queryProtocol := NormalizeDesktopProtocol(r.URL.Query().Get("protocol"))
	if queryProtocol != "" && queryProtocol != "vnc" {
		return queryProtocol
	}
	opts := d.GetDesktopSessionOptions(session.ID)
	if opts.Protocol != "" {
		return NormalizeDesktopProtocol(opts.Protocol)
	}
	if d.AssetStore != nil {
		if assetEntry, ok, err := d.AssetStore.GetAsset(session.Target); err == nil && ok {
			if strings.EqualFold(strings.TrimSpace(assetEntry.Platform), "windows") {
				return "rdp"
			}
		}
	}
	return "vnc"
}

// handleDirectVNCProxy proxies directly to a node's VNC port (agentless fallback).
// It resolves host/port from asset_protocol_configs (VNC), falling back to
// asset name as the host if no config exists.
func (d *Deps) handleDirectVNCProxy(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	host := strings.TrimSpace(session.Target)
	port := 5900

	if d.GetProtocolConfig != nil {
		pc, err := d.GetProtocolConfig(r.Context(), session.Target, protocols.ProtocolVNC)
		if err == nil && pc != nil {
			if pc.Port > 0 && pc.Port <= 65535 {
				port = pc.Port
			}
			if h := strings.TrimSpace(pc.Host); h != "" {
				host = h
			}
		}
	}
	d.handleDirectVNCProxyWithConfig(w, r, session, host, port, false)
}

// handleDirectVNCProxyWithConfig dials host:port directly and bridges it to
// the browser WebSocket. It is used by both the legacy agentless fallback path
// and the manual device protocol config path (where the env gate is bypassed).
//
// skipOutboundValidation must be true only for manual device paths where the
// host follows the manual-device policy, including DNS results resolved at
// connect time. The generic agentless fallback path
// (skipOutboundValidation=false) enforces the full securityruntime outbound
// policy.
func (d *Deps) handleDirectVNCProxyWithConfig(w http.ResponseWriter, r *http.Request, session terminal.Session, host string, port int, skipOutboundValidation bool) {
	var addr string
	if skipOutboundValidation {
		host = strings.TrimSpace(host)
		if err := protocols.ValidateManualDeviceHost(host); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid desktop target: "+d.SanitizeUpstreamError(err.Error()))
			return
		}
		if port <= 0 || port > 65535 {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid desktop port")
			return
		}
		addr = net.JoinHostPort(host, strconv.Itoa(port))
	} else {
		validatedHost, validatedPort, validateErr := securityruntime.ValidateOutboundHostPort(host, strconv.Itoa(port), 5900)
		if validateErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid desktop target: "+d.SanitizeUpstreamError(validateErr.Error()))
			return
		}
		host = validatedHost
		port = validatedPort
		addr = net.JoinHostPort(host, strconv.Itoa(port))
	}

	var vncConn net.Conn
	var dialErr error
	if skipOutboundValidation {
		// #nosec G102,G704 -- Host and resolved IPs are validated by DialManualDeviceTCPTimeout; this branch is an explicit manual-device policy.
		vncConn, dialErr = protocols.DialManualDeviceTCPTimeout(r.Context(), host, port, 10*time.Second)
	} else {
		vncConn, dialErr = securityruntime.DialOutboundTCPTimeout(host, port, 10*time.Second)
	}
	if dialErr != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to connect to VNC: "+d.SanitizeUpstreamError(dialErr.Error()))
		return
	}
	defer vncConn.Close()

	wsConn, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer wsConn.Close()
	wsConn.SetReadLimit(d.MaxDesktopInputReadBytes)

	securityruntime.Logf("desktop: direct VNC proxy for %s -> %s", session.ID, addr)

	// Bidirectional bridge: browser WS <-> VNC TCP.
	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }

	// writeMu guards all writes to wsConn. gorilla/websocket does not
	// allow concurrent writers; the VNC->Browser goroutine writes frames
	// while the Browser->VNC loop could also call WriteControl on error.
	var writeMu sync.Mutex
	stopKeepalive := d.StartBrowserWSKeepalive(wsConn, &writeMu, "desktop-direct-vnc:"+session.ID)
	defer stopKeepalive()

	// VNC -> Browser
	go func() {
		defer closeDone()
		buf := make([]byte, 16384)
		for {
			n, readErr := vncConn.Read(buf)
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

	// Browser -> VNC
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
			log.Printf("desktop: direct VNC browser input panic for %s: %v", session.ID, recovered) // #nosec G706 -- Session IDs are hub-generated identifiers and the panic value is local runtime state.
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
		if _, writeErr := vncConn.Write(payload); writeErr != nil {
			return
		}
	}
}
