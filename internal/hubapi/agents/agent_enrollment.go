package agents

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/platforms"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

// PendingAgent tracks an agent that has connected without credentials
// and is waiting for an operator to approve or reject its enrollment.
type PendingAgent struct {
	AssetID            string     `json:"asset_id"`
	Hostname           string     `json:"hostname"`
	Platform           string     `json:"platform"`
	RemoteIP           string     `json:"remote_ip"`
	ConnectedAt        time.Time  `json:"connected_at"`
	DeviceFingerprint  string     `json:"device_fingerprint,omitempty"`
	DeviceKeyAlg       string     `json:"device_key_alg,omitempty"`
	DevicePublicKey    string     `json:"device_public_key,omitempty"`
	IdentityVerified   bool       `json:"identity_verified"`
	IdentityVerifiedAt *time.Time `json:"identity_verified_at,omitempty"`
	ChallengeNonce     string
	ChallengeExpiresAt time.Time
	ConnMu             sync.Mutex      // protects writes to conn
	Conn               *websocket.Conn // unexported — used to send approval/rejection
}

// PendingAgentInfo is the JSON-safe representation of a PendingAgent,
// omitting the unexported WebSocket connection field.
type PendingAgentInfo struct {
	AssetID            string     `json:"asset_id"`
	Hostname           string     `json:"hostname"`
	Platform           string     `json:"platform"`
	RemoteIP           string     `json:"remote_ip"`
	ConnectedAt        time.Time  `json:"connected_at"`
	DeviceFingerprint  string     `json:"device_fingerprint,omitempty"`
	DeviceKeyAlg       string     `json:"device_key_alg,omitempty"`
	IdentityVerified   bool       `json:"identity_verified"`
	IdentityVerifiedAt *time.Time `json:"identity_verified_at,omitempty"`
}

// PendingAgents is a thread-safe in-memory registry of pending enrollment agents.
type PendingAgents struct {
	mu     sync.RWMutex
	agents map[string]*PendingAgent
}

const (
	maxPendingEnrollmentAgents       = 200
	maxPendingEnrollmentPerIP        = 5
	maxPendingEnrollmentTimeout      = 10 * time.Minute
	pendingChallengeTTL              = 2 * time.Minute
	maxAgentMessageBytes             = 32 << 20 // 32MB — matches agent_ws_handler.go
	maxPendingEnrollmentMessageBytes = maxAgentMessageBytes

	maxPendingFingerprintLen = 160
	maxPendingKeyAlgLen      = 64
	maxPendingPublicKeyLen   = 512
	maxPendingHostnameIDLen  = 64
)

var PendingEnrollmentAfterFunc = time.AfterFunc

