package servicehttp

import "net/http"

// AgentUnsupportedMessage is the whole answer for a device request the
// device's agent did not advertise. The Hub sends nothing to the agent.
const AgentUnsupportedMessage = "this device's agent does not support this"

// WriteAgentUnsupported answers 422 with AgentUnsupportedMessage, like a power
// action the agent's platform cannot do. It is not 409, so a client does not
// read it as "agent is not connected".
func WriteAgentUnsupported(w http.ResponseWriter) {
	WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": AgentUnsupportedMessage})
}
