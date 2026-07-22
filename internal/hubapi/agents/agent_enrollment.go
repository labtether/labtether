package agents

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
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
	DecisionClaimed    bool
	ClaimVersion       uint64
	Disconnected       bool
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

type PendingDecisionClaim struct {
	Info         PendingAgentInfo
	conn         *websocket.Conn
	connMu       *sync.Mutex
	record       *PendingAgent
	claimVersion uint64
}

var (
	ErrPendingAgentNotFound         = errors.New("pending agent not found")
	ErrPendingAgentAlreadyClaimed   = errors.New("pending agent decision already in progress")
	ErrPendingAgentIdentityUnproven = errors.New("pending agent identity is not verified")
	ErrPendingCapacityReached       = errors.New("pending enrollment capacity reached")
	ErrPendingPerIPCapacityReached  = errors.New("pending enrollment source capacity reached")
)

const (
	maxPendingEnrollmentAgents        = 200
	maxPendingEnrollmentPerIP         = 5
	maxPendingEnrollmentTimeout       = 10 * time.Minute
	pendingChallengeTTL               = 2 * time.Minute
	pendingApprovalDeliveryTTL        = 2 * time.Minute
	maxPendingEnrollmentMessageBytes  = 64 << 10
	maxPendingEnrollmentProofMessages = 4

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
	if normalizedHostname, ok := validEnrollmentHostname(hostname, true); !ok {
		servicehttp.WriteError(w, http.StatusBadRequest, "hostname must be valid UTF-8, at most 253 bytes, and contain no control characters")
		return
	} else {
		hostname = normalizedHostname
	}

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

	wsConn, err := shared.UpgradeWebSocket(&d.AgentWebSocketUpgrader, w, r, nil)
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

	if err := d.PendingAgents.TryAdd(agent, maxPendingEnrollmentAgents, maxPendingEnrollmentPerIP); err != nil {
		closeReason := "pending enrollment capacity reached"
		if errors.Is(err, ErrPendingPerIPCapacityReached) {
			closeReason = "too many pending enrollment connections from this source"
		}
		_ = wsConn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, closeReason),
			time.Now().Add(agentmgr.AgentWriteDeadline))
		_ = wsConn.Close()
		return
	}
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
		d.PendingAgents.RemoveIfMatch(tempAssetID, wsConn)
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
	proofMessages := 0
	proofAccepted := false
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

		proofMessages++
		if proofMessages > maxPendingEnrollmentProofMessages || proofAccepted {
			closePendingEnrollmentAgent(agent, websocket.ClosePolicyViolation, "enrollment proof already completed")
			return
		}

		var msg agentmgr.Message
		if err := json.Unmarshal(payload, &msg); err != nil {
			if proofMessages >= maxPendingEnrollmentProofMessages {
				closePendingEnrollmentAgent(agent, websocket.ClosePolicyViolation, "invalid enrollment proof budget exhausted")
				return
			}
			continue
		}

		if msg.Type != agentmgr.MsgEnrollmentProof {
			closePendingEnrollmentAgent(agent, websocket.ClosePolicyViolation, "unexpected pending enrollment message")
			return
		}
		if err := d.VerifyPendingEnrollmentProof(agent, msg); err != nil {
			// Do not log attacker-controlled proof fields or decoder errors. The
			// server-generated asset ID is sufficient for bounded diagnostics.
			securityruntime.Logf("enrollment: invalid proof asset_id=%s", tempAssetID)
			if proofMessages >= maxPendingEnrollmentProofMessages {
				closePendingEnrollmentAgent(agent, websocket.ClosePolicyViolation, "invalid enrollment proof budget exhausted")
				return
			}
			continue
		}
		proofAccepted = true
		info, _ := d.PendingAgents.Get(tempAssetID)
		securityruntime.Logf("enrollment: pending identity verified hostname=%s asset_id=%s fp=%s",
			hostname, tempAssetID, info.DeviceFingerprint)
		d.broadcastEvent("enrollment.pending_verified", map[string]any{
			"asset_id":           tempAssetID,
			"hostname":           hostname,
			"device_fingerprint": info.DeviceFingerprint,
			"identity_verified":  true,
		})
	}
}