// handlePendingEnrollment upgrades the connection to WebSocket and parks the
// agent in the pending registry until an operator approves or rejects it.
// It is called when an agent connects without a token but with the
// X-Request-Enrollment: true header set.
func (d *Deps) HandlePendingEnrollment(w http.ResponseWriter, r *http.Request) {
	hostname := strings.TrimSpace(r.Header.Get("X-Hostname"))
	platform := strings.TrimSpace(r.Header.Get("X-Platform"))
	deviceFingerprint := SanitizePendingIdentityHeader(r.Header.Get("X-Device-Fingerprint"), maxPendingFingerprintLen)
	deviceKeyAlg := strings.ToLower(SanitizePendingIdentityHeader(r.Header.Get("X-Device-Key-Alg"), maxPendingKeyAlgLen))
	devicePublicKey := SanitizePendingIdentityHeader(r.Header.Get("X-Device-Public-Key"), maxPendingPublicKeyLen)
	remoteIP := shared.RequestClientKey(r)

	if !d.EnforceRateLimit(w, r, "agent.enrollment.pending", 20, time.Minute) {
		return
	}
	if d.PendingAgents.Count() >= maxPendingEnrollmentAgents {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "pending enrollment capacity reached")
		return
	}
	if d.PendingAgents.CountByRemoteIP(remoteIP) >= maxPendingEnrollmentPerIP {
		servicehttp.WriteError(w, http.StatusTooManyRequests, "too many pending enrollment connections from this source")
		return
	}

	if hostname == "" {
		hostname = "unknown"
	}
	if platform != "" {
		platform = platforms.Resolve(platform, "", "", "", "")
	}
	if deviceKeyAlg == "" {
		deviceKeyAlg = "unknown"
	}

	// Derive a temporary, unique asset ID for this pending connection.
	tempAssetID := BuildPendingEnrollmentAssetID(hostname)

	wsConn, err := d.AgentWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		securityruntime.Logf("enrollment: WebSocket upgrade failed for pending agent %s: %v", hostname, err)
		return
	}
	wsConn.SetReadLimit(maxPendingEnrollmentMessageBytes)

	agent := &PendingAgent{
		AssetID:           tempAssetID,
		Hostname:          hostname,
		Platform:          platform,
		RemoteIP:          remoteIP,
		ConnectedAt:       time.Now().UTC(),
		DeviceFingerprint: deviceFingerprint,
		DeviceKeyAlg:      deviceKeyAlg,
		DevicePublicKey:   devicePublicKey,
		Conn:              wsConn,
	}

	d.PendingAgents.Add(agent)
	securityruntime.Logf("enrollment: pending agent connected hostname=%s asset_id=%s remote=%s", hostname, tempAssetID, remoteIP)

	d.broadcastEvent("enrollment.pending", map[string]any{
		"asset_id":           tempAssetID,
		"hostname":           hostname,
		"platform":           platform,
		"device_fingerprint": deviceFingerprint,
		"device_key_alg":     deviceKeyAlg,
		"identity_verified":  false,
	})

	if err := d.SendPendingEnrollmentChallenge(agent); err != nil {
		securityruntime.Logf("enrollment: failed to send challenge asset_id=%s: %v", tempAssetID, err)
	}

	defer func() {
		d.PendingAgents.Remove(tempAssetID)
		_ = wsConn.Close()
		securityruntime.Logf("enrollment: pending agent disconnected hostname=%s asset_id=%s", hostname, tempAssetID)
		d.broadcastEvent("enrollment.pending_removed", map[string]any{
			"asset_id": tempAssetID,
		})
	}()
	disconnectTimeout := PendingEnrollmentAfterFunc(maxPendingEnrollmentTimeout, func() {
		securityruntime.Logf("enrollment: pending agent timeout asset_id=%s remote=%s", tempAssetID, remoteIP)
		_ = wsConn.Close()
	})
	defer disconnectTimeout.Stop()

	// Hold the connection open while waiting for operator decision and for
	// enrollment proof messages from the pending agent.
	for {
		_, payload, err := wsConn.ReadMessage()
		if err != nil {
			// Normal close or network error — exit the loop.
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) &&
				!websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				securityruntime.Logf("enrollment: read error for pending agent %s: %v", tempAssetID, err)
			}
			return
		}

		var msg agentmgr.Message
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case agentmgr.MsgEnrollmentProof:
			if err := d.VerifyPendingEnrollmentProof(agent, msg); err != nil {
				securityruntime.Logf("enrollment: invalid proof asset_id=%s: %v", tempAssetID, err)
				continue
			}
			securityruntime.Logf("enrollment: pending identity verified hostname=%s asset_id=%s fp=%s",
				hostname, tempAssetID, agent.DeviceFingerprint)
			d.broadcastEvent("enrollment.pending_verified", map[string]any{
				"asset_id":           tempAssetID,
				"hostname":           hostname,
				"device_fingerprint": agent.DeviceFingerprint,
				"identity_verified":  true,
			})
		}
	}
}

// handleListPendingAgents returns the list of agents waiting for enrollment approval.
// GET /api/v1/agents/pending
func (d *Deps) HandleListPendingAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	list := d.PendingAgents.List()
	if list == nil {
		list = []PendingAgentInfo{}
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"count":  len(list),
		"agents": list,
	})
}

