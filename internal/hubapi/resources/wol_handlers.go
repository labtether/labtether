package resources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/wol"
)

var SendWakeOnLAN = wol.Send

const (
	wakeRateLimitBucket = "assets.wake"
	wakeRateLimitCount  = 12
	wakeRateLimitWindow = time.Minute
	maxWoLRelayAttempts = 3
	maxPendingWoLRelay  = 1024
	maxWoLAuditWindows  = 2048
	wolRelayResultTTL   = 30 * time.Second
	wolAuditThrottleTTL = time.Minute
)

type wolRelayDispatch struct {
	RequestID string
	RelayID   string
}

type pendingWoLRelay struct {
	ActorID     string
	TargetID    string
	RelayID     string
	ExpectedMAC string
	RequestRef  string
	ExpiresAt   time.Time
}

type wolPendingConsumeStatus uint8

const (
	wolPendingConsumed wolPendingConsumeStatus = iota
	wolPendingUnknown
	wolPendingExpired
	wolPendingRelayMismatch
	wolPendingMACMismatch
)

// WoLPendingRegistry is a bounded, process-local registry for agent-assisted
// Wake-on-LAN requests. It prevents unsolicited, replayed, cross-relay, and
// wrong-target results from being promoted into successful audit outcomes.
// The zero value is ready for use.
type WoLPendingRegistry struct {
	mu           sync.Mutex
	entries      map[string]pendingWoLRelay
	auditWindows map[string]time.Time
}

func (r *WoLPendingRegistry) store(requestID string, pending pendingWoLRelay) bool {
	if r == nil || strings.TrimSpace(requestID) == "" {
		return false
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneExpiredLocked(now)
	if r.entries == nil {
		r.entries = make(map[string]pendingWoLRelay, 16)
	}
	if len(r.entries) >= maxPendingWoLRelay {
		return false
	}
	if _, exists := r.entries[requestID]; exists {
		return false
	}
	r.entries[requestID] = pending
	return true
}

func (r *WoLPendingRegistry) delete(requestID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.entries, requestID)
	r.mu.Unlock()
}

func (r *WoLPendingRegistry) refreshExpiry(requestID string, expiresAt time.Time) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, ok := r.entries[requestID]
	if !ok {
		return false
	}
	pending.ExpiresAt = expiresAt
	r.entries[requestID] = pending
	return true
}

