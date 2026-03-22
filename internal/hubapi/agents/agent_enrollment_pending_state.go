package agents

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"fmt"
	"strings"
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

// Remove deletes a pending agent from the registry by its temporary asset ID.
func (p *PendingAgents) Remove(assetID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.agents, assetID)
}

// Get returns the pending agent for the given asset ID, if present.
func (p *PendingAgents) Get(assetID string) (*PendingAgent, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	agent, ok := p.agents[assetID]
	return agent, ok
}

// List returns a snapshot of all currently pending agents in JSON-safe form.
func (p *PendingAgents) List() []PendingAgentInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]PendingAgentInfo, 0, len(p.agents))
	for _, a := range p.agents {
		result = append(result, PendingAgentInfo{
			AssetID:            a.AssetID,
			Hostname:           a.Hostname,
			Platform:           a.Platform,
			RemoteIP:           a.RemoteIP,
			ConnectedAt:        a.ConnectedAt,
			DeviceFingerprint:  a.DeviceFingerprint,
			DeviceKeyAlg:       a.DeviceKeyAlg,
			IdentityVerified:   a.IdentityVerified,
			IdentityVerifiedAt: a.IdentityVerifiedAt,
		})
	}
	return result
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
