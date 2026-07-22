package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/terminal"
)

type mockAssetStore struct {
	assets []assets.Asset
}

func toolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("expected tool result text content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	return text.Text
}

func (m *mockAssetStore) ListAssets() ([]assets.Asset, error) { return m.assets, nil }
func (m *mockAssetStore) GetAsset(id string) (assets.Asset, bool, error) {
	for _, a := range m.assets {
		if a.ID == id {
			return a, true, nil
		}
	}
	return assets.Asset{}, false, nil
}

type mockAgentMgr struct{ connected map[string]bool }

func (m *mockAgentMgr) IsConnected(id string) bool { return m.connected[id] }

func newTestDeps() *Deps {
	return &Deps{
		AssetStore: &mockAssetStore{assets: []assets.Asset{
			{ID: "srv1", Name: "Server 1", Platform: "linux", Status: "online"},
			{ID: "srv2", Name: "Server 2", Platform: "linux", Status: "offline"},
		}},
		AgentMgr:         &mockAgentMgr{connected: map[string]bool{"srv1": true}},
		ExecuteViaAgent:  nil,
		GetScopes:        func(ctx context.Context) []string { return []string{"assets:read", "assets:exec"} },
		GetAllowedAssets: func(ctx context.Context) []string { return nil },
		GetActorID:       func(ctx context.Context) string { return "test" },
		AuthorizeMutation: func(context.Context, string, string) error {
			return nil
		},
	}
}

func TestHandleWhoami(t *testing.T) {
	deps := newTestDeps()
	result, err := deps.handleWhoami(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("whoami error: %v", err)
	}
	if result.IsError {
		t.Fatal("whoami should not error")
	}
}

func TestHandleAssetsList(t *testing.T) {
	deps := newTestDeps()
	result, err := deps.handleAssetsList(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("assets_list error: %v", err)
	}
	if result.IsError {
		t.Fatal("assets_list should not error")
	}
}

func TestHandleAssetsGet(t *testing.T) {
	deps := newTestDeps()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1"}
	result, err := deps.handleAssetsGet(context.Background(), req)
	if err != nil {
		t.Fatalf("assets_get error: %v", err)
	}
	if result.IsError {
		t.Fatal("assets_get should not error for existing asset")
	}
}

func TestHandleAssetsGet_NotFound(t *testing.T) {
	deps := newTestDeps()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "nonexistent"}
	result, err := deps.handleAssetsGet(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for nonexistent asset")
	}
}

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

func TestMCPResourcesRequireTheirReadScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		read   func(*Deps) error
	}{
		{
			name:   "assets",
			scopes: []string{"alerts:read"},
			read: func(deps *Deps) error {
				_, err := deps.handleAssetsResource(context.Background(), mcp.ReadResourceRequest{})
				return err
			},
		},
		{
			name:   "alerts",
			scopes: []string{"assets:read"},
			read: func(deps *Deps) error {
				_, err := deps.handleActiveAlertsResource(context.Background(), mcp.ReadResourceRequest{})
				return err
			},
		},
		{
			name:   "groups",
			scopes: []string{"assets:read"},
			read: func(deps *Deps) error {
				_, err := deps.handleGroupsResource(context.Background(), mcp.ReadResourceRequest{})
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestDeps()
			deps.GetScopes = func(context.Context) []string { return tc.scopes }
			if err := tc.read(deps); err == nil || !strings.Contains(err.Error(), "insufficient scope") {
				t.Fatalf("expected insufficient-scope error, got %v", err)
			}
		})
	}
}

// --- New tool tests ---

func TestHandleServicesList_ScopeDenied(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"assets:read"} }
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1"}
	result, err := deps.handleServicesList(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for insufficient scope")
	}
}

func TestHandleServicesList_NoAgent(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"services:read"} }
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv2"} // srv2 is offline/not connected
	result, err := deps.handleServicesList(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when agent is not connected")
	}
}

func TestHandleAlertsList_NilDep(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"alerts:read"} }
	deps.ListAlerts = nil // not configured
	result, err := deps.handleAlertsList(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when ListAlerts is nil")
	}
}

func TestHandleGroupsList_NilDep(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"groups:read"} }
	deps.ListGroups = nil // not configured
	result, err := deps.handleGroupsList(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when ListGroups is nil")
	}
}

func TestHandleAssetReboot_ScopeDenied(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"assets:read"} }
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1"}
	result, err := deps.handleAssetReboot(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for insufficient scope")
	}
}