func (r *WoLPendingRegistry) consume(requestID, relayID, canonicalMAC string) (pendingWoLRelay, wolPendingConsumeStatus) {
	if r == nil {
		return pendingWoLRelay{}, wolPendingUnknown
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, ok := r.entries[requestID]
	if !ok {
		return pendingWoLRelay{}, wolPendingUnknown
	}
	if !pending.ExpiresAt.After(now) {
		delete(r.entries, requestID)
		return pending, wolPendingExpired
	}
	if !strings.EqualFold(strings.TrimSpace(pending.RelayID), strings.TrimSpace(relayID)) {
		return pending, wolPendingRelayMismatch
	}
	if !strings.EqualFold(pending.ExpectedMAC, canonicalMAC) {
		return pending, wolPendingMACMismatch
	}
	delete(r.entries, requestID)
	return pending, wolPendingConsumed
}

func (r *WoLPendingRegistry) expire(requestID string) (pendingWoLRelay, bool) {
	if r == nil {
		return pendingWoLRelay{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, ok := r.entries[requestID]
	if ok {
		delete(r.entries, requestID)
	}
	return pending, ok
}

func (r *WoLPendingRegistry) pruneExpiredLocked(now time.Time) {
	for requestID, pending := range r.entries {
		if !pending.ExpiresAt.After(now) {
			delete(r.entries, requestID)
		}
	}
}

func (r *WoLPendingRegistry) allowAudit(key string, now time.Time) bool {
	if r == nil || strings.TrimSpace(key) == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.auditWindows == nil {
		r.auditWindows = make(map[string]time.Time, 16)
	}
	for candidate, expiresAt := range r.auditWindows {
		if !expiresAt.After(now) {
			delete(r.auditWindows, candidate)
		}
	}
	if expiresAt, exists := r.auditWindows[key]; exists && expiresAt.After(now) {
		return false
	}
	if len(r.auditWindows) >= maxWoLAuditWindows {
		var oldestKey string
		var oldestExpiry time.Time
		for candidate, expiresAt := range r.auditWindows {
			if oldestExpiry.IsZero() || expiresAt.Before(oldestExpiry) {
				oldestKey = candidate
				oldestExpiry = expiresAt
			}
		}
		delete(r.auditWindows, oldestKey)
	}
	r.auditWindows[key] = now.Add(wolAuditThrottleTTL)
	return true
}

func (d *Deps) HandleWakeOnLAN(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.enforceAssetActionGuard(w, assetID) {
		d.appendWakeRequestAudit(r.Context(), "asset.wake.outcome", assetID, "denied", "maintenance_blocked", nil)
		return
	}

	if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, wakeRateLimitBucket, wakeRateLimitCount, wakeRateLimitWindow) {
		d.appendRateLimitedWakeAudit(r.Context(), assetID)
		return
	}
	d.appendWakeRequestAudit(r.Context(), "asset.wake.requested", assetID, "attempted", "", nil)
	if d.AssetStore == nil {
		d.appendWakeRequestAudit(r.Context(), "asset.wake.outcome", assetID, "failed", "asset_store_unavailable", nil)
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset inventory is unavailable")
		return
	}

	assetEntry, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		d.appendWakeRequestAudit(r.Context(), "asset.wake.outcome", assetID, "failed", "asset_lookup_failed", nil)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load asset")
		return
	}
	if !ok {
		d.appendWakeRequestAudit(r.Context(), "asset.wake.outcome", assetID, "denied", "asset_not_found", nil)
		servicehttp.WriteError(w, http.StatusNotFound, "asset not found")
		return
	}

	macStr := FindAssetMAC(assetEntry.Metadata)
	if macStr == "" {
		d.appendWakeRequestAudit(r.Context(), "asset.wake.outcome", assetID, "denied", "mac_missing", nil)
		servicehttp.WriteError(w, http.StatusUnprocessableEntity, "no MAC address known for this asset; add a MAC address to its network metadata")
		return
	}
	mac, err := wol.ParseMAC(macStr)
	if err != nil {
		d.appendWakeRequestAudit(r.Context(), "asset.wake.outcome", assetID, "denied", "mac_invalid", nil)
		servicehttp.WriteError(w, http.StatusUnprocessableEntity, "the asset has an invalid MAC address; update its network metadata")
		return
	}

	dispatch, relayAttempts := d.tryAgentAssistedWoL(r.Context(), assetEntry, mac.String())
	if dispatch.RelayID != "" {
		d.appendWakeRequestAudit(r.Context(), "asset.wake.outcome", assetEntry.ID, "queued", "", map[string]any{
			"method":         "agent-assisted",
			"relay_asset_id": dispatch.RelayID,
			"relay_attempts": relayAttempts,
			"request_ref":    wolRequestReference(dispatch.RequestID),
		})
		servicehttp.WriteJSON(w, http.StatusAccepted, map[string]string{
			"status": "queued",
			"method": "agent-assisted",
			"mac":    macStr,
		})
		return
	}

	// Fallback to hub-side broadcast.
	broadcastAddr := "255.255.255.255:9"
	if err := SendWakeOnLAN(mac, broadcastAddr); err != nil {
		securityruntime.Logf("wol: direct send failed asset=%s reason=udp_broadcast_unavailable", assetEntry.ID)
		d.appendWakeRequestAudit(r.Context(), "asset.wake.outcome", assetEntry.ID, "failed", "direct_send_failed", map[string]any{
			"method":         "direct",
			"relay_attempts": relayAttempts,
		})
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send a Wake-on-LAN packet from the hub; verify UDP broadcast access or configure an online relay agent")
		return
	}

	securityruntime.Logf("wol: direct magic packet sent asset=%s", assetEntry.ID)
	d.appendWakeRequestAudit(r.Context(), "asset.wake.outcome", assetEntry.ID, "succeeded", "", map[string]any{
		"method":         "direct",
		"relay_attempts": relayAttempts,
	})
	servicehttp.WriteJSON(w, http.StatusAccepted, map[string]string{
		"status": "sent",
		"method": "direct",
		"mac":    macStr,
	})
}

func FindAssetMAC(metadata map[string]string) string {
	candidates := []string{
		strings.TrimSpace(metadata["mac_address"]),
		strings.TrimSpace(metadata["mac"]),
		strings.TrimSpace(metadata["primary_mac"]),
		strings.TrimSpace(metadata["guest_primary_mac"]),
	}
	if list := strings.TrimSpace(metadata["guest_mac_addresses"]); list != "" {
		for _, candidate := range strings.Split(list, ",") {
			candidates = append(candidates, strings.TrimSpace(candidate))
		}
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := wol.ParseMAC(candidate); err == nil {
			return candidate
		}
	}

	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func (d *Deps) tryAgentAssistedWoL(ctx context.Context, target assets.Asset, macAddr string) (wolRelayDispatch, int) {
	if d.AgentMgr == nil || d.AssetStore == nil || d.WoLPending == nil {
		return wolRelayDispatch{}, 0
	}

	actorID := ""
	if d.PrincipalActorID != nil {
		actorID = d.PrincipalActorID(ctx)
	}
	attempts := 0
	for _, relay := range d.EligibleWoLRelays(target.ID, target) {
		if attempts >= maxWoLRelayAttempts {
			break
		}
		attempts++
		connectedID := relay.AssetID
		requestID := idgen.New("wolreq")
		requestRef := wolRequestReference(requestID)
		pending := pendingWoLRelay{
			ActorID:     actorID,
			TargetID:    target.ID,
			RelayID:     connectedID,
			ExpectedMAC: macAddr,
			RequestRef:  requestRef,
			ExpiresAt:   time.Now().UTC().Add(wolRelayResultTTL + agentmgr.AgentWriteDeadline),
		}
		if !d.WoLPending.store(requestID, pending) {
			securityruntime.Logf("wol: agent-assisted dispatch skipped target=%s reason=pending_registry_full", target.ID)
			d.appendWakeRequestAudit(ctx, "asset.wake.relay_dispatch", target.ID, "failed", "pending_registry_full", map[string]any{
				"relay_asset_id": connectedID,
				"attempt":        attempts,
			})
			break
		}

		payload, _ := json.Marshal(agentmgr.WoLSendData{
			RequestID: requestID,
			MAC:       macAddr,
			Broadcast: "255.255.255.255:9",
		})
		err := d.AgentMgr.SendToAgent(connectedID, agentmgr.Message{
			Type: agentmgr.MsgWoLSend,
			ID:   target.ID,
			Data: payload,
		})
		if err != nil {
			d.WoLPending.delete(requestID)
			securityruntime.Logf("wol: agent-assisted dispatch failed relay=%s target=%s reason=relay_unavailable", connectedID, target.ID)
			d.appendWakeRequestAudit(ctx, "asset.wake.relay_dispatch", target.ID, "failed", "relay_unavailable", map[string]any{
				"relay_asset_id": connectedID,
				"request_ref":    requestRef,
				"attempt":        attempts,
			})
			continue
		}
		if d.WoLPending.refreshExpiry(requestID, time.Now().UTC().Add(wolRelayResultTTL)) {
			time.AfterFunc(wolRelayResultTTL, func() {
				if expired, ok := d.WoLPending.expire(requestID); ok {
					d.appendWakeAuditWithActor("asset.wake.relay_result", expired.ActorID, expired.TargetID, "failed", "relay_result_timeout", map[string]any{
						"method":            "agent-assisted",
						"relay_asset_id":    expired.RelayID,
						"request_ref":       expired.RequestRef,
						"reported_by_agent": false,
					})
				}
			})
		}
		securityruntime.Logf("wol: agent-assisted packet queued relay=%s target=%s request_ref=%s", connectedID, target.ID, requestRef)
		return wolRelayDispatch{RequestID: requestID, RelayID: connectedID}, attempts
	}
	return wolRelayDispatch{}, attempts
}

type WoLRelayCandidate struct {
	AssetID  string
	Platform string
}

func (d *Deps) EligibleWoLRelays(targetAssetID string, target assets.Asset) []WoLRelayCandidate {
	targetGroupID := strings.TrimSpace(target.GroupID)
	candidates := make([]WoLRelayCandidate, 0)
	for _, connectedID := range d.AgentMgr.ConnectedAssets() {
		if connectedID == targetAssetID {
			continue
		}
		agentAsset, aok, aerr := d.AssetStore.GetAsset(connectedID)
		if aerr != nil || !aok {
			continue
		}
		if targetGroupID != "" && !strings.EqualFold(strings.TrimSpace(agentAsset.GroupID), targetGroupID) {
			continue
		}
		candidates = append(candidates, WoLRelayCandidate{
			AssetID:  connectedID,
			Platform: strings.TrimSpace(agentAsset.Platform),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		leftPriority := WoLRelayPlatformPriority(candidates[i].Platform)
		rightPriority := WoLRelayPlatformPriority(candidates[j].Platform)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}

		leftID := strings.ToLower(strings.TrimSpace(candidates[i].AssetID))
		rightID := strings.ToLower(strings.TrimSpace(candidates[j].AssetID))
		if leftID != rightID {
			return leftID < rightID
		}
		return strings.TrimSpace(candidates[i].AssetID) < strings.TrimSpace(candidates[j].AssetID)
	})

	return candidates
}

func WoLRelayPlatformPriority(platform string) int {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "linux":
		return 0
	case "":
		return 2
	default:
		return 1
	}
}

func (d *Deps) ProcessAgentWoLResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	relayID := "unknown"
	if conn != nil && strings.TrimSpace(conn.AssetID) != "" {
		relayID = strings.TrimSpace(conn.AssetID)
	}

	var result agentmgr.WoLResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		securityruntime.Logf("wol: invalid wol.result payload relay=%s reason=invalid_payload", relayID)
		d.appendAgentWakeResultAudit(relayID, "rejected", "invalid_payload", "")
		return
	}

	requestID := strings.TrimSpace(result.RequestID)
	messageRequestID := strings.TrimSpace(msg.ID)
	if requestID == "" || messageRequestID == "" || messageRequestID != requestID {
		securityruntime.Logf("wol: invalid wol.result request id relay=%s reason=request_id_mismatch", relayID)
		d.appendAgentWakeResultAudit(relayID, "rejected", "request_id_mismatch", "")
		return
	}
	mac, err := wol.ParseMAC(result.MAC)
	if err != nil {
		securityruntime.Logf("wol: invalid wol.result MAC relay=%s request_ref=%s", relayID, wolRequestReference(requestID))
		d.appendAgentWakeResultAudit(relayID, "rejected", "invalid_mac_result", requestID)
		return
	}
	if result.OK && strings.TrimSpace(result.Error) != "" {
		securityruntime.Logf("wol: inconsistent wol.result relay=%s request_ref=%s", relayID, wolRequestReference(requestID))
		d.appendAgentWakeResultAudit(relayID, "rejected", "inconsistent_result", requestID)
		return
	}
	if d.WoLPending == nil {
		securityruntime.Logf("wol: uncorrelated wol.result relay=%s request_ref=%s reason=registry_unavailable", relayID, wolRequestReference(requestID))
		d.appendAgentWakeResultAudit(relayID, "rejected", "registry_unavailable", requestID)
		return
	}

	pending, status := d.WoLPending.consume(requestID, relayID, mac.String())
	switch status {
	case wolPendingUnknown:
		securityruntime.Logf("wol: rejected wol.result relay=%s request_ref=%s reason=unknown_or_replayed_request", relayID, wolRequestReference(requestID))
		d.appendAgentWakeResultAudit(relayID, "rejected", "unknown_or_replayed_request", requestID)
		return
	case wolPendingExpired:
		securityruntime.Logf("wol: rejected wol.result relay=%s request_ref=%s reason=request_expired", relayID, pending.RequestRef)
		d.appendCorrelatedAgentWakeResultAudit(pending, relayID, "rejected", "request_expired")
		return
	case wolPendingRelayMismatch:
		securityruntime.Logf("wol: rejected wol.result relay=%s request_ref=%s reason=relay_mismatch", relayID, pending.RequestRef)
		d.appendCorrelatedAgentWakeResultAudit(pending, relayID, "rejected", "relay_mismatch")
		return
	case wolPendingMACMismatch:
		securityruntime.Logf("wol: rejected wol.result relay=%s request_ref=%s reason=mac_mismatch", relayID, pending.RequestRef)
		d.appendCorrelatedAgentWakeResultAudit(pending, relayID, "rejected", "mac_mismatch")
		return
	}

	if result.OK {
		securityruntime.Logf("wol: relay reported packet send relay=%s target=%s request_ref=%s", relayID, pending.TargetID, pending.RequestRef)
		d.appendCorrelatedAgentWakeResultAudit(pending, relayID, "succeeded", "")
		return
	}
	securityruntime.Logf("wol: relay reported packet failure relay=%s target=%s request_ref=%s reason=relay_send_failed", relayID, pending.TargetID, pending.RequestRef)
	d.appendCorrelatedAgentWakeResultAudit(pending, relayID, "failed", "relay_send_failed")
}

