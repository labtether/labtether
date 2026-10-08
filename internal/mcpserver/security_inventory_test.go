package mcpserver

import (
	"context"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/mark3labs/mcp-go/mcp"
	"strings"
	"testing"
)

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
