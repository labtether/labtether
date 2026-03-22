package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestDockerExecBridgeReasonLifecycle(t *testing.T) {
	bridge := &dockerExecBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}

	if got := bridge.ReasonOr("fallback"); got != "fallback" {
		t.Fatalf("unexpected default reason: %q", got)
	}

	bridge.Close("  startup failed  ")
	if got := bridge.ReasonOr("fallback"); got != "startup failed" {
		t.Fatalf("reason = %q, want %q", got, "startup failed")
	}

	bridge.Close("another reason")
	if got := bridge.ReasonOr("fallback"); got != "startup failed" {
		t.Fatalf("reason should not be overwritten, got %q", got)
	}

	select {
	case <-bridge.ClosedCh:
	default:
		t.Fatal("expected bridge to be closed")
	}
}

func TestProcessAgentDockerExecClosedStoresReason(t *testing.T) {
	s := &apiServer{}
	bridge := &dockerExecBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
	}
	s.dockerExecBridges.Store("sess-1", bridge)

	raw, _ := json.Marshal(agentmgr.DockerExecCloseData{
		SessionID: "sess-1",
		Reason:    "stdin write failed: broken pipe",
	})
	s.processAgentDockerExecClosed(&agentmgr.AgentConn{AssetID: "node-1"}, agentmgr.Message{
		Type: agentmgr.MsgDockerExecClosed,
		ID:   "sess-1",
		Data: raw,
	})

	if got := bridge.ReasonOr("fallback"); got != "stdin write failed: broken pipe" {
		t.Fatalf("unexpected close reason: %q", got)
	}
	select {
	case <-bridge.ClosedCh:
	default:
		t.Fatal("expected bridge to be closed")
	}
}

func TestProcessAgentDockerExecHandlersIgnoreMismatchedSender(t *testing.T) {
	s := &apiServer{}
	bridge := &dockerExecBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
	}
	s.dockerExecBridges.Store("sess-mismatch", bridge)

	started, _ := json.Marshal(agentmgr.DockerExecStartedData{SessionID: "sess-mismatch"})
	data, _ := json.Marshal(agentmgr.DockerExecDataPayload{
		SessionID: "sess-mismatch",
		Data:      "aGVsbG8=",
	})
	closed, _ := json.Marshal(agentmgr.DockerExecCloseData{
		SessionID: "sess-mismatch",
		Reason:    "wrong sender",
	})

	conn := &agentmgr.AgentConn{AssetID: "node-2"}
	s.processAgentDockerExecStarted(conn, agentmgr.Message{Data: started})
	s.processAgentDockerExecData(conn, agentmgr.Message{Data: data})
	s.processAgentDockerExecClosed(conn, agentmgr.Message{Data: closed})

	select {
	case <-bridge.OutputCh:
		t.Fatal("expected no output to be delivered from mismatched sender")
	default:
	}
	select {
	case <-bridge.ClosedCh:
		t.Fatal("expected bridge to remain open for mismatched sender")
	default:
	}
}

func TestDockerExecCommandFromQuery(t *testing.T) {
	tooLong := strings.Repeat("x", 140)

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty uses default", raw: "", want: []string{"sh"}},
		{name: "trimmed single token", raw: "  /bin/sh  ", want: []string{"/bin/sh"}},
		{name: "multi token command", raw: "bash -lc", want: []string{"bash", "-lc"}},
		{name: "drops oversized tokens", raw: "bash " + tooLong + " -l", want: []string{"bash", "-l"}},
		{name: "all tokens invalid uses default", raw: tooLong, want: []string{"sh"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dockerExecCommandFromQuery(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("len(command)=%d want %d (%v)", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("command[%d]=%q want %q (all=%v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

func TestCloseDockerExecBridgesForAssetClosesMatchingSessionsOnly(t *testing.T) {
	var s apiServer
	matching := &dockerExecBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
	}
	nonMatching := &dockerExecBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-2",
	}

	s.dockerExecBridges.Store("sess-node-1", matching)
	s.dockerExecBridges.Store("sess-node-2", nonMatching)
	s.dockerExecBridges.Store("non-bridge-entry", "ignore-me")

	s.closeDockerExecBridgesForAsset("node-1")

	select {
	case <-matching.ClosedCh:
	default:
		t.Fatal("expected matching docker exec bridge to be closed")
	}
	if got := matching.ReasonOr(""); got != "container host agent disconnected" {
		t.Fatalf("unexpected close reason %q", got)
	}

	select {
	case <-nonMatching.ClosedCh:
		t.Fatal("expected non-matching docker exec bridge to remain open")
	default:
	}
}
