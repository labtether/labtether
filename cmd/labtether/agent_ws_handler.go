package main

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	agentPingInterval           = 30 * time.Second
	agentReadDeadline           = 60 * time.Second
	agentWriteDeadline          = agentmgr.AgentWriteDeadline
	maxAgentMessageBytes        = 32 << 20 // 32MB — covers file chunks and VNC frames
	maxAgentWSAssetIDBytes      = 255
	maxAgentWSPlatformBytes     = 64
	maxAgentWSAgentVersionBytes = 128
	maxAgentWSMessageTypeBytes  = 128

	defaultAgentWSCredentialLease = 2 * time.Second
	minAgentWSCredentialLease     = 250 * time.Millisecond
	maxAgentWSCredentialLease     = 5 * time.Second

	defaultAgentWSMessagesPerSecond = 256
	defaultAgentWSMessageBurst      = 512
	hardAgentWSMessagesPerSecond    = 8192
	hardAgentWSMessageBurst         = 16384
	defaultAgentWSBytesPerSecond    = 16 << 20
	defaultAgentWSByteBurst         = 64 << 20
	hardAgentWSBytesPerSecond       = 256 << 20
	hardAgentWSByteBurst            = 1 << 30
)

type agentWSInboundBudget struct {
	messagesPerSecond float64
	messageBurst      float64
	bytesPerSecond    float64
	byteBurst         float64
	messageTokens     float64
	byteTokens        float64
	lastRefill        time.Time
}

func newAgentWSInboundBudget() *agentWSInboundBudget {
	messagesPerSecond := boundedAgentWSEnvInt("LABTETHER_AGENT_WS_MESSAGES_PER_SECOND", defaultAgentWSMessagesPerSecond, hardAgentWSMessagesPerSecond)
	messageBurst := boundedAgentWSEnvInt("LABTETHER_AGENT_WS_MESSAGE_BURST", defaultAgentWSMessageBurst, hardAgentWSMessageBurst)
	bytesPerSecond := boundedAgentWSEnvInt("LABTETHER_AGENT_WS_BYTES_PER_SECOND", defaultAgentWSBytesPerSecond, hardAgentWSBytesPerSecond)
	byteBurst := boundedAgentWSEnvInt("LABTETHER_AGENT_WS_BYTE_BURST", defaultAgentWSByteBurst, hardAgentWSByteBurst)
	return &agentWSInboundBudget{
		messagesPerSecond: float64(messagesPerSecond),
		messageBurst:      float64(messageBurst),
		bytesPerSecond:    float64(bytesPerSecond),
		byteBurst:         float64(byteBurst),
		messageTokens:     float64(messageBurst),
		byteTokens:        float64(byteBurst),
		lastRefill:        time.Now(),
	}
}

func (b *agentWSInboundBudget) allow(frameBytes int, now time.Time) bool {
	if b == nil || frameBytes < 0 {
		return false
	}
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.messageTokens = min(b.messageBurst, b.messageTokens+elapsed*b.messagesPerSecond)
		b.byteTokens = min(b.byteBurst, b.byteTokens+elapsed*b.bytesPerSecond)
		b.lastRefill = now
	}
	if b.messageTokens < 1 || b.byteTokens < float64(frameBytes) {
		return false
	}
	b.messageTokens--
	b.byteTokens -= float64(frameBytes)
	return true
}

func boundedAgentWSEnvInt(key string, fallback, hardMax int) int {
	return enrollment.BoundedLimit(envOrDefaultInt(key, fallback), fallback, hardMax)
}

