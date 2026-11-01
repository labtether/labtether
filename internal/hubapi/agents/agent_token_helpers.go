package agents

import (
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

func ConfiguredAgentTokenTTLHours() int {
	return shared.ConfiguredAgentTokenTTLHours()
}

func NewAgentTokenExpiry(now time.Time) time.Time {
	return now.UTC().Add(shared.AgentTokenTTL())
}
