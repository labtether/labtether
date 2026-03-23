package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	agentPingInterval    = 30 * time.Second
	agentReadDeadline    = 60 * time.Second
	agentWriteDeadline   = agentmgr.AgentWriteDeadline
	maxAgentMessageBytes = 32 << 20 // 32MB — covers file chunks and VNC frames
)

var agentWebSocketUpgrader = websocket.Upgrader{
	// Agent clients are non-browser and typically omit Origin.
	// If Origin is present (e.g. browser tooling), enforce same-host policy.
	CheckOrigin: checkSameOrigin,
}

// buildWSRouter constructs a closure-based WebSocket message router.
// Each entry captures the apiServer receiver, so handlers use the
// shared.WSHandler signature: func(conn, msg) with no server argument.
func (s *apiServer) buildWSRouter() shared.WSRouter {
	router := make(shared.WSRouter, 64)

	router[agentmgr.MsgHeartbeat] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentHeartbeat(conn, msg)
	}
	router[agentmgr.MsgTelemetry] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentTelemetry(conn, msg)
	}
	router[agentmgr.MsgCommandResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentCommandResult(conn, msg)
	}
	router[agentmgr.MsgPong] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentPong(conn, msg)
	}
	router[agentmgr.MsgLogStream] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentLogStream(conn, msg)
	}
	router[agentmgr.MsgLogBatch] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentLogBatch(conn, msg)
	}
	router[agentmgr.MsgJournalEntries] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentJournalEntries(conn, msg)
	}
	router[agentmgr.MsgUpdateProgress] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentUpdateProgress(conn, msg)
	}
	router[agentmgr.MsgUpdateResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentUpdateResult(conn, msg)
	}
	router[agentmgr.MsgTerminalProbed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentTerminalProbed(conn, msg)
	}
	router[agentmgr.MsgTerminalStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentTerminalStarted(conn, msg)
	}
	router[agentmgr.MsgTerminalData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentTerminalData(conn, msg)
	}
	router[agentmgr.MsgTerminalClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentTerminalClosed(conn, msg)
	}
	router[agentmgr.MsgSSHKeyInstalled] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentSSHKeyInstalled(conn, msg)
	}
	router[agentmgr.MsgSSHKeyRemoved] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentSSHKeyRemoved(conn, msg)
	}
	router[agentmgr.MsgDesktopStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopStarted(conn, msg)
	}
	router[agentmgr.MsgDesktopData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopData(conn, msg)
	}
	router[agentmgr.MsgDesktopClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopClosed(conn, msg)
	}
	router[agentmgr.MsgDesktopDisplays] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopDisplays(conn, msg)
	}
	router[agentmgr.MsgDesktopAudioData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopAudioData(conn, msg)
	}
	router[agentmgr.MsgDesktopAudioState] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopAudioState(conn, msg)
	}
	router[agentmgr.MsgWebRTCCapabilities] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebRTCCapabilities(conn, msg)
	}
	router[agentmgr.MsgWebRTCStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebRTCStarted(conn, msg)
	}
	router[agentmgr.MsgWebRTCAnswer] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebRTCAnswer(conn, msg)
	}
	router[agentmgr.MsgWebRTCICE] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebRTCICE(conn, msg)
	}
	router[agentmgr.MsgWebRTCStopped] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebRTCStopped(conn, msg)
	}
	router[agentmgr.MsgWoLResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWoLResult(conn, msg)
	}
	router[agentmgr.MsgFileListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentFileListed(conn, msg)
	}
	router[agentmgr.MsgFileData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentFileData(conn, msg)
	}
	router[agentmgr.MsgFileWritten] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentFileWritten(conn, msg)
	}
	router[agentmgr.MsgFileResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentFileResult(conn, msg)
	}
	router[agentmgr.MsgProcessListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentProcessListed(conn, msg)
	}
	router[agentmgr.MsgProcessKillResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentProcessKillResult(conn, msg)
	}
	router[agentmgr.MsgServiceListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentServiceListed(conn, msg)
	}
	router[agentmgr.MsgServiceResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentServiceResult(conn, msg)
	}
	router[agentmgr.MsgDiskListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDiskListed(conn, msg)
	}
	router[agentmgr.MsgNetworkListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentNetworkListed(conn, msg)
	}
	router[agentmgr.MsgNetworkResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentNetworkResult(conn, msg)
	}
	router[agentmgr.MsgPackageListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentPackageListed(conn, msg)
	}
	router[agentmgr.MsgPackageResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentPackageResult(conn, msg)
	}
	router[agentmgr.MsgCronListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentCronListed(conn, msg)
	}
	router[agentmgr.MsgUsersListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentUsersListed(conn, msg)
	}
	router[agentmgr.MsgConfigApplied] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentConfigApplied(conn, msg)
	}
	router[agentmgr.MsgAgentSettingsApplied] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentSettingsApplied(conn, msg)
	}
	router[agentmgr.MsgAgentSettingsState] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentSettingsState(conn, msg)
	}
	router[agentmgr.MsgDockerDiscovery] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerDiscovery(conn, msg)
	}
	router[agentmgr.MsgDockerDiscoveryDelta] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerDiscoveryDelta(conn, msg)
	}
	router[agentmgr.MsgDockerStats] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerStats(conn, msg)
	}
	router[agentmgr.MsgDockerEvents] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerEvents(conn, msg)
	}
	router[agentmgr.MsgDockerActionResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerActionResult(conn, msg)
	}
	router[agentmgr.MsgDockerExecStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerExecStartedMessage(conn, msg)
	}
	router[agentmgr.MsgDockerExecData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerExecDataMessage(conn, msg)
	}
	router[agentmgr.MsgDockerExecClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerExecClosedMessage(conn, msg)
	}
	router[agentmgr.MsgDockerLogsStream] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerLogsStreamMessage(conn, msg)
	}
	router[agentmgr.MsgDockerComposeResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerComposeResult(conn, msg)
	}
	router[agentmgr.MsgWebServiceReport] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebServiceReport(conn, msg)
	}
	router[agentmgr.MsgClipboardData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentClipboardData(conn, msg)
	}
	router[agentmgr.MsgClipboardSetAck] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentClipboardSetAck(conn, msg)
	}
	router[agentmgr.MsgDesktopDiagnosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopDiagnosed(conn, msg)
	}

	return router
}

