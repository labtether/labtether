package agents

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

// A queue claim may be retried on the same agent socket after cancellation.
// Only the fresh attempt may complete the logical job, including updates.
func TestAgentContextAttemptRejectsLateSameSocketResult(t *testing.T) {
	for _, update := range []bool{false, true} {
		name := "command"
		if update {
			name = "update"
		}
		t.Run(name, func(t *testing.T) {
			manager, conn, peer := newDockerEndpointTestAgentConn(t, "context-node", "linux")
			deps := &Deps{AgentMgr: manager, PendingAgentCmds: &sync.Map{}}
			job := terminal.CommandJob{JobID: "same-job", Target: conn.AssetID, SessionID: "session", CommandID: "command", Command: "true", TimeoutSec: 60}
			run := func(ctx context.Context) agentmgr.CommandResultData {
				if update {
					return deps.ExecuteUpdateViaAgentContext(ctx, job.JobID, job.Target, "os_packages", nil, time.Minute, false)
				}
				r := deps.ExecuteViaAgentContext(ctx, job)
				return agentmgr.CommandResultData{JobID: r.JobID, SessionID: r.SessionID, CommandID: r.CommandID, Status: r.Status, Output: r.Output}
			}
			var started []<-chan struct{}
			t.Cleanup(func() {
				conn.Close()
				for _, done := range started {
					select {
					case <-done:
					case <-time.After(2 * time.Second):
						t.Error("request worker did not exit during fixture cleanup")
					}
				}
			})
			launch := func(ctx context.Context) <-chan agentmgr.CommandResultData {
				results := make(chan agentmgr.CommandResultData, 1)
				done := make(chan struct{})
				started = append(started, done)
				go func() { defer close(done); results <- run(ctx) }()
				return results
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if r := run(ctx); r.Status != "failed" || !strings.Contains(r.Output, "before dispatch") {
				t.Fatalf("already canceled: %+v", r)
			}
			ctxA, cancelA := context.WithCancel(context.Background())
			defer cancelA()
			resultsA := launch(ctxA)
			requestA := readPrivateAgentRequest(t, peer, update)
			if requestA.ID == job.JobID {
				t.Fatal("logical job exposed as wire attempt")
			}
			var duplicate agentmgr.CommandResultData
			if update {
				duplicate = deps.ExecuteUpdateViaAgent(job.JobID, job.Target, "os_packages", nil, time.Minute, false)
			} else {
				r := deps.ExecuteViaAgent(job)
				duplicate = agentmgr.CommandResultData{Status: r.Status, Output: r.Output}
			}
			if duplicate.Status != "failed" || !strings.Contains(duplicate.Output, "already in progress") {
				t.Fatalf("duplicate: %+v", duplicate)
			}
			cancelA()
			if r := awaitContextAgentResult(t, resultsA); r.JobID != job.JobID || r.Status != "failed" || !strings.Contains(r.Output, "outcome unknown") {
				t.Fatalf("canceled A: %+v", r)
			}
			if _, exists := deps.PendingAgentCmds.Load(requestA.ID); exists {
				t.Fatal("canceled wire waiter retained")
			}
			if update && deps.acceptsAgentUpdate(conn, requestA.ID) {
				t.Fatal("canceled update progress retained")
			}
			ctxB, cancelB := context.WithCancel(context.Background())
			defer cancelB()
			resultsB := launch(ctxB)
			requestB := readPrivateAgentRequest(t, peer, update)
			if requestB.ID == requestA.ID || requestB.ID == job.JobID {
				t.Fatal("retry reused a wire attempt")
			}
			deliver := func(sender *agentmgr.AgentConn, id, output string) {
				if update {
					data, err := json.Marshal(agentmgr.UpdateResultData{JobID: id, Status: "succeeded", Output: output})
					if err != nil {
						t.Fatal(err)
					}
					deps.ProcessAgentUpdateResult(sender, agentmgr.Message{Type: agentmgr.MsgUpdateResult, ID: id, Data: data})
				} else {
					deps.DeliverPendingAgentResult(sender, agentmgr.CommandResultData{JobID: id, Status: "succeeded", Output: output})
				}
			}
			deliver(conn, requestA.ID, "late A")
			deliver(&agentmgr.AgentConn{AssetID: conn.AssetID}, requestB.ID, "wrong socket")
			raw, exists := deps.PendingAgentCmds.Load(requestB.ID)
			if !exists {
				t.Fatal("late result removed B")
			}
			if len(raw.(*PendingAgentCommand).ResultCh) != 0 {
				t.Fatal("stale result reached B's channel")
			}
			if update && !deps.acceptsAgentUpdate(conn, requestB.ID) {
				t.Fatal("late update ended B progress")
			}
			deliver(conn, requestB.ID, "current B")
			r := awaitContextAgentResult(t, resultsB)
			if r.JobID != job.JobID || r.Status != "succeeded" || r.Output != "current B" {
				t.Fatalf("retry result: %+v", r)
			}
			if !update && (r.SessionID != job.SessionID || r.CommandID != job.CommandID) {
				t.Fatalf("logical correlation lost: %+v", r)
			}
			deps.PendingAgentCmds.Range(func(key, value any) bool { t.Errorf("pending reservation remains: %v", key); return true })
		})
	}
}

func readPrivateAgentRequest(t *testing.T, peer *websocket.Conn, update bool) agentmgr.Message {
	t.Helper()
	if err := peer.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var msg agentmgr.Message
	if err := peer.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	var body struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(msg.Data, &body); err != nil {
		t.Fatal(err)
	}
	expected := agentmgr.MsgCommandRequest
	if update {
		expected = agentmgr.MsgUpdateRequest
	}
	if msg.Type != expected || msg.ID == "" || msg.ID != body.JobID {
		t.Fatalf("bad attempt request: %+v", msg)
	}
	return msg
}

func awaitContextAgentResult(t *testing.T, ch <-chan agentmgr.CommandResultData) agentmgr.CommandResultData {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("context request did not finish")
	}
	return agentmgr.CommandResultData{}
}
