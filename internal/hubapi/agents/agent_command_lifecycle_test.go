package agents

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

// Exercise the real command/update send and result owners over a real socket.
// A retired socket must release the waiter and cannot own a replacement job.
func TestAgentRequestLifecycle(t *testing.T) {
	for _, update := range []bool{false, true} {
		name := "command"
		if update {
			name = "update"
		}
		t.Run(name, func(t *testing.T) {
			manager, conn, peer := newDockerEndpointTestAgentConn(t, "request-node", "linux")
			deps := &Deps{AgentMgr: manager, PendingAgentCmds: &sync.Map{}}
			resultCh := make(chan agentmgr.CommandResultData, 1)
			run := func() agentmgr.CommandResultData {
				if update {
					return deps.ExecuteUpdateViaAgent("job", "request-node", "self", nil, time.Minute, false)
				}
				result := deps.ExecuteViaAgent(terminal.CommandJob{JobID: "job", Target: "request-node", SessionID: "session", CommandID: "command", Command: "true", TimeoutSec: 60})
				return agentmgr.CommandResultData{JobID: result.JobID, SessionID: result.SessionID, CommandID: result.CommandID, Status: result.Status, Output: result.Output}
			}
			go func() { resultCh <- run() }()
			readAgentLifecycleRequest(t, peer)
			raw, exists := deps.PendingAgentCmds.Load("job")
			if !exists {
				t.Fatal("request was not registered")
			}
			pending := raw.(*PendingAgentCommand)
			if duplicate := run(); duplicate.Status != "failed" || !strings.Contains(duplicate.Output, "already in progress") {
				t.Fatalf("duplicate result: %+v", duplicate)
			}
			if current, _ := deps.PendingAgentCmds.Load("job"); current != pending {
				t.Fatal("duplicate replaced or removed the original waiter")
			}
			other := &agentmgr.AgentConn{AssetID: conn.AssetID}
			reply := agentmgr.CommandResultData{JobID: "job", SessionID: "session", CommandID: "command", Status: "succeeded", Output: "stale"}
			deps.DeliverPendingAgentResult(other, reply)
			if len(pending.ResultCh) != 0 {
				t.Fatal("same-asset different connection delivered a result")
			}
			if update {
				payload, err := json.Marshal(agentmgr.UpdateResultData{JobID: "job", Status: "succeeded"})
				if err != nil {
					t.Fatal(err)
				}
				deps.ProcessAgentUpdateResult(other, agentmgr.Message{Type: agentmgr.MsgUpdateResult, Data: payload})
				if !deps.acceptsAgentUpdate(conn, "job") {
					t.Fatal("stale result ended the current update progress owner")
				}
				if len(pending.ResultCh) != 0 {
					t.Fatal("stale update result delivered")
				}
			}
			manager.UnregisterIfMatch(conn.AssetID, conn)
			select {
			case result := <-resultCh:
				if result.Status != "failed" || !strings.Contains(result.Output, "outcome unknown") {
					t.Fatalf("disconnect result: %+v", result)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("request still waits for its full timeout after disconnect")
			}
			if _, exists := deps.PendingAgentCmds.Load("job"); exists {
				t.Fatal("retired request remains registered")
			}
			if update && deps.acceptsAgentUpdate(conn, "job") {
				t.Fatal("retired update progress remains registered")
			}
		})
	}
}

// Older agents may omit optional correlation fields and the exit code. The
// owning connection still supplies a valid result, the pending request fills
// the correlation fields, and an absent exit code stays unknown, never 0.
// A newer agent's exit code 2 reaches the caller as 2, not as a bare failure.
func TestAgentRequestAcceptsResultFromOwningConnection(t *testing.T) {
	for _, reply := range []agentmgr.CommandResultData{
		{JobID: "job", Status: "succeeded", Output: "done"},
		{JobID: "job", Status: "failed", Output: "done", ExitCode: new(2)},
	} {
		manager, conn, peer := newDockerEndpointTestAgentConn(t, "result-node", "linux")
		deps := &Deps{AgentMgr: manager, PendingAgentCmds: &sync.Map{}}
		results := make(chan terminal.CommandResult, 1)
		go func() {
			results <- deps.ExecuteViaAgent(terminal.CommandJob{JobID: "job", Target: conn.AssetID, SessionID: "session", CommandID: "command", Command: "true"})
		}()
		readAgentLifecycleRequest(t, peer)
		deps.DeliverPendingAgentResult(conn, reply)
		select {
		case result := <-results:
			if result.Status != reply.Status || result.SessionID != "session" || result.CommandID != "command" || result.Output != "done" {
				t.Fatalf("result: %+v", result)
			}
			if got, want := exitCodeText(result.ExitCode), exitCodeText(reply.ExitCode); got != want {
				t.Fatalf("exit code %s, want %s", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("matching result did not finish request")
		}
	}
}

func readAgentLifecycleRequest(t *testing.T, peer *websocket.Conn) {
	t.Helper()
	if err := peer.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var msg agentmgr.Message
	if err := peer.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	if msg.ID != "job" || (msg.Type != agentmgr.MsgCommandRequest && msg.Type != agentmgr.MsgUpdateRequest) {
		t.Fatalf("unexpected request: %+v", msg)
	}
}

func exitCodeText(code *int) string {
	if code == nil {
		return "unknown"
	}
	return strconv.Itoa(*code)
}