func configuredAgentWSCredentialLease() time.Duration {
	lease := envOrDefaultDuration("LABTETHER_AGENT_WS_CREDENTIAL_LEASE", defaultAgentWSCredentialLease)
	if lease < minAgentWSCredentialLease {
		return minAgentWSCredentialLease
	}
	if lease > maxAgentWSCredentialLease {
		return maxAgentWSCredentialLease
	}
	return lease
}

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
	router[agentmgr.MsgPowerResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentPowerResult(conn, msg)
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
	router[agentmgr.MsgDockerEndpointTestResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerEndpointTestResult(conn, msg)
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

	assetID, assetIDValid := boundedAgentWebSocketHeader(r.Header.Get("X-Asset-ID"), maxAgentWSAssetIDBytes)
	platform, platformValid := boundedAgentWebSocketHeader(r.Header.Get("X-Platform"), maxAgentWSPlatformBytes)
	agentVersion, agentVersionValid := boundedAgentWebSocketHeader(r.Header.Get("X-Agent-Version"), maxAgentWSAgentVersionBytes)
	upgradeHeaders := http.Header{}
	if assetID == "" {
		http.Error(w, "X-Asset-ID header required", http.StatusBadRequest)
		return
	}
	if !assetIDValid {
		http.Error(w, "invalid X-Asset-ID header", http.StatusBadRequest)
		return
	}
	if !platformValid {
		http.Error(w, "invalid X-Platform header", http.StatusBadRequest)
		return
	}
	if !agentVersionValid {
		http.Error(w, "invalid X-Agent-Version header", http.StatusBadRequest)
		return
	}

	// Authenticate before upgrading. Per-agent bearer credentials are the
	// default; the historical shared owner-token path is opt-in only.
	agentTokenID := ""
	ownerAuthenticated := s.allowLegacySharedAgentAuth && s.validateOwnerTokenRequest(r)
	if !ownerAuthenticated {
		// Try per-agent token auth
		extracted := auth.ExtractBearerToken(r)
		if extracted == "" || s.enrollmentStore == nil {
			// If the agent requests enrollment approval flow, park it in pending state.
			if r.Header.Get("X-Request-Enrollment") == "true" {
				releaseAdmission, admitted := s.reserveAgentWSAdmission(w, r)
				if !admitted {
					return
				}
				defer releaseAdmission()
				s.handlePendingEnrollment(w, r)
				return
			}
			securityruntime.Logf("agentws: rejected connection from %s: no valid token (asset=%s)",
				r.RemoteAddr, assetID)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		hashed := auth.HashToken(extracted)
		agentTok, valid, err := s.enrollmentStore.ValidateAgentToken(hashed)
		if err != nil || !valid {
			securityruntime.Logf("agentws: rejected connection from %s: invalid agent token (asset=%s)",
				r.RemoteAddr, assetID)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		tokenAssetID, tokenAssetIDValid := boundedAgentWebSocketHeader(agentTok.AssetID, maxAgentWSAssetIDBytes)
		if tokenAssetID == "" || !tokenAssetIDValid {
			securityruntime.Logf(
				"agentws: rejected connection from %s: token has no valid bound asset (header_asset=%s)",
				r.RemoteAddr,
				assetID,
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !strings.EqualFold(tokenAssetID, assetID) {
			// The validated token binding is authoritative. Older agents only
			// persisted the bearer token, not the canonical asset ID returned by
			// enrollment, so a restart could present their pre-enrollment display
			// name here. Binding the connection to tokenAssetID preserves isolation
			// while allowing those agents to self-heal.
			securityruntime.Logf(
				"agentws: normalized stale agent asset header from %s to token-bound asset %s for %s",
				assetID,
				tokenAssetID,
				r.RemoteAddr,
			)
		}
		assetID = tokenAssetID
		upgradeHeaders.Set("X-LabTether-Asset-ID", tokenAssetID)
		agentTokenID = agentTok.ID
	}
	if ownerAuthenticated {
		if s.assetStore == nil {
			http.Error(w, "agent asset unavailable", http.StatusServiceUnavailable)
			return
		}
		if _, exists, err := s.assetStore.GetAsset(assetID); err != nil {
			http.Error(w, "agent asset unavailable", http.StatusServiceUnavailable)
			return
		} else if !exists {
			securityruntime.Logf("agentws: rejected owner-token connection for missing asset %s", assetID)
			http.Error(w, "agent asset must already be enrolled", http.StatusConflict)
			return
		}
	}
	releaseAdmission, admitted := s.reserveAgentWSAdmission(w, r)
	if !admitted {
		return
	}
	defer releaseAdmission()

	wsConn, err := shared.UpgradeWebSocket(&agentWebSocketUpgrader, w, r, upgradeHeaders)
	if err != nil {
		securityruntime.Logf("agentws: upgrade failed for %s: %v", assetID, err)
		return
	}
	wsConn.SetReadLimit(maxAgentMessageBytes)
	if agentTokenID != "" && !s.agentWSCredentialStillValid(agentTokenID, assetID) {
		_ = wsConn.Close()
		return
	}

	conn := agentmgr.NewAgentConn(wsConn, assetID, platform)
	if agentTokenID != "" {
		conn.SetMeta("auth.mode", "agent-token")
		conn.SetMeta("auth.agent_token_id", agentTokenID)
		conn.SetCredentialValidatorWithLease(s.agentWSCredentialValidator(agentTokenID, assetID), configuredAgentWSCredentialLease())
	} else {
		conn.SetMeta("auth.mode", "owner-token")
	}
	if agentVersion != "" {
		conn.SetMeta("agent_version", agentVersion)
	}
	s.agentMgr.Register(conn)
	credentialValid := true
	if agentTokenID != "" {
		credentialValid = s.agentWSCredentialStillValid(agentTokenID, assetID)
	} else if _, exists, err := s.assetStore.GetAsset(assetID); err != nil || !exists {
		credentialValid = false
	}
	if !credentialValid {
		s.agentMgr.UnregisterIfMatch(assetID, conn)
		return
	}

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
	if s.currentHubSSHIdentity() != nil {
		s.sendSSHKeyInstall(conn)
	}

	// Push current runtime config to the agent.
	s.sendConfigUpdate(conn)
	s.sendAgentSettingsApply(conn)
	// Pre-warm terminal capability metadata so first terminal open is instant.
	s.startAgentTmuxProbeAsync(conn)

	// Pong handler resets the read deadline.
	conn.SetPongHandler(func(_ string) error {
		if err := conn.ValidateCredential(); err != nil {
			return err
		}
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
					if s.agentMgr != nil {
						s.agentMgr.UnregisterIfMatch(assetID, conn)
					}
					return
				}
			}
		}
	}()

	// Build the closure-based router once per connection.
	router := s.buildWSRouter()

	// Read loop. Account for the exact raw frame size before unmarshalling.
	inboundBudget := newAgentWSInboundBudget()
	_ = conn.SetReadDeadline(time.Now().Add(agentReadDeadline))
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				securityruntime.Logf("agentws: read error for %s: %v", assetID, err)
			}
			return
		}
		if !inboundBudget.allow(len(payload), time.Now()) {
			securityruntime.Logf("agentws: inbound budget exceeded for %s", assetID)
			_ = conn.WriteClose(websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "inbound rate limit exceeded"))
			return
		}

		// Revalidate before parsing attacker-controlled JSON. Heartbeats perform
		// a second atomic validation with their asset update to close the delete
		// race.
		if err := conn.ValidateCredential(); err != nil {
			if s.agentMgr != nil {
				s.agentMgr.UnregisterIfMatch(assetID, conn)
			}
			return
		}
		var msg agentmgr.Message
		if err := json.Unmarshal(payload, &msg); err != nil {
			securityruntime.Logf("agentws: malformed message from %s: %v", assetID, err)
			_ = conn.WriteClose(websocket.FormatCloseMessage(websocket.CloseUnsupportedData, "invalid JSON message"))
			return
		}
		messageType, validMessageType := boundedAgentWebSocketHeader(msg.Type, maxAgentWSMessageTypeBytes)
		if messageType == "" || !validMessageType {
			securityruntime.Logf("agentws: rejected invalid message type from %s", assetID)
			_ = conn.WriteClose(websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid message type"))
			return
		}
		msg.Type = messageType
		conn.TouchLastMessage()
		s.dispatchAgentWebSocketMessage(router, assetID, conn, msg)
	}
}

