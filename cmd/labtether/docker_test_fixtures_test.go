package main

import (
	"encoding/json"

	"github.com/labtether/labtether/internal/agentmgr"
)

// The Docker handler tests moved to internal/hubapi/dockerpkg. This is a
// byte-for-byte copy of their helper, for the cmd tests that still use it.

// makeDockerDiscovery constructs a discovery message from the given data.
func makeDockerDiscovery(data agentmgr.DockerDiscoveryData) agentmgr.Message {
	raw, _ := json.Marshal(data)
	return agentmgr.Message{Type: agentmgr.MsgDockerDiscovery, Data: raw}
}
