package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/mark3labs/mcp-go/mcp"
)

// Cancelling an MCP request must release its real agent waiter without closing
// the shared socket or allowing a canceled request to dispatch later.
func TestMCPExecCancellationReachesAgentWaiter(t *testing.T) {
	for _, tool := range []string{"exec", "exec_multi"} {
		t.Run(tool, func(t *testing.T) {
			s := newTestAPIServer(t)
			s.agentMgr = agentmgr.NewManager()
			const assetID = "mcp-context-node"
			if _, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
				AssetID: assetID, Name: assetID, Status: "online", Platform: "linux",
			}); err != nil {
				t.Fatal(err)
			}
			serverWS, peerWS, cleanup := createWSPairForNetworkTest(t)
			agent := agentmgr.NewAgentConn(serverWS, assetID, "linux")
			s.agentMgr.Register(agent)
			base := contextWithPrincipal(context.Background(), "mcp-context-owner", "admin")
			base = contextWithScopes(base, []string{"assets:exec"})
			base = contextWithAllowedAssets(base, []string{assetID})
			ctx, cancel := context.WithCancel(base)
			var workers []<-chan struct{}
			t.Cleanup(func() {
				cancel()
				agent.Close()
				cleanup()
				s.agentMgr.Unregister(assetID)
				for _, done := range workers {
					select {
					case <-done:
					case <-time.After(2 * time.Second):
						t.Error("MCP command worker did not stop")
					}
				}
			})
			handler := s.buildMCPServer().GetTool(tool).Handler
			type response struct {
				result *mcp.CallToolResult
				err    error
			}
			launch := func(callCtx context.Context, command string) <-chan response {
				args := map[string]any{"command": command, "timeout": 60}
				if tool == "exec" {
					args["asset_id"] = assetID
				} else {
					args["targets"] = []any{assetID}
				}
				result, done := make(chan response, 1), make(chan struct{})
				workers = append(workers, done)
				go func() {
					defer close(done)
					value, err := handler(callCtx, mcpBridgeToolRequest(args))
					result <- response{value, err}
				}()
				return result
			}
			await := func(ch <-chan response, failed bool, output string) {
				t.Helper()
				select {
				case got := <-ch:
					if got.err != nil || got.result == nil || got.result.IsError != failed || len(got.result.Content) != 1 {
						t.Fatalf("MCP response=%#v error=%v", got.result, got.err)
					}
					text, ok := got.result.Content[0].(mcp.TextContent)
					if !ok || !strings.Contains(text.Text, output) {
						t.Fatalf("MCP content=%#v, want %q", got.result.Content, output)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("MCP request kept waiting after cancellation/result")
				}
			}
			assertNoWaiters := func() {
				t.Helper()
				s.pendingAgentCmds.Range(func(key, value any) bool {
					t.Errorf("MCP request left pending agent waiter %v", key)
					return true
				})
			}
			readRequest := func(command string) agentmgr.CommandRequestData {
				t.Helper()
				if err := peerWS.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
					t.Fatal(err)
				}
				var wire agentmgr.Message
				if err := peerWS.ReadJSON(&wire); err != nil {
					t.Fatal(err)
				}
				var request agentmgr.CommandRequestData
				if err := json.Unmarshal(wire.Data, &request); err != nil {
					t.Fatal(err)
				}
				if wire.Type != agentmgr.MsgCommandRequest || wire.ID == "" || wire.ID != request.JobID || request.Command != command {
					t.Fatalf("unexpected agent dispatch: wire=%+v request=%+v", wire, request)
				}
				return request
			}
			alreadyCanceled, cancelBefore := context.WithCancel(base)
			cancelBefore()
			await(launch(alreadyCanceled, "uname -a"), true, "canceled")
			assertNoWaiters()

			inflight := launch(ctx, "uname -s")
			// Socket order also proves the canceled uname -a emitted no request.
			first := readRequest("uname -s")
			cancel()
			await(inflight, true, "outcome unknown")
			assertNoWaiters()
			if current, ok := s.agentMgr.Get(assetID); !ok || current != agent {
				t.Fatal("cancellation removed the shared agent connection")
			}

			positive := launch(base, "uname -n")
			next := readRequest("uname -n")
			if next.JobID == first.JobID {
				t.Fatal("new MCP request reused canceled wire ID")
			}
			data, err := json.Marshal(agentmgr.CommandResultData{
				JobID: next.JobID, SessionID: next.SessionID, CommandID: next.CommandID,
				Status: "succeeded", Output: "same-socket-result",
			})
			if err != nil {
				t.Fatal(err)
			}
			s.processAgentCommandResult(agent, agentmgr.Message{Type: agentmgr.MsgCommandResult, ID: next.JobID, Data: data})
			await(positive, false, "same-socket-result")
			assertNoWaiters()
		})
	}
}