// handleAgentWebSocket upgrades an agent HTTP connection to WebSocket,
// registers it with the AgentManager, and processes messages in a read loop.
func (s *apiServer) handleAgentWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.demoMode {
		http.Error(w, `{"error":"Agent connections are disabled in demo mode.","demo":true}`, http.StatusForbidden)
		return
	}

	assetID := strings.TrimSpace(r.Header.Get("X-Asset-ID"))
	platform := strings.TrimSpace(r.Header.Get("X-Platform"))
	agentVersion := strings.TrimSpace(r.Header.Get("X-Agent-Version"))
	if assetID == "" {
		http.Error(w, "X-Asset-ID header required", http.StatusBadRequest)
		return
	}

	// Authenticate before upgrading — owner token first, then per-agent token fallback.
	agentTokenID := ""
	if !s.validateOwnerTokenRequest(r) {
		// Try per-agent token auth
		extracted := auth.ExtractBearerToken(r)
		if extracted == "" || s.enrollmentStore == nil {
			// If the agent requests enrollment approval flow, park it in pending state.
			if r.Header.Get("X-Request-Enrollment") == "true" {
				s.handlePendingEnrollment(w, r)
				return
			}
			securityruntime.Logf("agentws: rejected connection from %s: no valid token (asset=%s)",
				r.RemoteAddr, r.Header.Get("X-Asset-ID"))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		hashed := auth.HashToken(extracted)
		agentTok, valid, err := s.enrollmentStore.ValidateAgentToken(hashed)
		if err != nil || !valid {
			securityruntime.Logf("agentws: rejected connection from %s: invalid agent token (asset=%s)",
				r.RemoteAddr, r.Header.Get("X-Asset-ID"))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		tokenAssetID := strings.TrimSpace(agentTok.AssetID)
		if tokenAssetID == "" || !strings.EqualFold(tokenAssetID, assetID) {
			securityruntime.Logf(
				"agentws: rejected connection from %s: token asset mismatch (token_asset=%s header_asset=%s)",
				r.RemoteAddr,
				tokenAssetID,
				assetID,
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		agentTokenID = agentTok.ID
		// Touch last_used_at in background with timeout to prevent goroutine pile-up.
		go func() {
			_ = s.enrollmentStore.TouchAgentTokenLastUsed(agentTok.ID)
		}()
	}
	_ = agentTokenID // used for audit if needed later

	wsConn, err := agentWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		securityruntime.Logf("agentws: upgrade failed for %s: %v", assetID, err)
		return
	}

	conn := agentmgr.NewAgentConn(wsConn, assetID, platform)
	if agentVersion != "" {
		conn.SetMeta("agent_version", agentVersion)
	}
	wsConn.SetReadLimit(maxAgentMessageBytes)
	s.agentMgr.Register(conn)

	// Track presence in DB.
	sessionID := generateRequestID()
	conn.SetMeta("presence.session_id", sessionID)
	now := time.Now().UTC()
	if s.presenceStore != nil {
		_ = s.presenceStore.UpsertPresence(persistence.AgentPresence{
			AssetID:         assetID,
			Transport:       "agent",
			ConnectedAt:     now,
			LastHeartbeatAt: now,
			SessionID:       sessionID,
		})
		_ = s.presenceStore.UpdateAssetTransportType(assetID, "agent")
	}

	if s.broadcaster != nil {
		s.broadcaster.Broadcast("agent.connected", map[string]any{"asset_id": assetID})
	}

	// Proactively probe tmux availability so the result is cached before
	// any terminal session is started. Without this, the first terminal
	// connect always misses the probe (async race).
	s.ensureTerminalDeps().StartAgentTmuxProbeAsync(conn)

	defer func() {
		removed := true
		if s.agentMgr != nil {
			removed = s.agentMgr.UnregisterIfMatch(assetID, conn)
		}
		if !removed {
			// A newer connection for the same asset is active; do not run global
			// disconnect cleanup for this stale connection.
			return
		}
		if s.agentMgr != nil && s.agentMgr.IsConnected(assetID) {
			// Asset reconnected after we unregistered this socket; skip global
			// disconnect side effects for the previous connection.
			return
		}

		if s.broadcaster != nil {
			s.broadcaster.Broadcast("agent.disconnected", map[string]any{"asset_id": assetID})
		}
		s.closeWebRTCBridgesForAsset(assetID)
		s.closeDockerExecBridgesForAsset(assetID)
		s.closeDesktopBridgesForAsset(assetID)
		s.closeTerminalBridgesForAsset(assetID)
		if s.presenceStore != nil {
			deleted, err := s.presenceStore.DeletePresenceForSession(assetID, sessionID)
			if err != nil {
				securityruntime.Logf("agentws: presence delete failed for %s session=%s: %v", assetID, sessionID, err)
			}
			if deleted {
				_ = s.presenceStore.UpdateAssetTransportType(assetID, "offline")
			}
		}
		if s.dockerCoordinator != nil {
			s.dockerCoordinator.RemoveHost(assetID)
		}
		if s.webServiceCoordinator != nil {
			s.webServiceCoordinator.MarkHostDisconnected(assetID)
		}
	}()

	// Send hub SSH public key for auto-provisioning.
	if s.hubIdentity != nil {
		s.sendSSHKeyInstall(conn)
	}

	// Push current runtime config to the agent.
	s.sendConfigUpdate(conn)
	s.sendAgentSettingsApply(conn)
	// Pre-warm terminal capability metadata so first terminal open is instant.
	s.startAgentTmuxProbeAsync(conn)

	// Pong handler resets the read deadline.
	conn.SetPongHandler(func(_ string) error {
		return conn.SetReadDeadline(time.Now().Add(agentReadDeadline))
	})

	// Ping goroutine — sends pings every 30s to detect dead connections.
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(agentPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WritePing(); err != nil {
					securityruntime.Logf("agentws: ping failed for %s: %v", assetID, err)
					return
				}
			}
		}
	}()

	// Build the closure-based router once per connection.
	router := s.buildWSRouter()

	// Read loop.
	_ = conn.SetReadDeadline(time.Now().Add(agentReadDeadline))
	for {
		var msg agentmgr.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				securityruntime.Logf("agentws: read error for %s: %v", assetID, err)
			}
			return
		}

		conn.TouchLastMessage()
		s.dispatchAgentWebSocketMessage(router, assetID, conn, msg)
	}
}

func (s *apiServer) dispatchAgentWebSocketMessage(router shared.WSRouter, assetID string, conn *agentmgr.AgentConn, msg agentmgr.Message) {
	handler, ok := router[msg.Type]
	if !ok {
		securityruntime.Logf("agentws: unknown message type %q from %s", msg.Type, assetID)
		return
	}
	handler(conn, msg)
}

func (s *apiServer) processAgentPong(conn *agentmgr.AgentConn, _ agentmgr.Message) {
	// Application-level pong — reset deadline.
	_ = conn.SetReadDeadline(time.Now().Add(agentReadDeadline))
}
