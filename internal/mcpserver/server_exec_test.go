package mcpserver

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/mark3labs/mcp-go/mcp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHandleExec_ScopeDenied(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"assets:read"} } // no exec
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1", "command": "uptime"}
	result, err := deps.handleExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for insufficient scope")
	}
}

func TestHandleExec_AssetOffline(t *testing.T) {
	deps := newTestDeps()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv2", "command": "uptime"}
	result, err := deps.handleExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for offline asset")
	}
}

func TestHandleExecRejectsCommandDeniedByPolicy(t *testing.T) {
	deps := newTestDeps()
	dispatched := false
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		dispatched = true
		return terminal.CommandResult{Status: "succeeded"}
	}
	deps.EvaluateCommandPolicy = func(context.Context, string, string) policy.CheckResponse {
		return policy.CheckResponse{Allowed: false, Reason: "command not in allowlist", Mode: "structured"}
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1", "command": "curl example.com"}

	result, err := deps.handleExec(context.Background(), req)
	if err != nil {
		t.Fatalf("handleExec error: %v", err)
	}
	if !result.IsError {
		t.Fatal("policy-denied command returned success")
	}
	if dispatched {
		t.Fatal("policy-denied command reached execution backend")
	}
}

func TestHandleExecMultiRejectsCommandDeniedByPolicy(t *testing.T) {
	deps := newTestDeps()
	dispatched := false
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		dispatched = true
		return terminal.CommandResult{Status: "succeeded"}
	}
	deps.EvaluateCommandPolicy = func(context.Context, string, string) policy.CheckResponse {
		return policy.CheckResponse{Allowed: false, Reason: "command not in allowlist", Mode: "structured"}
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"targets": []any{"srv1"}, "command": "curl example.com"}

	result, err := deps.handleExecMulti(context.Background(), req)
	if err != nil {
		t.Fatalf("handleExecMulti error: %v", err)
	}
	if result == nil || !strings.Contains(toolResultText(t, result), "policy_denied") {
		t.Fatalf("policy denial missing from result: %#v", result)
	}
	if dispatched {
		t.Fatal("policy-denied command reached execution backend")
	}
}

func TestHandleExecPassesTimeoutToAgentCommand(t *testing.T) {
	deps := newTestDeps()
	var captured terminal.CommandJob
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		captured = job
		return terminal.CommandResult{Status: "succeeded", Output: "ok"}
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1", "command": "uptime", "timeout": 45}
	result, err := deps.handleExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("should not return error for connected asset")
	}
	if captured.TimeoutSec != 45 {
		t.Fatalf("TimeoutSec = %d, want 45", captured.TimeoutSec)
	}
}

func TestHandleExecNormalizesTimeoutBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  int
		want int
	}{
		{name: "default", raw: 0, want: defaultExecTimeoutSeconds},
		{name: "negative", raw: -5, want: defaultExecTimeoutSeconds},
		{name: "max", raw: maxExecTimeoutSeconds + 1, want: maxExecTimeoutSeconds},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestDeps()
			var captured terminal.CommandJob
			deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
				captured = job
				return terminal.CommandResult{Status: "succeeded", Output: "ok"}
			}

			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{"asset_id": "srv1", "command": "uptime", "timeout": tc.raw}
			result, err := deps.handleExec(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsError {
				t.Fatal("should not return error for connected asset")
			}
			if captured.TimeoutSec != tc.want {
				t.Fatalf("TimeoutSec = %d, want %d", captured.TimeoutSec, tc.want)
			}
		})
	}
}

func TestHandleExecMultiPassesClampedTimeoutToAgentCommands(t *testing.T) {
	deps := newTestDeps()
	deps.AgentMgr = &mockAgentMgr{connected: map[string]bool{"srv1": true, "srv2": true}}
	captured := make(chan terminal.CommandJob, 2)
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		captured <- job
		return terminal.CommandResult{Status: "succeeded", Output: "ok"}
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"targets": []any{"srv1", "srv2"},
		"command": "uptime",
		"timeout": maxExecTimeoutSeconds + 99,
	}
	result, err := deps.handleExecMulti(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("should not return error for connected assets")
	}
	if len(captured) != 2 {
		t.Fatalf("captured %d jobs, want 2", len(captured))
	}
	for i := 0; i < 2; i++ {
		job := <-captured
		if job.TimeoutSec != maxExecTimeoutSeconds {
			t.Fatalf("TimeoutSec for %s = %d, want %d", job.Target, job.TimeoutSec, maxExecTimeoutSeconds)
		}
	}
}

func TestHandleExecMultiDeduplicatesTargets(t *testing.T) {
	deps := newTestDeps()
	var calls atomic.Int64
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		calls.Add(1)
		return terminal.CommandResult{Status: "succeeded", Output: job.Target}
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"targets": []any{" srv1 ", "srv1", "srv1"},
		"command": "uptime",
	}
	result, err := deps.handleExecMulti(context.Background(), req)
	if err != nil || result.IsError {
		t.Fatalf("exec_multi failed: err=%v result=%v", err, result)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("duplicate target executed %d times, want 1", got)
	}
}

func TestHandleExecMultiRejectsExcessTargetsBeforeDispatch(t *testing.T) {
	deps := newTestDeps()
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		t.Fatalf("ExecuteViaAgent called for oversized request: %#v", job)
		return terminal.CommandResult{}
	}
	targets := make([]any, maxExecMultiTargets+1)
	for i := range targets {
		targets[i] = "srv1"
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"targets": targets, "command": "uptime"}
	result, err := deps.handleExecMulti(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("oversized target list should be rejected")
	}
}

func TestHandleExecMultiCapsConcurrency(t *testing.T) {
	deps := newTestDeps()
	targets := make([]any, maxExecMultiTargets)
	connected := make(map[string]bool, len(targets))
	for i := range targets {
		target := fmt.Sprintf("srv-%02d", i)
		targets[i] = target
		connected[target] = true
	}
	deps.AgentMgr = &mockAgentMgr{connected: connected}

	var current atomic.Int64
	var maximum atomic.Int64
	var dispatched atomic.Int64
	started := make(chan struct{}, maxExecMultiConcurrency)
	release := make(chan struct{})
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		active := current.Add(1)
		for {
			observed := maximum.Load()
			if active <= observed || maximum.CompareAndSwap(observed, active) {
				break
			}
		}
		if dispatched.Add(1) <= maxExecMultiConcurrency {
			started <- struct{}{}
		}
		<-release
		current.Add(-1)
		return terminal.CommandResult{Status: "succeeded"}
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"targets": targets, "command": "uptime"}
	done := make(chan *mcp.CallToolResult, 1)
	go func() {
		result, _ := deps.handleExecMulti(context.Background(), req)
		done <- result
	}()
	for range maxExecMultiConcurrency {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("worker pool did not reach expected concurrency")
		}
	}
	if got := maximum.Load(); got > maxExecMultiConcurrency {
		t.Fatalf("observed concurrency %d, max allowed %d", got, maxExecMultiConcurrency)
	}
	close(release)
	select {
	case result := <-done:
		if result == nil || result.IsError {
			t.Fatalf("exec_multi failed: %#v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("exec_multi did not complete")
	}
	if got := maximum.Load(); got > maxExecMultiConcurrency {
		t.Fatalf("observed concurrency %d, max allowed %d", got, maxExecMultiConcurrency)
	}
}
