package servicehttp

import "net/http"

// AgentTimeoutMessage is the whole answer for a request whose device agent
// did not reply in time. It names no host, request or internal detail.
const AgentTimeoutMessage = "device did not answer in time"

// WriteAgentTimeout answers 504 with AgentTimeoutMessage. WriteError hides
// every 5xx text; this writer takes no text of its own, so it cannot carry
// anything but these fixed words.
func WriteAgentTimeout(w http.ResponseWriter) {
	WriteJSON(w, http.StatusGatewayTimeout, map[string]any{"error": AgentTimeoutMessage})
}