// handleListPendingAgents returns the list of agents waiting for enrollment approval.
// GET /api/v1/agents/pending
func (d *Deps) HandleListPendingAgents(w http.ResponseWriter, r *http.Request) {
	if denyAssetRestrictedEnrollment(w, r) {
		return
	}
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
	if denyAssetRestrictedEnrollment(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	assetID, ok := DecodePendingEnrollmentAssetID(w, r)
	if !ok {
		return
	}

	claim, err := d.PendingAgents.ClaimDecision(assetID, true)
	if err != nil {
		switch {
		case errors.Is(err, ErrPendingAgentNotFound):
			servicehttp.WriteError(w, http.StatusNotFound, "pending agent not found")
		case errors.Is(err, ErrPendingAgentIdentityUnproven):
			servicehttp.WriteError(w, http.StatusConflict, "pending agent identity is not verified yet")
		default:
			servicehttp.WriteError(w, http.StatusConflict, "pending agent decision already in progress")
		}
		return
	}

	if d.EnrollmentTransactions == nil {
		if !d.PendingAgents.ReleaseDecision(claim) {
			log.Printf("enrollment: lost pending decision ownership while enrollment store was unavailable asset_id=%s", assetID) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		}
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "enrollment store not available")
		return
	}

	stableAssetID := ResolveApprovedAssetID(claim.Info, assetID)

	// Generate a per-agent token.
	rawToken, hashedToken, err := auth.GenerateSessionToken()
	if err != nil {
		if !d.PendingAgents.ReleaseDecision(claim) {
			log.Printf("enrollment: lost pending decision ownership after token generation failure asset_id=%s", assetID) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		}
		log.Printf("enrollment: failed to generate agent token for %s: %v", stableAssetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to generate agent token")
		return
	}
	expiresAt := NewAgentTokenExpiry(time.Now().UTC())

	prepared, err := d.EnrollmentTransactions.PrepareAgentApproval(r.Context(), persistence.AgentApprovalPrepareRequest{
		AssetID:                stableAssetID,
		AgentTokenHash:         hashedToken,
		PreparedTokenExpiresAt: time.Now().UTC().Add(pendingApprovalDeliveryTTL),
		MaxEnrolledAgents:      d.MaxEnrolledAgents,
	})
	if err != nil {
		if !d.PendingAgents.ReleaseDecision(claim) {
			log.Printf("enrollment: lost pending decision ownership after token prepare failure asset_id=%s", assetID) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		}
		log.Printf("enrollment: failed to prepare agent token for %s: %v", stableAssetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		if errors.Is(err, persistence.ErrAgentApprovalAssetConflict) {
			servicehttp.WriteError(w, http.StatusConflict, "an enrolled asset already uses this hostname; use identity recovery instead")
			return
		}
		if errors.Is(err, persistence.ErrAgentFleetCapacityReached) {
			servicehttp.WriteError(w, http.StatusConflict, "agent fleet capacity reached; decommission an enrolled agent or raise the configured limit")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to prepare agent token")
		return
	}
	cancelPrepared := func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if cancelErr := d.EnrollmentTransactions.CancelAgentApproval(cleanupCtx, prepared.ID); cancelErr != nil {
			log.Printf("enrollment: failed to cancel prepared token %s: %v", prepared.ID, cancelErr) // #nosec G706 -- Token IDs are hub-generated identifiers.
		}
	}

	// Send the approval message with token and final asset ID over WebSocket.
	if err := SendPendingEnrollmentClaimDecision(claim, agentmgr.MsgEnrollmentApproved, agentmgr.EnrollmentApprovedData{
		Token:   rawToken,
		AssetID: stableAssetID,
	}, ""); err != nil {
		cancelPrepared()
		if !d.PendingAgents.ReleaseDecision(claim) {
			log.Printf("enrollment: lost pending decision ownership after approval delivery failure asset_id=%s", assetID) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		}
		log.Printf("enrollment: failed to send approval to %s: %v", stableAssetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to deliver approval to agent")
		return
	}

	// Delivery is irreversible from the agent's perspective: it has already
	// adopted the raw token. Finalization therefore uses an independent bounded
	// context so an operator HTTP cancellation cannot strand an invalid bearer.
	finalizeCtx, finalizeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer finalizeCancel()
	if _, err := d.EnrollmentTransactions.FinalizeAgentApproval(finalizeCtx, persistence.AgentApprovalFinalizeRequest{
		PreparedTokenID:     prepared.ID,
		AssetID:             stableAssetID,
		Hostname:            claim.Info.Hostname,
		Platform:            claim.Info.Platform,
		DeviceFingerprint:   claim.Info.DeviceFingerprint,
		DeviceKeyAlgorithm:  claim.Info.DeviceKeyAlg,
		AgentTokenExpiresAt: expiresAt,
	}); err != nil {
		cancelPrepared()
		_ = claim.conn.Close()
		if !d.PendingAgents.CompleteDecision(claim) {
			log.Printf("enrollment: lost pending decision ownership after approval finalization failure asset_id=%s", assetID) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		}
		log.Printf("enrollment: failed to finalize approval for %s: %v", stableAssetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to finalize agent approval")
		return
	}

	if !d.PendingAgents.CompleteDecision(claim) {
		log.Printf("enrollment: approval finalized after pending session ownership was lost asset_id=%s", assetID) // #nosec G706 -- Asset IDs are hub-generated identifiers.
	}
	if err := ClosePendingEnrollmentClaim(claim, "enrollment approved"); err != nil {
		log.Printf("enrollment: approval finalized but pending socket close failed asset_id=%s: %v", assetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
	}
	log.Printf("enrollment: approved pending agent hostname=%s stable_asset_id=%s", claim.Info.Hostname, stableAssetID) // #nosec G706 -- Hostname and asset ID are bounded enrollment fields already persisted by the hub.

	d.broadcastEvent("enrollment.approved", map[string]any{
		"asset_id": stableAssetID,
		"hostname": claim.Info.Hostname,
	})

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":             "approved",
		"asset_id":           stableAssetID,
		"device_fingerprint": claim.Info.DeviceFingerprint,
		"identity_verified":  true,
	})
}

// handleRejectAgent rejects a pending agent enrollment, notifies it via
// WebSocket, and closes the connection.
// POST /api/v1/agents/reject
func (d *Deps) HandleRejectAgent(w http.ResponseWriter, r *http.Request) {
	if denyAssetRestrictedEnrollment(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	assetID, ok := DecodePendingEnrollmentAssetID(w, r)
	if !ok {
		return
	}

	claim, err := d.PendingAgents.ClaimDecision(assetID, false)
	if err != nil {
		if errors.Is(err, ErrPendingAgentNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "pending agent not found")
		} else {
			servicehttp.WriteError(w, http.StatusConflict, "pending agent decision already in progress")
		}
		return
	}

	// Send rejection message before closing the connection.
	if err := SendPendingEnrollmentClaimDecision(claim, agentmgr.MsgEnrollmentRejected, agentmgr.EnrollmentRejectedData{
		Reason: "rejected by operator",
	}, "enrollment rejected"); err != nil {
		if !d.PendingAgents.ReleaseDecision(claim) {
			log.Printf("enrollment: lost pending decision ownership after rejection delivery failure asset_id=%s", assetID) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		}
		log.Printf("enrollment: failed to send rejection to %s: %v", assetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to deliver rejection to agent")
		return
	}

	if !d.PendingAgents.CompleteDecision(claim) {
		log.Printf("enrollment: rejection delivered after pending session ownership was lost asset_id=%s", assetID) // #nosec G706 -- Asset IDs are hub-generated identifiers.
		servicehttp.WriteError(w, http.StatusConflict, "pending session ended during rejection")
		return
	}
	log.Printf("enrollment: rejected pending agent hostname=%s asset_id=%s", claim.Info.Hostname, assetID) // #nosec G706 -- Hostname and asset ID are bounded enrollment fields already persisted by the hub.

	d.broadcastEvent("enrollment.rejected", map[string]any{
		"asset_id": assetID,
		"hostname": claim.Info.Hostname,
	})

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "rejected",
	})
}
