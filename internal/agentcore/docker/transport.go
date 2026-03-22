package docker

import (
	"context"

	"github.com/labtether/labtether/internal/agentmgr"
)

// Transport abstracts the agent-to-hub communication channel so this package
// does not depend on the concrete wsTransport type in the parent agentcore package.
type Transport interface {
	Connect(ctx context.Context) error
	Send(msg agentmgr.Message) error
	Receive() (agentmgr.Message, error)
	Close()
	Connected() bool
}
