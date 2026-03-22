package agents

import (
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

const (
	defaultAgentTokenTTLHours = 30 * 24
	maxAgentTokenTTLHours     = 365 * 24
)

func ConfiguredAgentTokenTTLHours() int {
	ttlHours := shared.EnvOrDefaultInt("LABTETHER_AGENT_TOKEN_TTL_HOURS", defaultAgentTokenTTLHours)
	if ttlHours < 1 {
		ttlHours = 1
	}
	if ttlHours > maxAgentTokenTTLHours {
		return maxAgentTokenTTLHours
	}
	return ttlHours
}

func NewAgentTokenExpiry(now time.Time) time.Time {
	return now.UTC().Add(time.Duration(ConfiguredAgentTokenTTLHours()) * time.Hour)
}
