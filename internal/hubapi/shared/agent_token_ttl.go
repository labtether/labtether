package shared

import "time"

const (
	defaultAgentTokenTTLHours = 30 * 24
	maxAgentTokenTTLHours     = 365 * 24
)

// ConfiguredAgentTokenTTLHours is how long an agent credential stays valid
// after it is issued or after the agent's last authenticated heartbeat.
func ConfiguredAgentTokenTTLHours() int {
	ttlHours := EnvOrDefaultInt("LABTETHER_AGENT_TOKEN_TTL_HOURS", defaultAgentTokenTTLHours)
	if ttlHours < 1 {
		ttlHours = 1
	}
	if ttlHours > maxAgentTokenTTLHours {
		return maxAgentTokenTTLHours
	}
	return ttlHours
}

// AgentTokenTTL returns ConfiguredAgentTokenTTLHours as a duration. Each
// authenticated heartbeat moves an active credential's expiry this far ahead,
// so only an agent that stays offline longer than this has to re-enroll.
func AgentTokenTTL() time.Duration {
	return time.Duration(ConfiguredAgentTokenTTLHours()) * time.Hour
}