func TestHandleAssetPowerUsesTypedExecutorNeverRawCommand(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return []string{"assets:power"} }
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		t.Fatalf("raw command executor must not be used for power action: %q", job.Command)
		return terminal.CommandResult{}
	}
	var gotAsset, gotAction string
	deps.ExecutePowerAction = func(_ context.Context, assetID, action string) (string, error) {
		gotAsset, gotAction = assetID, action
		return "reboot accepted", nil
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1"}
	result, err := deps.handleAssetReboot(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError || toolResultText(t, result) != "reboot accepted" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if gotAsset != "srv1" || gotAction != "reboot" {
		t.Fatalf("typed call asset=%q action=%q", gotAsset, gotAction)
	}
}

func TestHandleAssetPowerPropagatesTypedRejection(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return []string{"assets:power"} }
	deps.ExecutePowerAction = func(context.Context, string, string) (string, error) {
		return "", fmt.Errorf("power action rejected")
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1"}
	result, err := deps.handleAssetShutdown(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError || !strings.Contains(toolResultText(t, result), "rejected") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestHandleSystemDisks_NoAgent(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"disks:read"} }
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv2"} // srv2 not connected
	result, err := deps.handleSystemDisks(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when agent is not connected")
	}
}

func TestHandleServicesRestartRejectsShellMetacharacters(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"services:write"} }
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		t.Fatalf("ExecuteViaAgent should not be called for invalid service names; got %q", job.Command)
		return terminal.CommandResult{}
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1", "service_name": "sshd.service>pwn"}

	result, err := deps.handleServicesRestart(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for shell metacharacters in service name")
	}
}

func TestHandleDockerContainerToolsRejectShellMetacharacters(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"docker:read"} }
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		t.Fatalf("ExecuteViaAgent should not be called for invalid container IDs; got %q", job.Command)
		return terminal.CommandResult{}
	}

	for _, tc := range []struct {
		name string
		run  func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}{
		{name: "logs", run: deps.handleDockerContainerLogs},
		{name: "stats", run: deps.handleDockerContainerStats},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{"asset_id": "srv1", "container_id": "web>pwn"}

			result, err := tc.run(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("should return error for shell metacharacters in container ID")
			}
		})
	}
}

func TestHandleDockerContainerLogsUsesTypedDependency(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"docker:read"} }
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		t.Fatalf("raw command executor must not be used for Docker logs: %q", job.Command)
		return terminal.CommandResult{}
	}
	var gotAsset, gotContainer string
	var gotTail int
	deps.DockerContainerLogs = func(_ context.Context, assetID, containerID string, tail int) (string, error) {
		gotAsset, gotContainer, gotTail = assetID, containerID, tail
		return "ok", nil
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1", "container_id": "web.1_abc-2", "tail": 5}

	result, err := deps.handleDockerContainerLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("should allow Docker-style IDs and names")
	}
	if gotAsset != "srv1" || gotContainer != "web.1_abc-2" || gotTail != 5 {
		t.Fatalf("typed Docker log call asset=%q container=%q tail=%d", gotAsset, gotContainer, gotTail)
	}
}

func TestHandleDockerHosts_NilDep(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"docker:read"} }
	deps.ListDockerHosts = nil
	result, err := deps.handleDockerHosts(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when ListDockerHosts is nil")
	}
}

func TestHandleMetricsOverview_NilDep(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"metrics:read"} }
	deps.MetricsOverview = nil
	result, err := deps.handleMetricsOverview(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when MetricsOverview is nil")
	}
}

func TestHandleDockerHosts_WithData(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"docker:read"} }
	deps.ListDockerHosts = func(ctx context.Context) ([]map[string]any, error) {
		return []map[string]any{{"agent_id": "agent1", "containers": 3}}, nil
	}
	result, err := deps.handleDockerHosts(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("should not return error when dep is configured")
	}
}

func TestHandleDockerHostsFiltersAllowedAssets(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"docker:read"} }
	deps.GetAllowedAssets = func(ctx context.Context) []string { return []string{"srv1"} }
	deps.ListDockerHosts = func(ctx context.Context) ([]map[string]any, error) {
		return []map[string]any{
			{"agent_id": "srv1", "containers": 1},
			{"agent_id": "srv2", "containers": 2},
		}, nil
	}

	result, err := deps.handleDockerHosts(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("should not return error when dep is configured")
	}
	text := toolResultText(t, result)
	if !strings.Contains(text, "srv1") {
		t.Fatalf("expected allowed host in response, got %s", text)
	}
	if strings.Contains(text, "srv2") {
		t.Fatalf("response leaked disallowed docker host: %s", text)
	}
}

func TestHandleAlertsList_WithData(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"alerts:read"} }
	deps.ListAlerts = func(context.Context) ([]map[string]any, error) {
		return []map[string]any{{"id": "alert1", "status": "firing"}}, nil
	}
	result, err := deps.handleAlertsList(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("should not return error when dep is configured")
	}
}
