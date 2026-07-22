package mcpserver

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/labtether/labtether/internal/terminal"
)

func toolRequest(arguments map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = arguments
	return req
}

func TestAdvertisedMCPInventoryIsCovered(t *testing.T) {
	server := NewServer(newTestDeps())
	expectedTools := []string{
		"whoami", "assets_list", "assets_get", "exec", "exec_multi",
		"services_list", "services_restart", "files_list", "files_read",
		"docker_hosts", "docker_containers", "docker_container_restart",
		"system_processes", "system_network", "system_disks", "system_packages",
		"alerts_list", "alerts_acknowledge", "asset_reboot", "asset_shutdown", "asset_wake",
		"groups_list", "metrics_overview", "schedules_list", "webhooks_list",
		"saved_actions_list", "credentials_list", "topology_edges", "updates_list_plans",
		"connectors_health", "docker_container_logs", "docker_container_stats",
	}
	tools := server.ListTools()
	if len(tools) != len(expectedTools) {
		t.Fatalf("advertised tool count=%d want %d", len(tools), len(expectedTools))
	}
	for _, name := range expectedTools {
		if tools[name] == nil {
			t.Errorf("advertised tool %q is missing", name)
		}
	}
	expectedResources := []string{"labtether://assets", "labtether://alerts/active", "labtether://groups"}
	resources := server.ListResources()
	if len(resources) != len(expectedResources) {
		t.Fatalf("advertised resource count=%d want %d", len(resources), len(expectedResources))
	}
	for _, uri := range expectedResources {
		if _, ok := resources[uri]; !ok {
			t.Errorf("advertised resource %q is missing", uri)
		}
	}
}