// handleApproveAgent approves a pending agent enrollment, issues it a
// per-agent token, upserts its asset record, and notifies it via WebSocket.
// POST /api/v1/agents/approve
func (d *Deps) HandleApproveAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	assetID, ok := DecodePendingEnrollmentAssetID(w, r)
	if !ok {
		return
	}

	agent, ok := d.PendingAgents.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "pending agent not found")
		return
	}
	if !agent.IdentityVerified {
		servicehttp.WriteError(w, http.StatusConflict, "pending agent identity is not verified yet")
		return
	}

	if d.EnrollmentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "enrollment store not available")
		return
	}

	stableAssetID := ResolveApprovedAssetID(agent, assetID)

	// Revoke any existing agent token for this asset before issuing a new one.
	_ = d.EnrollmentStore.RevokeAgentTokensByAsset(stableAssetID)

	// Generate a per-agent token.
	rawToken, hashedToken, err := auth.GenerateSessionToken()
	if err != nil {
		log.Printf("enrollment: failed to generate agent token for %s: %v", stableAssetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to generate agent token")
		return
	}
	expiresAt := NewAgentTokenExpiry(time.Now().UTC())

	if _, err := d.EnrollmentStore.CreateAgentToken(stableAssetID, hashedToken, "console-approval", expiresAt); err != nil {
		log.Printf("enrollment: failed to store agent token for %s: %v", stableAssetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to store agent token")
		return
	}

	// Upsert the asset record so it appears in the asset inventory.
	metadata := map[string]string{}
	if agent.DeviceFingerprint != "" {
		metadata["agent_device_fingerprint"] = agent.DeviceFingerprint
	}
	if agent.DeviceKeyAlg != "" {
		metadata["agent_device_key_alg"] = agent.DeviceKeyAlg
	}
	if agent.IdentityVerifiedAt != nil {
		metadata["agent_identity_verified_at"] = agent.IdentityVerifiedAt.UTC().Format(time.RFC3339)
	}
	hbReq := assets.HeartbeatRequest{
		AssetID:  stableAssetID,
		Type:     "node",
		Name:     agent.Hostname,
		Source:   "agent",
		Status:   "pending",
		Platform: agent.Platform,
		Metadata: metadata,
	}
	if _, err := d.ProcessHeartbeatRequest(hbReq); err != nil {
		log.Printf("enrollment: heartbeat upsert failed for %s: %v", stableAssetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		// Non-fatal — the token is issued, so continue.
	}

	// Send the approval message with token and final asset ID over WebSocket.
	if err := SendPendingEnrollmentDecision(agent, agentmgr.MsgEnrollmentApproved, agentmgr.EnrollmentApprovedData{
		Token:   rawToken,
		AssetID: stableAssetID,
	}, ""); err != nil {
		log.Printf("enrollment: failed to send approval to %s: %v", stableAssetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		// Close the connection so the read loop exits and the agent reconnects
		// with its new token instead of lingering in a zombie state.
		_ = agent.Conn.Close()
	}

	d.PendingAgents.Remove(assetID)
	log.Printf("enrollment: approved pending agent hostname=%s stable_asset_id=%s", agent.Hostname, stableAssetID) // #nosec G706 -- Hostname and asset ID are bounded enrollment fields already persisted by the hub.

	d.broadcastEvent("enrollment.approved", map[string]any{
		"asset_id": stableAssetID,
		"hostname": agent.Hostname,
	})

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":             "approved",
		"asset_id":           stableAssetID,
		"device_fingerprint": agent.DeviceFingerprint,
		"identity_verified":  true,
	})
}

// handleRejectAgent rejects a pending agent enrollment, notifies it via
// WebSocket, and closes the connection.
// POST /api/v1/agents/reject
func (d *Deps) HandleRejectAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	assetID, ok := DecodePendingEnrollmentAssetID(w, r)
	if !ok {
		return
	}

	agent, ok := d.PendingAgents.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "pending agent not found")
		return
	}

	// Send rejection message before closing the connection.
	if err := SendPendingEnrollmentDecision(agent, agentmgr.MsgEnrollmentRejected, agentmgr.EnrollmentRejectedData{
		Reason: "rejected by operator",
	}, "enrollment rejected"); err != nil {
		log.Printf("enrollment: failed to send rejection to %s: %v", assetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
	}

	d.PendingAgents.Remove(assetID)
	log.Printf("enrollment: rejected pending agent hostname=%s asset_id=%s", agent.Hostname, assetID) // #nosec G706 -- Hostname and asset ID are bounded enrollment fields already persisted by the hub.

	d.broadcastEvent("enrollment.rejected", map[string]any{
		"asset_id": assetID,
		"hostname": agent.Hostname,
	})

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "rejected",
	})
}
