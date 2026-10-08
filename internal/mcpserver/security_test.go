package mcpserver

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/mark3labs/mcp-go/mcp"
	"strings"
	"sync/atomic"
	"testing"
)

func toolRequest(arguments map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = arguments
	return req
}

func TestAssetRestrictedContextFiltersDirectInventoriesAndAssetsResource(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return []string{"*"} }
	deps.GetAllowedAssets = func(context.Context) []string { return []string{"srv1"} }
	deps.ListDockerHosts = func(context.Context) ([]map[string]any, error) {
		return []map[string]any{
			{"agent_id": "srv1", "name": "allowed-host"},
			{"agent_id": "srv2", "name": "hidden-host-sentinel"},
		}, nil
	}

	server := NewServer(deps)
	for _, name := range []string{"whoami", "assets_list", "docker_hosts"} {
		t.Run(name, func(t *testing.T) {
			tool := server.GetTool(name)
			result, err := tool.Handler(context.Background(), toolRequest(map[string]any{}))
			if err != nil || result == nil || result.IsError {
				t.Fatalf("restricted inventory failed: result=%#v err=%v", result, err)
			}
			text := toolResultText(t, result)
			if !strings.Contains(text, "srv1") || strings.Contains(text, "srv2") || strings.Contains(text, "hidden-host-sentinel") {
				t.Fatalf("restricted inventory leaked or omitted an asset: %s", text)
			}
		})
	}

	resource := server.ListResources()["labtether://assets"]
	contents, err := resource.Handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "labtether://assets"},
	})
	if err != nil || len(contents) != 1 {
		t.Fatalf("restricted assets resource failed: contents=%#v err=%v", contents, err)
	}
	text, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("assets resource content type=%T", contents[0])
	}
	if !strings.Contains(text.Text, "srv1") || strings.Contains(text.Text, "srv2") {
		t.Fatalf("restricted assets resource leaked or omitted an asset: %s", text.Text)
	}
}

func TestAssetRestrictedContextDeniesEveryDirectAssetToolBeforeDispatch(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return []string{"*"} }
	deps.GetAllowedAssets = func(context.Context) []string { return []string{"srv1"} }
	var dispatches atomic.Int32
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		dispatches.Add(1)
		return terminal.CommandResult{Status: "succeeded"}
	}
	deps.ExecutePowerAction = func(context.Context, string, string) (string, error) {
		dispatches.Add(1)
		return "unexpected", nil
	}
	deps.ListServices = func(context.Context, string) (any, error) { dispatches.Add(1); return nil, nil }
	deps.RestartService = func(context.Context, string, string) (any, error) { dispatches.Add(1); return nil, nil }
	deps.ListFiles = func(context.Context, string, string) (any, error) { dispatches.Add(1); return nil, nil }
	deps.ReadFile = func(context.Context, string, string) (any, error) { dispatches.Add(1); return nil, nil }
	deps.ListProcesses = func(context.Context, string) (any, error) { dispatches.Add(1); return nil, nil }
	deps.ListNetwork = func(context.Context, string) (any, error) { dispatches.Add(1); return nil, nil }
	deps.ListDisks = func(context.Context, string) (any, error) { dispatches.Add(1); return nil, nil }
	deps.ListPackages = func(context.Context, string) (any, error) { dispatches.Add(1); return nil, nil }
	deps.GetEdgesForAsset = func(context.Context, string) ([]map[string]any, error) { dispatches.Add(1); return nil, nil }
	deps.DockerContainerLogs = func(context.Context, string, string, int) (string, error) { dispatches.Add(1); return "", nil }
	deps.DockerContainerStats = func(context.Context, string, string) (map[string]any, error) { dispatches.Add(1); return nil, nil }
	deps.WakeAsset = func(context.Context, string) (map[string]any, error) { dispatches.Add(1); return nil, nil }
	deps.AuthorizeMutation = func(context.Context, string, string) error {
		dispatches.Add(1)
		return nil
	}

	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "assets_get", args: map[string]any{"asset_id": "srv2"}},
		{name: "exec", args: map[string]any{"asset_id": "srv2", "command": "id"}},
		{name: "exec_multi", args: map[string]any{"targets": []any{"srv2"}, "command": "id"}},
		{name: "services_list", args: map[string]any{"asset_id": "srv2"}},
		{name: "services_restart", args: map[string]any{"asset_id": "srv2", "service_name": "sshd"}},
		{name: "files_list", args: map[string]any{"asset_id": "srv2", "path": "/tmp"}},
		{name: "files_read", args: map[string]any{"asset_id": "srv2", "path": "/tmp/a"}},
		{name: "system_processes", args: map[string]any{"asset_id": "srv2"}},
		{name: "system_network", args: map[string]any{"asset_id": "srv2"}},
		{name: "system_disks", args: map[string]any{"asset_id": "srv2"}},
		{name: "system_packages", args: map[string]any{"asset_id": "srv2"}},
		{name: "asset_reboot", args: map[string]any{"asset_id": "srv2"}},
		{name: "asset_shutdown", args: map[string]any{"asset_id": "srv2"}},
		{name: "asset_wake", args: map[string]any{"asset_id": "srv2"}},
		{name: "topology_edges", args: map[string]any{"asset_id": "srv2"}},
		{name: "docker_container_logs", args: map[string]any{"asset_id": "srv2", "container_id": "hidden"}},
		{name: "docker_container_stats", args: map[string]any{"asset_id": "srv2", "container_id": "hidden"}},
	}

	server := NewServer(deps)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := server.GetTool(tc.name).Handler(context.Background(), toolRequest(tc.args))
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("out-of-scope target was not denied: %#v", result)
			}
		})
	}
	if got := dispatches.Load(); got != 0 {
		t.Fatalf("out-of-scope tools reached a dependency or mutation policy %d times", got)
	}
}