func (s *apiServer) reserveAgentWSAdmission(w http.ResponseWriter, r *http.Request) (func(), bool) {
	if s.agentMgr == nil {
		http.Error(w, "agent connection manager unavailable", http.StatusServiceUnavailable)
		return func() {}, false
	}
	globalLimit := boundedAgentWSEnvInt("LABTETHER_MAX_AGENT_CONNECTIONS", enrollment.DefaultMaxAgentConnections, enrollment.HardMaxAgentConnections)
	sourceLimit := boundedAgentWSEnvInt("LABTETHER_MAX_AGENT_CONNECTIONS_PER_SOURCE", enrollment.DefaultMaxConnectionsPerPeer, enrollment.HardMaxConnectionsPerPeer)
	release, rejection := s.agentMgr.TryReserveAdmission(shared.RequestClientKey(r), globalLimit, sourceLimit)
	switch rejection {
	case agentmgr.AdmissionAllowed:
		return release, true
	case agentmgr.AdmissionSourceLimit:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "too many agent connections from source", http.StatusTooManyRequests)
	default:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "agent connection capacity reached", http.StatusServiceUnavailable)
	}
	return func() {}, false
}

func (s *apiServer) agentWSCredentialStillValid(tokenID, assetID string) bool {
	return s.agentWSCredentialValidator(tokenID, assetID)() == nil
}

func (s *apiServer) agentWSCredentialValidator(tokenID, assetID string) func() error {
	return func() error {
		transactions, ok := s.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
		if !ok || strings.TrimSpace(tokenID) == "" {
			return persistence.ErrAgentCredentialInactive
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return transactions.ValidateActiveAgentTokenID(ctx, tokenID, assetID)
	}
}

func boundedAgentWebSocketHeader(raw string, maxBytes int) (string, bool) {
	value := strings.TrimSpace(raw)
	if len(value) > maxBytes || !utf8.ValidString(value) {
		return "", false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return value, true
}

func (s *apiServer) dispatchAgentWebSocketMessage(router shared.WSRouter, assetID string, conn *agentmgr.AgentConn, msg agentmgr.Message) {
	defer func() {
		if err := recover(); err != nil {
			securityruntime.Logf("agentws: panic in handler for message type %q from %s: %v\n%s", msg.Type, assetID, err, debug.Stack())
		}
	}()
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
