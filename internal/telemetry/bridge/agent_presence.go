package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// AgentPresenceSource provides agent connection and heartbeat metrics.
type AgentPresenceSource interface {
	// AllAgentPresenceMetrics returns presence metrics for all known agents.
	AllAgentPresenceMetrics() []AgentPresenceEntry
}

// AgentPresenceEntry holds per-agent connection state and heartbeat age.
type AgentPresenceEntry struct {
	AssetID             string
	Connected           float64 // 0 or 1
	LastHeartbeatAgeSec float64 // seconds since last heartbeat
	Labels              map[string]string
}

// AgentPresenceBridge is a MetricsBridge that reads agent presence data from
// an AgentPresenceSource and converts it to MetricSample objects.
type AgentPresenceBridge struct {
	source AgentPresenceSource
}

// NewAgentPresenceBridge creates an AgentPresenceBridge backed by the given source.
func NewAgentPresenceBridge(source AgentPresenceSource) *AgentPresenceBridge {
	return &AgentPresenceBridge{source: source}
}

// Name returns the bridge identifier.
func (b *AgentPresenceBridge) Name() string { return "agent-presence" }

// Interval returns how often this bridge should be collected.
func (b *AgentPresenceBridge) Interval() time.Duration { return 30 * time.Second }

// Collect iterates all agents from the source and produces 2 MetricSamples
// per agent: agent_connected and agent_last_heartbeat_age_seconds.
func (b *AgentPresenceBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllAgentPresenceMetrics()
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	out := make([]telemetry.MetricSample, 0, len(entries)*2)

	for _, e := range entries {
		labels := e.Labels

		out = append(out,
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricAgentConnected,
				Unit:        "bool",
				Value:       e.Connected,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricAgentLastHeartbeatAgeSeconds,
				Unit:        "seconds",
				Value:       e.LastHeartbeatAgeSec,
				CollectedAt: now,
				Labels:      labels,
			},
		)
	}

	return out
}