func TestEveryAdvertisedMutationInvokesPolicyBeforeDispatch(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "exec", args: map[string]any{"asset_id": "srv1", "command": "id"}},
		{name: "exec_multi", args: map[string]any{"targets": []any{"srv1"}, "command": "id"}},
		{name: "services_restart", args: map[string]any{"asset_id": "srv1", "service_name": "sshd"}},
		{name: "docker_container_restart", args: map[string]any{"container_id": "abc"}},
		{name: "alerts_acknowledge", args: map[string]any{"alert_id": "alert1"}},
		{name: "asset_reboot", args: map[string]any{"asset_id": "srv1"}},
		{name: "asset_shutdown", args: map[string]any{"asset_id": "srv1"}},
		{name: "asset_wake", args: map[string]any{"asset_id": "srv1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestDeps()
			deps.GetScopes = func(context.Context) []string { return []string{"*"} }
			var policyCalls atomic.Int32
			var dispatches atomic.Int32
			deps.AuthorizeMutation = func(_ context.Context, tool, _ string) error {
				if tool != tc.name {
					t.Errorf("policy tool=%q want %q", tool, tc.name)
				}
				policyCalls.Add(1)
				return errors.New("MCP mutations require operator role")
			}
			deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult { dispatches.Add(1); return terminal.CommandResult{} }
			deps.ExecutePowerAction = func(context.Context, string, string) (string, error) { dispatches.Add(1); return "", nil }
			deps.RestartService = func(context.Context, string, string) (any, error) { dispatches.Add(1); return nil, nil }
			deps.RestartDockerContainer = func(context.Context, string) error { dispatches.Add(1); return nil }
			deps.AcknowledgeAlert = func(context.Context, string) error { dispatches.Add(1); return nil }
			deps.WakeAsset = func(context.Context, string) (map[string]any, error) { dispatches.Add(1); return nil, nil }

			result, err := NewServer(deps).GetTool(tc.name).Handler(context.Background(), toolRequest(tc.args))
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("viewer-equivalent policy denial was not returned: %#v", result)
			}
			if policyCalls.Load() != 1 {
				t.Fatalf("policy calls=%d want 1", policyCalls.Load())
			}
			if dispatches.Load() != 0 {
				t.Fatalf("denied mutation dispatched %d times", dispatches.Load())
			}
		})
	}
}

