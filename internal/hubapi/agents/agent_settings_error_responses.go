package agents

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/agentsettings"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

func writeAgentSettingsInternalError(w http.ResponseWriter, clientMessage string, err error) {
	securityruntime.Logf("agent settings: %s: %v", clientMessage, err)
	servicehttp.WriteError(w, http.StatusInternalServerError, clientMessage)
}

// safeAgentSettingsApplyError only preserves bounded, expected agent responses.
// Arbitrary agent-provided text is not reflected into later API responses.
func safeAgentSettingsApplyError(raw string) string {
	switch strings.TrimSpace(raw) {
	case "":
		return ""
	case "runtime unavailable":
		return "agent runtime unavailable"
	case "remote overrides are disabled on this agent":
		return "remote overrides are disabled on this agent"
	case "fingerprint mismatch for settings apply":
		return "agent identity changed; reconnect the agent before applying settings"
	}

	for _, definition := range agentsettings.AgentSettingDefinitions() {
		if strings.TrimSpace(raw) == "setting "+definition.Key+" is local-only" {
			return "setting " + definition.Key + " is local-only"
		}
	}
	return "agent failed to apply settings"
}
