package powercontrol

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func acceptedResult(t *testing.T, msg agentmgr.Message) agentmgr.Message {
	t.Helper()
	var action agentmgr.PowerActionData
	if err := json.Unmarshal(msg.Data, &action); err != nil {
		t.Errorf("decode action: %v", err)
	}
	raw, _ := json.Marshal(agentmgr.PowerResultData{
		RequestID: action.RequestID, AssetID: action.AssetID, Action: action.Action, Status: agentmgr.PowerResultAccepted,
	})
	return agentmgr.Message{Type: agentmgr.MsgPowerResult, ID: action.RequestID, Data: raw}
}

// Fault: a result from another connection that carries the same device ID
// (an older socket of the same agent) answers the action. Only the
// connection the action was sent on may answer it.
func TestCoordinatorTakesTheResultOnlyFromTheSendingConnection(t *testing.T) {
	sender := connectedSender(t)
	coordinator := New(sender, 50*time.Millisecond)
	sender.respond(func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		older := &agentmgr.AgentConn{AssetID: conn.AssetID}
		if coordinator.HandleResult(older, acceptedResult(t, msg)) {
			t.Error("a result from another connection of node-1 was delivered")
		}
	})
	if _, err := coordinator.Execute(context.Background(), "node-1", agentmgr.PowerActionReboot); KindOf(err) != ErrorTimedOut {
		t.Fatalf("kind=%q err=%v, want %q after the other connection's result was dropped", KindOf(err), err, ErrorTimedOut)
	}
}

// Fault: a reboot or shut down whose connection closes before a result is
// reported as "agent is not connected". The device going down closes it, so
// that answer would say nothing happened when the action most likely worked.
// On purpose the action waits for its timeout and answers as a timeout, the
// same "no confirmation" as before connections were watched.
func TestCoordinatorTreatsADisconnectAsNoConfirmation(t *testing.T) {
	const wait = 50 * time.Millisecond
	sender := connectedSender(t)
	coordinator := New(sender, wait)
	sender.respond(func(conn *agentmgr.AgentConn, _ agentmgr.Message) {
		sender.peer.Manager.UnregisterIfMatch(conn.AssetID, conn)
	})
	began := time.Now()
	result, err := coordinator.Execute(context.Background(), "node-1", agentmgr.PowerActionReboot)
	if KindOf(err) != ErrorTimedOut || err.Error() != "timed out waiting for the agent power result" {
		t.Fatalf("kind=%q err=%v, want %q with the timeout words", KindOf(err), err, ErrorTimedOut)
	}
	if took := time.Since(began); took < wait {
		t.Fatalf("action ended %v after it was sent, before its %v wait", took, wait)
	}
	if result.RequestID == "" {
		t.Fatal("the result carries no request ID for the audit log")
	}
}