func (d *Deps) appendWakeRequestAudit(ctx context.Context, eventType, target, decision, reason string, details map[string]any) {
	if d == nil {
		return
	}
	actorID := ""
	if d.PrincipalActorID != nil {
		actorID = d.PrincipalActorID(ctx)
	}
	d.appendWakeAuditWithActor(eventType, actorID, target, decision, reason, details)
}

func (d *Deps) appendRateLimitedWakeAudit(ctx context.Context, target string) {
	if d == nil || d.WoLPending == nil {
		return
	}
	actorID := ""
	if d.PrincipalActorID != nil {
		actorID = d.PrincipalActorID(ctx)
	}
	auditKey := "rate_limited:" + wolRequestReference(actorID)
	if !d.WoLPending.allowAudit(auditKey, time.Now().UTC()) {
		return
	}
	d.appendWakeAuditWithActor("asset.wake.outcome", actorID, target, "denied", "rate_limited", map[string]any{
		"throttled": true,
	})
}

func (d *Deps) appendWakeAuditWithActor(eventType, actorID, target, decision, reason string, details map[string]any) {
	if d == nil || d.AppendAuditEventBestEffort == nil {
		return
	}
	event := audit.NewEvent(eventType)
	event.ActorID = strings.TrimSpace(actorID)
	event.Target = strings.TrimSpace(target)
	event.Decision = decision
	event.Reason = reason
	event.Details = details
	d.AppendAuditEventBestEffort(event, "api warning: failed to append Wake-on-LAN audit event")
}

