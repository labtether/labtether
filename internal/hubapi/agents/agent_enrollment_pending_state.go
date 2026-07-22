package agents

import (
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// NewPendingAgents returns an initialized, empty PendingAgents registry.
func NewPendingAgents() *PendingAgents {
	return &PendingAgents{
		agents: make(map[string]*PendingAgent),
	}
}

// Add inserts a pending agent into the registry.
func (p *PendingAgents) Add(agent *PendingAgent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.agents[agent.AssetID] = agent
}

// TryAdd atomically enforces both global and per-source hard caps while
// admitting a WebSocket that has already completed its HTTP upgrade.
func (p *PendingAgents) TryAdd(agent *PendingAgent, maxTotal, maxPerIP int) error {
	if agent == nil {
		return ErrPendingAgentNotFound
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if maxTotal > 0 && len(p.agents) >= maxTotal {
		return ErrPendingCapacityReached
	}
	needle := strings.TrimSpace(agent.RemoteIP)
	if maxPerIP > 0 && needle != "" {
		count := 0
		for _, existing := range p.agents {
			if strings.EqualFold(strings.TrimSpace(existing.RemoteIP), needle) {
				count++
			}
		}
		if count >= maxPerIP {
			return ErrPendingPerIPCapacityReached
		}
	}
	agent.Disconnected = false
	p.agents[agent.AssetID] = agent
	return nil
}

// Remove deletes a pending agent from the registry by its temporary asset ID.
func (p *PendingAgents) Remove(assetID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.agents, assetID)
}

func (p *PendingAgents) RemoveIfMatch(assetID string, conn *websocket.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	agent, ok := p.agents[assetID]
	if !ok || agent.Conn != conn {
		return false
	}
	agent.Disconnected = true
	delete(p.agents, assetID)
	return true
}

// Get returns the pending agent for the given asset ID, if present.
func (p *PendingAgents) Get(assetID string) (PendingAgentInfo, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	agent, ok := p.agents[assetID]
	if !ok {
		return PendingAgentInfo{}, false
	}
	return pendingAgentInfo(agent), true
}

// List returns a snapshot of all currently pending agents in JSON-safe form.
func (p *PendingAgents) List() []PendingAgentInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]PendingAgentInfo, 0, len(p.agents))
	for _, a := range p.agents {
		result = append(result, pendingAgentInfo(a))
	}
	return result
}

func pendingAgentInfo(agent *PendingAgent) PendingAgentInfo {
	info := PendingAgentInfo{
		AssetID:           agent.AssetID,
		Hostname:          agent.Hostname,
		Platform:          agent.Platform,
		RemoteIP:          agent.RemoteIP,
		ConnectedAt:       agent.ConnectedAt,
		DeviceFingerprint: agent.DeviceFingerprint,
		DeviceKeyAlg:      agent.DeviceKeyAlg,
		IdentityVerified:  agent.IdentityVerified,
	}
	if agent.IdentityVerifiedAt != nil {
		verifiedAt := agent.IdentityVerifiedAt.UTC()
		info.IdentityVerifiedAt = &verifiedAt
	}
	return info
}

func (p *PendingAgents) SetChallenge(assetID string, conn *websocket.Conn, nonce string, expiresAt time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	agent, ok := p.agents[assetID]
	if !ok || agent.Conn != conn || agent.Disconnected || agent.DecisionClaimed {
		return false
	}
	agent.ChallengeNonce = nonce
	agent.ChallengeExpiresAt = expiresAt.UTC()
	return true
}

type pendingChallengeSnapshot struct {
	AssetID string
	Nonce   string
	Expires time.Time
}

func (p *PendingAgents) ChallengeSnapshot(assetID string, conn *websocket.Conn) (pendingChallengeSnapshot, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	agent, ok := p.agents[assetID]
	if !ok || agent.Conn != conn || agent.Disconnected || agent.DecisionClaimed {
		return pendingChallengeSnapshot{}, false
	}
	return pendingChallengeSnapshot{AssetID: agent.AssetID, Nonce: agent.ChallengeNonce, Expires: agent.ChallengeExpiresAt}, true
}

