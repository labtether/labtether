package agents

import (
	"errors"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestDisconnectRevokedAgentConnectionClosesMatchingGeneration(t *testing.T) {
	manager, oldConn, _ := newDockerEndpointTestAgentConn(t, "recovered-node", "linux")
	oldConn.SetMeta("auth.agent_token_id", "old-token")

	deps := Deps{AgentMgr: manager}
	deps.disconnectRevokedAgentConnection("recovered-node", []string{"other-old-token", "old-token"})

	if manager.IsConnected("recovered-node") {
		t.Fatal("matching revoked connection remains registered")
	}
	if err := oldConn.ValidateCredential(); !errors.Is(err, agentmgr.ErrAgentCredentialRejected) {
		t.Fatalf("closed connection validation error=%v, want immediate rejection", err)
	}
}

func TestDisconnectRevokedAgentConnectionKeepsNewerGeneration(t *testing.T) {
	manager, oldConn, _ := newDockerEndpointTestAgentConn(t, "recovered-node", "linux")
	oldConn.SetMeta("auth.agent_token_id", "old-token")

	_, newerConn, _ := newDockerEndpointTestAgentConn(t, "recovered-node", "linux")
	newerConn.SetMeta("auth.agent_token_id", "new-token")
	manager.Register(newerConn)

	deps := Deps{AgentMgr: manager}
	deps.disconnectRevokedAgentConnection("recovered-node", []string{"old-token"})

	got, ok := manager.Get("recovered-node")
	if !ok || got != newerConn {
		t.Fatal("cleanup removed the newer connection generation")
	}
	if err := newerConn.ValidateCredential(); err != nil {
		t.Fatalf("newer connection was rejected: %v", err)
	}
}
