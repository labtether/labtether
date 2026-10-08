package docker

import (
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectorsdk"
	"strings"
	"testing"
)

func TestCoordinatorExecuteActionRoutes(t *testing.T) {
	sent := make(chan agentmgr.Message, 1)
	mock := &mockAgentCommander{sendFn: func(id string, msg agentmgr.Message) error {
		sent <- msg
		return nil
	}}

	coord := NewCoordinator(mock)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID:     "agent-01",
		Containers: []agentmgr.DockerContainerInfo{{ID: "abc123456789ab", Name: "nginx", State: "running"}},
	}))

	// Execute restart in background and simulate result
	go func() {
		msg := <-sent
		if msg.Type != agentmgr.MsgDockerAction {
			t.Errorf("expected docker.action, got %s", msg.Type)
		}
		var req agentmgr.DockerActionData
		json.Unmarshal(msg.Data, &req)

		// Send result back
		resultData := agentmgr.DockerActionResultData{
			RequestID: req.RequestID,
			Success:   true,
		}
		raw, _ := json.Marshal(resultData)
		coord.HandleActionResult("agent-01", agentmgr.Message{Type: agentmgr.MsgDockerActionResult, ID: req.RequestID, Data: raw})
	}()

	result, err := coord.ExecuteAction(context.Background(), "container.restart", connectorsdk.ActionRequest{
		TargetID: "docker-ct-agent-01-abc123456789",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" {
		t.Errorf("status = %q, want succeeded", result.Status)
	}
}