func (p *PendingAgents) CommitIdentityProof(assetID string, conn *websocket.Conn, expectedNonce, fingerprint, keyAlgorithm, publicKey string, verifiedAt time.Time) (PendingAgentInfo, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	agent, ok := p.agents[assetID]
	if !ok || agent.Conn != conn || agent.Disconnected || agent.DecisionClaimed || agent.ChallengeNonce != expectedNonce {
		return PendingAgentInfo{}, false
	}
	agent.DeviceFingerprint = fingerprint
	agent.DeviceKeyAlg = keyAlgorithm
	agent.DevicePublicKey = publicKey
	agent.IdentityVerified = true
	verifiedAt = verifiedAt.UTC()
	agent.IdentityVerifiedAt = &verifiedAt
	agent.ChallengeNonce = ""
	agent.ChallengeExpiresAt = time.Time{}
	return pendingAgentInfo(agent), true
}

func (p *PendingAgents) ClaimDecision(assetID string, requireVerified bool) (PendingDecisionClaim, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	agent, ok := p.agents[assetID]
	if !ok || agent.Disconnected {
		return PendingDecisionClaim{}, ErrPendingAgentNotFound
	}
	if agent.DecisionClaimed {
		return PendingDecisionClaim{}, ErrPendingAgentAlreadyClaimed
	}
	if requireVerified && !agent.IdentityVerified {
		return PendingDecisionClaim{}, ErrPendingAgentIdentityUnproven
	}
	agent.DecisionClaimed = true
	agent.ClaimVersion++
	return PendingDecisionClaim{
		Info:         pendingAgentInfo(agent),
		conn:         agent.Conn,
		connMu:       &agent.ConnMu,
		record:       agent,
		claimVersion: agent.ClaimVersion,
	}, nil
}

func (p *PendingAgents) ReleaseDecision(claim PendingDecisionClaim) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	agent, ok := p.agents[claim.Info.AssetID]
	if !ok || agent != claim.record || agent.Conn != claim.conn || agent.Disconnected || agent.ClaimVersion != claim.claimVersion {
		return false
	}
	agent.DecisionClaimed = false
	return true
}

func (p *PendingAgents) CompleteDecision(claim PendingDecisionClaim) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	agent, ok := p.agents[claim.Info.AssetID]
	if !ok || agent != claim.record || agent.Conn != claim.conn || agent.ClaimVersion != claim.claimVersion {
		return false
	}
	agent.Disconnected = true
	delete(p.agents, claim.Info.AssetID)
	return true
}

// IsIdentityVerified returns whether the pending agent with the given asset ID
// has been identity-verified. Returns false if the agent is not found.
func (p *PendingAgents) IsIdentityVerified(assetID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	agent, ok := p.agents[assetID]
	return ok && agent.IdentityVerified
}

// Count returns the number of currently pending enrollment connections.
func (p *PendingAgents) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.agents)
}

// CountByRemoteIP returns the number of pending connections for a single client IP.
func (p *PendingAgents) CountByRemoteIP(remoteIP string) int {
	needle := strings.TrimSpace(remoteIP)
	if needle == "" {
		return 0
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	count := 0
	for _, agent := range p.agents {
		if strings.EqualFold(strings.TrimSpace(agent.RemoteIP), needle) {
			count++
		}
	}
	return count
}

func SanitizePendingIdentityHeader(raw string, maxLen int) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if maxLen > 0 && len(value) > maxLen {
		return ""
	}
	return value
}

func BuildPendingEnrollmentAssetID(hostname string) string {
	hostComponent := strings.ToLower(strings.TrimSpace(hostname))
	if hostComponent == "" {
		hostComponent = "unknown"
	}
	if len(hostComponent) > maxPendingHostnameIDLen {
		hostComponent = hostComponent[:maxPendingHostnameIDLen]
	}

	var b strings.Builder
	b.Grow(len(hostComponent))
	for _, ch := range hostComponent {
		switch {
		case ch >= 'a' && ch <= 'z':
			b.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch == '-' || ch == '_' || ch == '.':
			b.WriteRune(ch)
		default:
			b.WriteByte('-')
		}
	}

	normalizedHost := strings.Trim(b.String(), "-")
	if normalizedHost == "" {
		normalizedHost = "unknown"
	}

	return fmt.Sprintf("pending-%s-%s", normalizedHost, shared.GenerateRequestID())
}
