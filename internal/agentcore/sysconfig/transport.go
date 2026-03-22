package sysconfig

import "github.com/labtether/labtether/internal/agentmgr"

// MessageSender abstracts the agent-to-hub send capability so this package
// does not depend on the concrete wsTransport type in the parent agentcore package.
type MessageSender interface {
	Send(msg agentmgr.Message) error
}