func TestExecFailureIsReportedAsMCPErrorAndAudited(t *testing.T) {
	deps := newTestDeps()
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		return terminal.CommandResult{Status: "failed", Output: "permission denied"}
	}
	var decision, reason string
	deps.AuditMutation = func(_ context.Context, _, _, gotDecision, gotReason string, _ map[string]any) {
		decision, reason = gotDecision, gotReason
	}
	result, err := deps.handleExec(context.Background(), toolRequest(map[string]any{
		"asset_id": "srv1",
		"command":  "id",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("failed command must be an MCP error: %#v", result)
	}
	if decision != "failed" || reason != "command_failed" {
		t.Fatalf("audit decision=%q reason=%q", decision, reason)
	}
}

func TestExecMutationDenialPreventsDispatchAndAudits(t *testing.T) {
	deps := newTestDeps()
	deps.AuthorizeMutation = func(context.Context, string, string) error {
		return errors.New("actions are blocked by active maintenance windows")
	}
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		t.Fatalf("denied command dispatched: %#v", job)
		return terminal.CommandResult{}
	}
	var decision, reason string
	deps.AuditMutation = func(_ context.Context, _, _, gotDecision, gotReason string, _ map[string]any) {
		decision, reason = gotDecision, gotReason
	}
	result, err := deps.handleExec(context.Background(), toolRequest(map[string]any{
		"asset_id": "srv1",
		"command":  "id",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("denied command must be an MCP error: %#v", result)
	}
	if decision != "denied" || reason != "maintenance_blocked" {
		t.Fatalf("audit decision=%q reason=%q", decision, reason)
	}
}

func TestExecMultiAllFailedIsReportedAsMCPError(t *testing.T) {
	deps := newTestDeps()
	deps.AuthorizeMutation = func(context.Context, string, string) error {
		return errors.New("MCP mutation rate limit exceeded")
	}
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		t.Fatalf("denied batch command dispatched: %#v", job)
		return terminal.CommandResult{}
	}
	result, err := deps.handleExecMulti(context.Background(), toolRequest(map[string]any{
		"targets": []any{"srv1"},
		"command": "id",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError || !strings.Contains(toolResultText(t, result), "rate_limited") {
		t.Fatalf("all-failed batch must be an MCP error: %#v", result)
	}
}

func TestMutationPolicyFailsClosedWhenMissing(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return []string{"assets:power"} }
	deps.AuthorizeMutation = nil
	deps.ExecutePowerAction = func(context.Context, string, string) (string, error) {
		t.Fatal("power action dispatched without mutation policy")
		return "", nil
	}
	result, err := deps.handleAssetReboot(context.Background(), toolRequest(map[string]any{"asset_id": "srv1"}))
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError || !strings.Contains(toolResultText(t, result), "policy is unavailable") {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestFilesReadPassesPathOnlyToTypedDependency(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return []string{"files:read"} }
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		t.Fatalf("file path reached command executor: %q", job.Command)
		return terminal.CommandResult{}
	}
	path := "/tmp/report;$(touch pwn).txt"
	var gotPath string
	deps.ReadFile = func(_ context.Context, assetID, value string) (any, error) {
		if assetID != "srv1" {
			t.Fatalf("assetID=%q", assetID)
		}
		gotPath = value
		return map[string]any{"content": "safe"}, nil
	}
	result, err := deps.handleFilesRead(context.Background(), toolRequest(map[string]any{"asset_id": "srv1", "path": path}))
	if err != nil || result == nil || result.IsError {
		t.Fatalf("typed file read failed: err=%v result=%#v", err, result)
	}
	if gotPath != path {
		t.Fatalf("path=%q want %q", gotPath, path)
	}
}

func TestAssetWakeExecutesRealDependency(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return []string{"assets:power"} }
	called := false
	deps.WakeAsset = func(_ context.Context, assetID string) (map[string]any, error) {
		called = true
		return map[string]any{"status": "sent", "asset_id": assetID}, nil
	}
	result, err := deps.handleAssetWake(context.Background(), toolRequest(map[string]any{"asset_id": "srv1"}))
	if err != nil || result == nil || result.IsError {
		t.Fatalf("wake failed: err=%v result=%#v", err, result)
	}
	if !called || !strings.Contains(toolResultText(t, result), `"status": "sent"`) {
		t.Fatalf("wake dependency not reflected in result: %s", toolResultText(t, result))
	}
}

func TestMetricsOverviewExecutesDependency(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return []string{"metrics:read"} }
	deps.MetricsOverview = func(context.Context) (map[string]any, error) {
		return map[string]any{"assets": []any{map[string]any{"asset_id": "srv1"}}}, nil
	}
	result, err := deps.handleMetricsOverview(context.Background(), mcp.CallToolRequest{})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("metrics failed: err=%v result=%#v", err, result)
	}
	if !strings.Contains(toolResultText(t, result), "srv1") {
		t.Fatalf("metrics response=%s", toolResultText(t, result))
	}
}