func (d *Deps) appendAgentWakeResultAudit(relayID, decision, reason, requestID string) {
	if d != nil && d.WoLPending != nil {
		auditKey := "agent_result:" + wolRequestReference(strings.TrimSpace(relayID)+"\x00"+reason)
		if !d.WoLPending.allowAudit(auditKey, time.Now().UTC()) {
			return
		}
	}
	details := map[string]any{
		"method":            "agent-assisted",
		"relay_asset_id":    relayID,
		"reported_by_agent": true,
	}
	if requestID != "" {
		details["request_ref"] = wolRequestReference(requestID)
	}
	d.appendWakeAuditWithActor("asset.wake.relay_result", relayID, relayID, decision, reason, details)
}

func (d *Deps) appendCorrelatedAgentWakeResultAudit(pending pendingWoLRelay, reportingRelayID, decision, reason string) {
	d.appendWakeAuditWithActor("asset.wake.relay_result", reportingRelayID, pending.TargetID, decision, reason, map[string]any{
		"method":             "agent-assisted",
		"relay_asset_id":     pending.RelayID,
		"reporting_asset_id": reportingRelayID,
		"request_ref":        pending.RequestRef,
		"requested_by":       pending.ActorID,
		"reported_by_agent":  true,
	})
}

func wolRequestReference(requestID string) string {
	sum := sha256.Sum256([]byte(requestID))
	return hex.EncodeToString(sum[:8])
}
