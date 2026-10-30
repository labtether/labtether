package resources

import (
	"context"
	"errors"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
)

// A delete refused because the agent is still connected must not first tell
// that agent to remove the Hub SSH key.
func TestDeleteAssetRefusedForConnectedAgentSendsNothing(t *testing.T) {
	manager := agentmgr.NewManager()
	manager.Register(agentmgr.NewAgentConn(nil, "node-1", "linux"))
	sent := 0
	d := &Deps{
		AgentMgr:                manager,
		SendSSHKeyRemoveToAsset: func(string) { sent++ },
	}

	err := d.DeleteAsset(context.Background(), assets.Asset{ID: "node-1", Source: "agent"})
	if !errors.Is(err, ErrConnectedAgentDeletion) {
		t.Fatalf("DeleteAsset error = %v, want ErrConnectedAgentDeletion", err)
	}
	if sent != 0 {
		t.Fatalf("refused delete sent %d SSH key removals", sent)
	}
	if !manager.IsConnected("node-1") {
		t.Fatal("refused delete disconnected the agent")
	}
}