func TestResourcesFailClosedWhenDependencyMissing(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return []string{"alerts:read", "groups:read"} }
	if _, err := deps.handleActiveAlertsResource(context.Background(), mcp.ReadResourceRequest{}); !errors.Is(err, errMCPDependencyUnavailable) {
		t.Fatalf("alerts resource error=%v", err)
	}
	if _, err := deps.handleGroupsResource(context.Background(), mcp.ReadResourceRequest{}); !errors.Is(err, errMCPDependencyUnavailable) {
		t.Fatalf("groups resource error=%v", err)
	}
}

func TestAssetRestrictedKeysCannotInvokeGlobalMCPReads(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string {
		return []string{"webhooks:read", "credentials:read", "connectors:read"}
	}
	deps.GetAllowedAssets = func(context.Context) []string { return []string{"srv1"} }
	called := 0
	deps.ListWebhooks = func(context.Context) ([]map[string]any, error) {
		called++
		return []map[string]any{{"id": "webhook1"}}, nil
	}
	deps.ListCredentialProfiles = func(context.Context) ([]map[string]any, error) {
		called++
		return []map[string]any{{"id": "credential1"}}, nil
	}
	deps.ConnectorsHealth = func(context.Context) ([]map[string]any, error) {
		called++
		return []map[string]any{{"id": "connector1"}}, nil
	}

	tests := []struct {
		name string
		call func() (*mcp.CallToolResult, error)
	}{
		{name: "webhooks", call: func() (*mcp.CallToolResult, error) {
			return deps.handleWebhooksList(context.Background(), mcp.CallToolRequest{})
		}},
		{name: "credentials", call: func() (*mcp.CallToolResult, error) {
			return deps.handleCredentialsList(context.Background(), mcp.CallToolRequest{})
		}},
		{name: "connector health", call: func() (*mcp.CallToolResult, error) {
			return deps.handleConnectorsHealth(context.Background(), mcp.CallToolRequest{})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || !result.IsError || !strings.Contains(toolResultText(t, result), "asset-restricted") {
				t.Fatalf("unexpected result: %#v", result)
			}
		})
	}
	if called != 0 {
		t.Fatalf("global dependency was invoked %d times", called)
	}
}

func TestCollectionLimitRejectsOversizedResult(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return []string{"alerts:read"} }
	deps.ListAlerts = func(context.Context) ([]map[string]any, error) {
		return make([]map[string]any, maxMCPCollectionItems+1), nil
	}
	result, err := deps.handleAlertsList(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError || !strings.Contains(toolResultText(t, result), "MCP limit") {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestBoundedToolTextNeverExceedsLimit(t *testing.T) {
	result := boundedToolText(strings.Repeat("x", maxMCPTextBytes+1024))
	if got := len(toolResultText(t, result)); got > maxMCPTextBytes {
		t.Fatalf("bounded text length=%d limit=%d", got, maxMCPTextBytes)
	}
}
