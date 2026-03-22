package terminal

import (
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func (d *Deps) HandleSessionStreamTicket(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.EnforceRateLimit(w, r, "terminal.stream_ticket.create", 240, time.Minute) {
		return
	}

	ticket, expiresAt, err := d.IssueStreamTicket(r.Context(), session.ID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to issue stream ticket")
		return
	}

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
		"session_id":  session.ID,
		"ticket":      ticket,
		"expires_at":  expiresAt,
		"stream_path": fmt.Sprintf("/terminal/sessions/%s/stream?ticket=%s", neturl.PathEscape(session.ID), neturl.QueryEscape(ticket)),
	})
}

func (d *Deps) HandleSessionStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer d.MarkPersistentTerminalStreamDetached(session)
	traceID := shared.BrowserStreamTraceID(r)
	traceLog := shared.StreamTraceLogValue(traceID)

	// Priority -1: explicit hub-local diagnostics shell (backend-only, env-gated).
	if IsHubLocalTerminalTarget(session.Target) {
		if !HubLocalTerminalEnabled() {
			servicehttp.WriteError(w, http.StatusForbidden, "hub local terminal is disabled")
			return
		}
		if !d.IsOwnerActor(d.PrincipalActorID(r.Context())) {
			servicehttp.WriteError(w, http.StatusForbidden, "hub local terminal requires owner role")
			return
		}
		d.HandleHubLocalTerminalStream(w, r, session)
		return
	}

	// Priority 0: Docker container terminal via docker exec bridge.
	if agentID, containerID, ok := d.ResolveDockerExecSessionTarget(session.Target); ok {
		d.HandleDockerExecTerminalStream(w, r, session, agentID, containerID)
		return
	}

	// Priority 1: Route through connected agent (zero config, automatic).
	if d.AgentMgr != nil && d.AgentMgr.IsConnected(session.Target) {
		d.HandleAgentTerminalStream(w, r, session)
		return
	}

	// Priority 2: Proxmox API websocket bridge for Proxmox-backed assets.
	if proxmoxTarget, ok, err := d.ResolveProxmoxSessionTarget(session.Target); err == nil && ok {
		if proxyErr := d.TryProxmoxTerminalStream(w, r, session, proxmoxTarget); proxyErr == nil {
			return
		} else {
			log.Printf("terminal-proxmox: falling through to SSH session=%s target=%s trace=%s err=%v", session.ID, session.Target, traceLog, proxyErr) // #nosec G706 -- Session, target, and trace IDs are hub-controlled runtime identifiers.
		}
	} else if err != nil {
		log.Printf("terminal: proxmox target resolution failed session=%s target=%s trace=%s err=%v", session.ID, session.Target, traceLog, err) // #nosec G706 -- Session, target, and trace IDs are hub-controlled runtime identifiers.
	}

	// Priority 3: TrueNAS WebSocket shell for TrueNAS-backed assets.
	if truenasTarget, ok, err := d.ResolveTrueNASSessionTarget(session.Target); err == nil && ok {
		if proxyErr := d.TryTrueNASTerminalStream(w, r, session, truenasTarget); proxyErr == nil {
			return
		} else {
			log.Printf("terminal-truenas: falling through to SSH session=%s target=%s trace=%s err=%v", session.ID, session.Target, traceLog, proxyErr) // #nosec G706 -- Session, target, and trace IDs are hub-controlled runtime identifiers.
		}
	} else if err != nil {
		log.Printf("terminal: truenas target resolution failed session=%s target=%s trace=%s err=%v", session.ID, session.Target, traceLog, err) // #nosec G706 -- Session, target, and trace IDs are hub-controlled runtime identifiers.
	}

	// Priority 3.5: Telnet via asset_protocol_configs (manual device).
	if d.GetProtocolConfig != nil {
		telnetCfg, err := d.GetProtocolConfig(r.Context(), session.Target, protocols.ProtocolTelnet)
		if err != nil {
			log.Printf("terminal: telnet protocol config lookup failed session=%s target=%s trace=%s err=%v", session.ID, session.Target, traceLog, err) // #nosec G706 -- Session, target, and trace IDs are hub-controlled runtime identifiers.
		} else if telnetCfg != nil && telnetCfg.Enabled {
			host := strings.TrimSpace(telnetCfg.Host)
			// Fall back to asset host if the protocol config has no explicit host.
			if host == "" && d.AssetStore != nil {
				if asset, ok, assetErr := d.AssetStore.GetAsset(session.Target); assetErr == nil && ok {
					host = strings.TrimSpace(asset.Host)
				}
			}
			if host == "" {
				host = session.Target
			}
			port := telnetCfg.Port
			if port <= 0 {
				port = protocols.DefaultPort(protocols.ProtocolTelnet)
			}
			d.HandleTelnetStream(w, r, session, host, port)
			return
		}
	}

	// Priority 4: SSH direct connection.
	d.HandleSSHTerminalStream(w, r, session)
}

func (d *Deps) MarkPersistentTerminalStreamDetached(session terminal.Session) {
	if d.TerminalPersistentStore == nil {
		return
	}
	persistentSessionID := strings.TrimSpace(session.PersistentSessionID)
	if persistentSessionID == "" {
		return
	}

	// Flush in-memory ring buffer to Postgres before marking detached.
	if d.TerminalInMemStore != nil && d.TerminalScrollbackStore != nil {
		ringBuf := d.TerminalInMemStore.GetOrCreateScrollbackBuffer(persistentSessionID)
		if snap := ringBuf.Snapshot(); len(snap) > 0 {
			_ = d.TerminalScrollbackStore.UpsertScrollback(persistentSessionID, snap, ringBuf.ByteSize(), ringBuf.Lines())
		}
	}

	_, _ = d.TerminalPersistentStore.MarkPersistentSessionDetached(persistentSessionID, time.Now().UTC())
}