func TestCoordinatorExecuteActionContainerCreateUsesHostTarget(t *testing.T) {
	sent := make(chan agentmgr.Message, 1)
	mock := &mockAgentCommander{sendFn: func(id string, msg agentmgr.Message) error {
		sent <- msg
		return nil
	}}

	coord := NewCoordinator(mock)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
	}))

	go func() {
		msg := <-sent
		if msg.Type != agentmgr.MsgDockerAction {
			t.Errorf("expected docker.action, got %s", msg.Type)
		}
		var req agentmgr.DockerActionData
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			t.Errorf("failed to unmarshal action data: %v", err)
			return
		}
		if req.Action != "container.create" {
			t.Errorf("action = %q, want container.create", req.Action)
		}
		if req.ContainerID != "" {
			t.Errorf("container_id = %q, want empty for host-target action", req.ContainerID)
		}
		if req.Params["image"] != "nginx:latest" {
			t.Errorf("image param = %q, want nginx:latest", req.Params["image"])
		}

		resultData := agentmgr.DockerActionResultData{
			RequestID: req.RequestID,
			Success:   true,
			Data:      "new-container-id",
		}
		raw, _ := json.Marshal(resultData)
		coord.HandleActionResult("agent-01", agentmgr.Message{Type: agentmgr.MsgDockerActionResult, ID: req.RequestID, Data: raw})
	}()

	result, err := coord.ExecuteAction(context.Background(), "container.create", connectorsdk.ActionRequest{
		TargetID: "docker-host-agent-01",
		Params: map[string]string{
			"image": "nginx:latest",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", result.Status)
	}
	if result.Output != "new-container-id" {
		t.Fatalf("output = %q, want new-container-id", result.Output)
	}
}

func TestCoordinatorExecuteActionStackDeployUsesHostTarget(t *testing.T) {
	sent := make(chan agentmgr.Message, 1)
	mock := &mockAgentCommander{sendFn: func(id string, msg agentmgr.Message) error {
		sent <- msg
		return nil
	}}

	coord := NewCoordinator(mock)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
	}))

	go func() {
		msg := <-sent
		if msg.Type != agentmgr.MsgDockerComposeAction {
			t.Errorf("expected docker.compose.action, got %s", msg.Type)
		}
		var req agentmgr.DockerComposeActionData
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			t.Errorf("failed to unmarshal compose action data: %v", err)
			return
		}
		if req.Action != "deploy" {
			t.Errorf("action = %q, want deploy", req.Action)
		}
		if req.StackName != "demo" {
			t.Errorf("stack_name = %q, want demo", req.StackName)
		}
		if !strings.Contains(req.ComposeYAML, "services:") {
			t.Errorf("compose yaml missing services stanza: %q", req.ComposeYAML)
		}

		resultData := agentmgr.DockerComposeResultData{
			RequestID: req.RequestID,
			Success:   true,
			Output:    "deployed",
		}
		raw, _ := json.Marshal(resultData)
		coord.HandleComposeResult("agent-01", agentmgr.Message{Type: agentmgr.MsgDockerComposeResult, ID: req.RequestID, Data: raw})
	}()

	result, err := coord.ExecuteAction(context.Background(), "stack.deploy", connectorsdk.ActionRequest{
		TargetID: "agent-01",
		Params: map[string]string{
			"stack_name":   "demo",
			"compose_yaml": "services:\n  web:\n    image: nginx:latest\n",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", result.Status)
	}
}

func TestCoordinatorActionResultRequiresExpectedAgentAndMatchingEnvelope(t *testing.T) {
	var coord *Coordinator
	mock := &mockAgentCommander{sendFn: func(id string, msg agentmgr.Message) error {
		if id != "agent-01" {
			t.Fatalf("sent to agent %q, want agent-01", id)
		}
		var req agentmgr.DockerActionData
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			t.Fatalf("unmarshal action request: %v", err)
		}

		spoofed, _ := json.Marshal(agentmgr.DockerActionResultData{
			RequestID: req.RequestID,
			Success:   true,
			Data:      "spoofed-agent",
		})
		coord.HandleActionResult("agent-02", agentmgr.Message{
			Type: agentmgr.MsgDockerActionResult,
			ID:   req.RequestID,
			Data: spoofed,
		})

		mismatched, _ := json.Marshal(agentmgr.DockerActionResultData{
			RequestID: req.RequestID,
			Success:   true,
			Data:      "mismatched-envelope",
		})
		coord.HandleActionResult("agent-01", agentmgr.Message{
			Type: agentmgr.MsgDockerActionResult,
			ID:   req.RequestID + "-wrong",
			Data: mismatched,
		})

		correct, _ := json.Marshal(agentmgr.DockerActionResultData{
			RequestID: req.RequestID,
			Success:   true,
			Data:      "correct-agent",
		})
		coord.HandleActionResult("agent-01", agentmgr.Message{
			Type: agentmgr.MsgDockerActionResult,
			ID:   req.RequestID,
			Data: correct,
		})
		return nil
	}}

	coord = NewCoordinator(mock)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID:     "agent-01",
		Containers: []agentmgr.DockerContainerInfo{{ID: "abc123456789ab", Name: "nginx", State: "running"}},
	}))

	result, err := coord.ExecuteAction(context.Background(), "container.restart", connectorsdk.ActionRequest{
		TargetID: "docker-ct-agent-01-abc123456789",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || result.Output != "correct-agent" {
		t.Fatalf("result = %#v, want only the expected agent's correlated response", result)
	}
}

func TestCoordinatorComposeResultRequiresExpectedAgentAndMatchingEnvelope(t *testing.T) {
	var coord *Coordinator
	mock := &mockAgentCommander{sendFn: func(id string, msg agentmgr.Message) error {
		if id != "agent-01" {
			t.Fatalf("sent to agent %q, want agent-01", id)
		}
		var req agentmgr.DockerComposeActionData
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			t.Fatalf("unmarshal compose request: %v", err)
		}

		spoofed, _ := json.Marshal(agentmgr.DockerComposeResultData{
			RequestID: req.RequestID,
			Success:   true,
			Output:    "spoofed-agent",
		})
		coord.HandleComposeResult("agent-02", agentmgr.Message{
			Type: agentmgr.MsgDockerComposeResult,
			ID:   req.RequestID,
			Data: spoofed,
		})

		mismatched, _ := json.Marshal(agentmgr.DockerComposeResultData{
			RequestID: req.RequestID,
			Success:   true,
			Output:    "mismatched-envelope",
		})
		coord.HandleComposeResult("agent-01", agentmgr.Message{
			Type: agentmgr.MsgDockerComposeResult,
			ID:   req.RequestID + "-wrong",
			Data: mismatched,
		})

		correct, _ := json.Marshal(agentmgr.DockerComposeResultData{
			RequestID: req.RequestID,
			Success:   true,
			Output:    "correct-agent",
		})
		coord.HandleComposeResult("agent-01", agentmgr.Message{
			Type: agentmgr.MsgDockerComposeResult,
			ID:   req.RequestID,
			Data: correct,
		})
		return nil
	}}

	coord = NewCoordinator(mock)
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{HostID: "agent-01"}))

	result, err := coord.ExecuteAction(context.Background(), "stack.deploy", connectorsdk.ActionRequest{
		TargetID: "agent-01",
		Params: map[string]string{
			"stack_name":   "demo",
			"compose_yaml": "services: {}",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || result.Output != "correct-agent" {
		t.Fatalf("result = %#v, want only the expected agent's correlated response", result)
	}
}

func TestCoordinatorExecuteActionStackDeployMissingStackName(t *testing.T) {
	coord := NewCoordinator(&mockAgentCommander{})
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
	}))

	result, err := coord.ExecuteAction(context.Background(), "stack.deploy", connectorsdk.ActionRequest{
		TargetID: "agent-01",
		Params: map[string]string{
			"compose_yaml": "services: {}",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if !strings.Contains(result.Message, "stack_name is required") {
		t.Fatalf("message = %q, want stack_name validation", result.Message)
	}
}
