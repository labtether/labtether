package docker

import (
	"context"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectorsdk"
)

// A downgrade after discovery can lose a reply, but it must never cause the
// coordinator to retry a Compose write using the old lossy message type.
func TestCoordinatorComposeCapabilityPreventsLegacyFallback(t *testing.T) {
	for _, version := range []int{0, 2} {
		ctx, cancel := context.WithCancel(context.Background())
		messages, legacyEffects := 0, 0
		coord := NewCoordinator(&mockAgentCommander{sendFn: func(_ string, msg agentmgr.Message) error {
			messages++
			// Model an old agent that understands only the original action type.
			if msg.Type == agentmgr.MsgDockerComposeAction {
				legacyEffects++
			}
			if msg.Type != agentmgr.MsgDockerComposeActionV2 {
				t.Errorf("unexpected wire type: %s", msg.Type)
			}
			cancel() // Bound the missing reply without waiting five minutes.
			return nil
		}})
		coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{ComposeActionVersion: version, ComposeEndpointID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}))
		result, err := coord.ExecuteAction(ctx, "stack.deploy", connectorsdk.ActionRequest{TargetID: "agent-01", Params: map[string]string{"stack_name": "native_project", "compose_yaml": "services: {}"}})
		cancel()
		if err != nil || result.Status != "failed" || legacyEffects != 0 {
			t.Fatalf("downgrade was unsafe: %+v err=%v legacy=%d", result, err, legacyEffects)
		}
		if version == 0 && (messages != 0 || !strings.Contains(result.Message, "upgraded agent")) {
			t.Fatalf("old capability did not fail before send: %+v messages=%d", result, messages)
		}
		if version == 2 && messages != 1 {
			t.Fatalf("missing v2 reply triggered fallback: messages=%d", messages)
		}
		if len(coord.pendingCompose) != 0 {
			t.Fatal("retired attempt retained a reply waiter")
		}
	}
	t.Run("deploy endpoint changed after admission", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		messages := 0
		coord := NewCoordinator(&mockAgentCommander{sendFn: func(_ string, _ agentmgr.Message) error {
			messages++
			cancel() // Bound a regression that sends without waiting for a reply.
			return nil
		}})
		coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
			ComposeActionVersion: 2, ComposeEndpointID: strings.Repeat("a", 64),
		}))
		target, err := coord.ResolveActionTarget("stack.deploy", "agent-01")
		if err != nil {
			t.Fatal(err)
		}
		coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
			ComposeActionVersion: 2, ComposeEndpointID: strings.Repeat("b", 64),
		}))
		ctx = WithActionTarget(ctx, target)
		result, err := coord.ExecuteAction(ctx, "stack.deploy", connectorsdk.ActionRequest{
			TargetID: "agent-01", Params: map[string]string{"stack_name": "native_project", "compose_yaml": "services: {}"},
		})
		if err != nil || result.Status != "failed" || !strings.Contains(result.Message, "authorized docker resource") || messages != 0 {
			t.Fatalf("deploy changed endpoint: %+v err=%v messages=%d", result, err, messages)
		}
	})
}
