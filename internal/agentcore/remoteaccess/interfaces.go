package remoteaccess

import "github.com/labtether/labtether/internal/agentmgr"

// MessageSender is the interface for sending messages back to the hub.
// Satisfied by *wsTransport in root agentcore.
type MessageSender interface {
	Send(msg agentmgr.Message) error
}

// SettingsProvider returns the current agent settings map.
// Satisfied by *Runtime in root agentcore.
type SettingsProvider interface {
	ReportedAgentSettings() map[string]string
}