func TestAssetRestrictedAuthorizationPlanCoversEveryAdvertisedSurface(t *testing.T) {
	// Every advertised surface must declare the authorization boundary that
	// protects it. This deliberately fails when a new tool or resource is added
	// without being included in the asset-restricted security audit.
	toolBoundaries := map[string]string{
		"whoami": "direct_filter", "assets_list": "direct_filter", "docker_hosts": "direct_filter",
		"assets_get": "direct_asset", "services_list": "direct_asset", "files_list": "direct_asset",
		"files_read": "direct_asset", "system_processes": "direct_asset", "system_network": "direct_asset",
		"system_disks": "direct_asset", "system_packages": "direct_asset", "topology_edges": "direct_asset",
		"docker_container_logs": "direct_asset", "docker_container_stats": "direct_asset",
		"docker_containers": "production_asset_resolution",
		"exec":              "mutation_policy", "exec_multi": "mutation_policy", "services_restart": "mutation_policy",
		"docker_container_restart": "mutation_policy", "alerts_acknowledge": "mutation_policy",
		"asset_reboot": "mutation_policy", "asset_shutdown": "mutation_policy", "asset_wake": "mutation_policy",
		"alerts_list": "production_filter", "groups_list": "production_filter", "metrics_overview": "production_filter",
		"schedules_list": "production_filter", "saved_actions_list": "production_filter", "updates_list_plans": "production_filter",
		"webhooks_list": "restricted_global_denial", "credentials_list": "restricted_global_denial",
		"connectors_health": "restricted_global_denial",
	}
	resourceBoundaries := map[string]string{
		"labtether://assets":        "direct_filter",
		"labtether://alerts/active": "production_filter",
		"labtether://groups":        "production_filter",
	}

	server := NewServer(newTestDeps())
	tools := server.ListTools()
	if len(toolBoundaries) != len(tools) {
		t.Fatalf("authorization plan covers %d tools, advertised inventory has %d", len(toolBoundaries), len(tools))
	}
	for name := range tools {
		if strings.TrimSpace(toolBoundaries[name]) == "" {
			t.Errorf("advertised tool %q has no asset-restricted authorization boundary", name)
		}
	}
	resources := server.ListResources()
	if len(resourceBoundaries) != len(resources) {
		t.Fatalf("authorization plan covers %d resources, advertised inventory has %d", len(resourceBoundaries), len(resources))
	}
	for uri := range resources {
		if strings.TrimSpace(resourceBoundaries[uri]) == "" {
			t.Errorf("advertised resource %q has no asset-restricted authorization boundary", uri)
		}
	}
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

func TestEveryAdvertisedMCPToolExecutesWithConfiguredDependencies(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(context.Context) []string { return nil }
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		return terminal.CommandResult{Status: "succeeded", Output: job.Target + ":ok"}
	}
	deps.ExecutePowerAction = func(_ context.Context, assetID, action string) (string, error) {
		return action + " accepted for " + assetID, nil
	}
	deps.ListServices = func(context.Context, string) (any, error) { return map[string]any{"services": []any{}}, nil }
	deps.RestartService = func(context.Context, string, string) (any, error) { return map[string]any{"success": true}, nil }
	deps.ListFiles = func(context.Context, string, string) (any, error) { return map[string]any{"entries": []any{}}, nil }
	deps.ReadFile = func(context.Context, string, string) (any, error) { return map[string]any{"content": "ok"}, nil }
	deps.ListProcesses = func(context.Context, string) (any, error) { return map[string]any{"processes": []any{}}, nil }
	deps.ListNetwork = func(context.Context, string) (any, error) { return map[string]any{"interfaces": []any{}}, nil }
	deps.ListDisks = func(context.Context, string) (any, error) { return map[string]any{"disks": []any{}}, nil }
	deps.ListPackages = func(context.Context, string) (any, error) { return map[string]any{"packages": []any{}}, nil }
	deps.ListDockerHosts = func(context.Context) ([]map[string]any, error) { return []map[string]any{{"agent_id": "srv1"}}, nil }
	deps.ListDockerContainers = func(context.Context, string) ([]map[string]any, error) { return []map[string]any{{"id": "abc"}}, nil }
	deps.RestartDockerContainer = func(context.Context, string) error { return nil }
	deps.DockerContainerLogs = func(context.Context, string, string, int) (string, error) { return "logs", nil }
	deps.DockerContainerStats = func(context.Context, string, string) (map[string]any, error) {
		return map[string]any{"cpu_percent": 1}, nil
	}
	deps.ListAlerts = func(context.Context) ([]map[string]any, error) { return []map[string]any{{"id": "alert1"}}, nil }
	deps.AcknowledgeAlert = func(context.Context, string) error { return nil }
	deps.ListGroups = func(context.Context) ([]map[string]any, error) { return []map[string]any{{"id": "group1"}}, nil }
	deps.MetricsOverview = func(context.Context) (map[string]any, error) { return map[string]any{"assets": []any{}}, nil }
	deps.WakeAsset = func(context.Context, string) (map[string]any, error) { return map[string]any{"status": "sent"}, nil }
	deps.ListSchedules = func(context.Context) ([]map[string]any, error) { return []map[string]any{}, nil }
	deps.ListWebhooks = func(context.Context) ([]map[string]any, error) { return []map[string]any{}, nil }
	deps.ListSavedActions = func(context.Context) ([]map[string]any, error) { return []map[string]any{}, nil }
	deps.ListCredentialProfiles = func(context.Context) ([]map[string]any, error) { return []map[string]any{}, nil }
	deps.GetEdgesForAsset = func(context.Context, string) ([]map[string]any, error) { return []map[string]any{}, nil }
	deps.ListUpdatePlans = func(context.Context) ([]map[string]any, error) { return []map[string]any{}, nil }
	deps.ConnectorsHealth = func(context.Context) ([]map[string]any, error) { return []map[string]any{{"status": "ok"}}, nil }

	requests := map[string]map[string]any{
		"whoami": {}, "assets_list": {}, "assets_get": {"asset_id": "srv1"},
		"exec":          {"asset_id": "srv1", "command": "id"},
		"exec_multi":    {"targets": []any{"srv1"}, "command": "id"},
		"services_list": {"asset_id": "srv1"}, "services_restart": {"asset_id": "srv1", "service_name": "sshd"},
		"files_list": {"asset_id": "srv1", "path": "/tmp"}, "files_read": {"asset_id": "srv1", "path": "/tmp/a"},
		"docker_hosts": {}, "docker_containers": {"host_id": "srv1"}, "docker_container_restart": {"container_id": "abc"},
		"system_processes": {"asset_id": "srv1"}, "system_network": {"asset_id": "srv1"},
		"system_disks": {"asset_id": "srv1"}, "system_packages": {"asset_id": "srv1"},
		"alerts_list": {}, "alerts_acknowledge": {"alert_id": "alert1"},
		"asset_reboot": {"asset_id": "srv1"}, "asset_shutdown": {"asset_id": "srv1"}, "asset_wake": {"asset_id": "srv1"},
		"groups_list": {}, "metrics_overview": {}, "schedules_list": {}, "webhooks_list": {},
		"saved_actions_list": {}, "credentials_list": {}, "topology_edges": {"asset_id": "srv1"},
		"updates_list_plans": {}, "connectors_health": {},
		"docker_container_logs":  {"asset_id": "srv1", "container_id": "abc"},
		"docker_container_stats": {"asset_id": "srv1", "container_id": "abc"},
	}
	server := NewServer(deps)
	for name, arguments := range requests {
		t.Run(name, func(t *testing.T) {
			tool := server.GetTool(name)
			if tool == nil {
				t.Fatal("tool is not registered")
			}
			result, err := tool.Handler(context.Background(), toolRequest(arguments))
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || result.IsError {
				t.Fatalf("configured tool failed: %#v", result)
			}
		})
	}

	for uri, resource := range server.ListResources() {
		t.Run(uri, func(t *testing.T) {
			result, err := resource.Handler(context.Background(), mcp.ReadResourceRequest{Params: mcp.ReadResourceParams{URI: uri}})
			if err != nil || len(result) != 1 {
				t.Fatalf("configured resource failed: result=%#v err=%v", result, err)
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
