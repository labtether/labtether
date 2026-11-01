package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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
)

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
		if err := s.legacyOwnerAgentCredentialValidator(assetID)(); err != nil {
			if errors.Is(err, persistence.ErrAgentIdentityRetired) || errors.Is(err, persistence.ErrNotFound) {
				securityruntime.Logf("agentws: rejected owner-token connection for inactive asset %s", assetID)
				http.Error(w, "agent asset must already be enrolled and active", http.StatusConflict)
				return
			}
			http.Error(w, "agent asset unavailable", http.StatusServiceUnavailable)
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
		conn.SetCredentialValidator(s.legacyOwnerAgentCredentialValidator(assetID))
	}
	if agentVersion != "" {
		conn.SetMeta("agent_version", agentVersion)
	}
	// Where the Hub's SSH fallback may connect: an address the Hub observed,
	// never one the agent reports about itself.
	conn.SetMeta("ssh_source_ip", agentSSHSourceAddress(r))
	if err := conn.ValidateCredential(); err != nil {
		conn.Close()
		return
	}

	// Publish presence inside the same lifecycle transition as replacement.
	sessionID := generateRequestID()
	conn.SetMeta("presence.session_id", sessionID)
	s.agentMgr.RegisterWithLifecycle(conn, func(owner *agentmgr.AgentConn) {
		s.cleanupAgentConnectionState(owner.AssetID, owner.Meta("presence.session_id"))
	}, func() {
		now := time.Now().UTC()
		if s.presenceStore != nil {
			_ = s.presenceStore.UpsertPresence(persistence.AgentPresence{
				AssetID: assetID, Transport: "agent", ConnectedAt: now,
				LastHeartbeatAt: now, SessionID: sessionID,
			})
			_ = s.presenceStore.UpdateAssetTransportType(assetID, "agent")
		}
		if s.broadcaster != nil {
			s.broadcaster.Broadcast("agent.connected", map[string]any{"asset_id": assetID})
		}
	})

	defer s.finishAgentWebSocketConnection(conn, sessionID)

	// Proactively probe tmux availability so the result is cached before
	// any terminal session is started. Without this, the first terminal
	// connect always misses the probe (async race).
	s.ensureTerminalDeps().StartAgentTmuxProbeAsync(conn)

	// Install the Hub SSH key only where an admin turned it on, and finish a
	// key removal that waited for this agent to connect.
	s.syncHubSSHKeyOnConnect(conn)

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
		// Over the rate, pause reading so TCP backpressure slows the agent.
		// Only a frame the budget can never pay for closes the socket.
		wait, fits := inboundBudget.reserve(len(payload), time.Now())
		if !fits {
			securityruntime.Logf("agentws: %d-byte frame from %s exceeds the inbound budget", len(payload), assetID)
			_ = conn.WriteClose(websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "inbound rate limit exceeded"))
			return
		}
		if !waitAgentWSInboundBudget(conn.Done(), wait) {
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
		if !s.agentMgr.DispatchIfCurrent(conn, func() {
			s.dispatchAgentWebSocketMessage(router, assetID, conn, msg)
		}) {
			return
		}
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

func (s *apiServer) legacyOwnerAgentCredentialValidator(assetID string) func() error {
	return func() error {
		transactions, ok := s.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
		if !ok || s.assetStore == nil {
			return persistence.ErrAgentEnrollmentTransactionsUnavailable
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		retired, err := transactions.IsAgentIdentityRetired(ctx, assetID)
		if err != nil {
			return err
		}
		if retired {
			return persistence.ErrAgentIdentityRetired
		}
		_, exists, err := s.assetStore.GetAsset(assetID)
		if err != nil {
			return err
		}
		if !exists {
			return persistence.ErrNotFound
		}
		return nil
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
