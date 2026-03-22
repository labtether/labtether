package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/terminal"
)

// Deps holds dependencies injected from the hub.
type Deps struct {
	AssetStore interface {
		ListAssets() ([]assets.Asset, error)
		GetAsset(id string) (assets.Asset, bool, error)
	}
	AgentMgr interface {
		IsConnected(assetID string) bool
	}
	ExecuteViaAgent func(job terminal.CommandJob) terminal.CommandResult
	// Scope/asset context for the current request.
	GetScopes        func(ctx context.Context) []string
	GetAllowedAssets func(ctx context.Context) []string
	GetActorID       func(ctx context.Context) string

	// Optional hub-internal dependencies. When nil, the relevant tool returns
	// errNotConfigured rather than panicking.
	ListDockerHosts        func() ([]map[string]any, error)
	ListDockerContainers   func(hostID string) ([]map[string]any, error)
	RestartDockerContainer func(containerID string) error
	ListAlerts             func() ([]map[string]any, error)
	AcknowledgeAlert       func(alertID string) error
	ListGroups             func() ([]map[string]any, error)
	MetricsOverview        func() (map[string]any, error)

	// Operational store closures.
	ListSchedules          func(ctx context.Context) ([]map[string]any, error)
	ListWebhooks           func(ctx context.Context) ([]map[string]any, error)
	ListSavedActions       func(ctx context.Context) ([]map[string]any, error)
	ListCredentialProfiles func(ctx context.Context) ([]map[string]any, error)
	GetEdgesForAsset       func(ctx context.Context, assetID string) ([]map[string]any, error)
	ListUpdatePlans        func(ctx context.Context) ([]map[string]any, error)

	// Connector health closure.
	ConnectorsHealth func(ctx context.Context) ([]map[string]any, error)
}

// NewServer creates and returns an MCP server with all LabTether tools.
func NewServer(deps *Deps) *server.MCPServer {
	s := server.NewMCPServer(
		"LabTether",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, false),
	)

	// --- Core tools ---

	s.AddTool(
		mcp.NewTool("whoami",
			mcp.WithDescription("Show what this API key has access to: scopes, allowed assets, and their online status"),
		),
		deps.handleWhoami,
	)

	s.AddTool(
		mcp.NewTool("assets_list",
			mcp.WithDescription("List all managed assets (servers, VMs, containers) with their status"),
			mcp.WithString("status", mcp.Description("Filter by status: online, offline")),
			mcp.WithString("platform", mcp.Description("Filter by platform: linux, windows, freebsd, darwin")),
		),
		deps.handleAssetsList,
	)

	s.AddTool(
		mcp.NewTool("assets_get",
			mcp.WithDescription("Get detailed information about a specific asset"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset ID")),
		),
		deps.handleAssetsGet,
	)

	s.AddTool(
		mcp.NewTool("exec",
			mcp.WithDescription("Run a command on a managed asset and return the output. The asset must be online with a connected agent."),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to run the command on")),
			mcp.WithString("command", mcp.Required(), mcp.Description("The shell command to execute")),
			mcp.WithNumber("timeout", mcp.Description("Max seconds to wait (default 30, max 300)")),
		),
		deps.handleExec,
	)

	s.AddTool(
		mcp.NewTool("exec_multi",
			mcp.WithDescription("Run a command on multiple assets in parallel and return aggregated results"),
			mcp.WithArray("targets", mcp.Description("List of asset IDs to run the command on")),
			mcp.WithString("command", mcp.Required(), mcp.Description("The shell command to execute")),
			mcp.WithNumber("timeout", mcp.Description("Max seconds to wait per target (default 30, max 300)")),
		),
		deps.handleExecMulti,
	)

	// --- Services ---

	s.AddTool(
		mcp.NewTool("services_list",
			mcp.WithDescription("List running system services on an asset (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to query")),
		),
		deps.handleServicesList,
	)

	s.AddTool(
		mcp.NewTool("services_restart",
			mcp.WithDescription("Restart a named system service on an asset (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to act on")),
			mcp.WithString("service_name", mcp.Required(), mcp.Description("The service to restart, e.g. nginx")),
		),
		deps.handleServicesRestart,
	)

	// --- Files ---

	s.AddTool(
		mcp.NewTool("files_list",
			mcp.WithDescription("List files in a directory on an asset (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to query")),
			mcp.WithString("path", mcp.Required(), mcp.Description("Absolute directory path to list")),
		),
		deps.handleFilesList,
	)

	s.AddTool(
		mcp.NewTool("files_read",
			mcp.WithDescription("Read the contents of a file on an asset (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to query")),
			mcp.WithString("path", mcp.Required(), mcp.Description("Absolute path to the file")),
		),
		deps.handleFilesRead,
	)

	// --- Docker ---

	s.AddTool(
		mcp.NewTool("docker_hosts",
			mcp.WithDescription("List Docker hosts managed by the hub"),
		),
		deps.handleDockerHosts,
	)

	s.AddTool(
		mcp.NewTool("docker_containers",
			mcp.WithDescription("List containers on a specific Docker host"),
			mcp.WithString("host_id", mcp.Required(), mcp.Description("The Docker host agent ID")),
		),
		deps.handleDockerContainers,
	)

	s.AddTool(
		mcp.NewTool("docker_container_restart",
			mcp.WithDescription("Restart a Docker container"),
			mcp.WithString("container_id", mcp.Required(), mcp.Description("The container ID or name")),
		),
		deps.handleDockerContainerRestart,
	)

	// --- System Info ---

	s.AddTool(
		mcp.NewTool("system_processes",
			mcp.WithDescription("List running processes on an asset, sorted by CPU usage (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to query")),
		),
		deps.handleSystemProcesses,
	)

	s.AddTool(
		mcp.NewTool("system_network",
			mcp.WithDescription("Get network interface information on an asset (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to query")),
		),
		deps.handleSystemNetwork,
	)

	s.AddTool(
		mcp.NewTool("system_disks",
			mcp.WithDescription("Get disk usage on an asset (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to query")),
		),
		deps.handleSystemDisks,
	)

	s.AddTool(
		mcp.NewTool("system_packages",
			mcp.WithDescription("List installed packages on an asset (requires connected agent; tries dpkg, rpm, pkg, brew)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to query")),
		),
		deps.handleSystemPackages,
	)

	// --- Alerts ---

	s.AddTool(
		mcp.NewTool("alerts_list",
			mcp.WithDescription("List active alert instances across the fleet"),
		),
		deps.handleAlertsList,
	)

	s.AddTool(
		mcp.NewTool("alerts_acknowledge",
			mcp.WithDescription("Acknowledge an alert instance"),
			mcp.WithString("alert_id", mcp.Required(), mcp.Description("The alert instance ID to acknowledge")),
		),
		deps.handleAlertsAcknowledge,
	)

	// --- Power ---

	s.AddTool(
		mcp.NewTool("asset_reboot",
			mcp.WithDescription("Reboot an asset (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to reboot")),
		),
		deps.handleAssetReboot,
	)

	s.AddTool(
		mcp.NewTool("asset_shutdown",
			mcp.WithDescription("Shut down an asset (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to shut down")),
		),
		deps.handleAssetShutdown,
	)

	s.AddTool(
		mcp.NewTool("asset_wake",
			mcp.WithDescription("Wake an asset via Wake-on-LAN"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to wake")),
		),
		deps.handleAssetWake,
	)

	// --- Groups ---

	s.AddTool(
		mcp.NewTool("groups_list",
			mcp.WithDescription("List asset groups"),
		),
		deps.handleGroupsList,
	)

	// --- Metrics ---

	s.AddTool(
		mcp.NewTool("metrics_overview",
			mcp.WithDescription("Get a fleet-wide metrics overview"),
		),
		deps.handleMetricsOverview,
	)

	// --- Operations ---

	s.AddTool(
		mcp.NewTool("schedules_list",
			mcp.WithDescription("List scheduled tasks configured in the hub"),
		),
		deps.handleSchedulesList,
	)

	s.AddTool(
		mcp.NewTool("webhooks_list",
			mcp.WithDescription("List webhook subscriptions configured in the hub"),
		),
		deps.handleWebhooksList,
	)

	s.AddTool(
		mcp.NewTool("saved_actions_list",
			mcp.WithDescription("List saved action sequences stored in the hub"),
		),
		deps.handleSavedActionsList,
	)

	s.AddTool(
		mcp.NewTool("credentials_list",
			mcp.WithDescription("List credential profiles stored in the hub (secrets are never returned)"),
		),
		deps.handleCredentialsList,
	)

	s.AddTool(
		mcp.NewTool("topology_edges",
			mcp.WithDescription("List dependency edges for an asset in the topology graph"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset to query edges for")),
		),
		deps.handleTopologyEdges,
	)

	s.AddTool(
		mcp.NewTool("updates_list_plans",
			mcp.WithDescription("List update plans in the hub"),
		),
		deps.handleUpdatesListPlans,
	)

	// --- Connectors ---

	s.AddTool(
		mcp.NewTool("connectors_health",
			mcp.WithDescription("Get health status of all registered connectors (Proxmox, TrueNAS, Portainer, etc.)"),
		),
		deps.handleConnectorsHealth,
	)

	s.AddTool(
		mcp.NewTool("docker_container_logs",
			mcp.WithDescription("Get logs for a Docker container on an asset (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset running Docker")),
			mcp.WithString("container_id", mcp.Required(), mcp.Description("The container ID or name")),
			mcp.WithNumber("tail", mcp.Description("Number of log lines to return (default 100, max 10000)")),
		),
		deps.handleDockerContainerLogs,
	)

	s.AddTool(
		mcp.NewTool("docker_container_stats",
			mcp.WithDescription("Get resource usage stats for a Docker container on an asset (requires connected agent)"),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("The asset running Docker")),
			mcp.WithString("container_id", mcp.Required(), mcp.Description("The container ID or name")),
		),
		deps.handleDockerContainerStats,
	)

	// --- Resources ---

	s.AddResource(
		mcp.NewResource(
			"labtether://assets",
			"Asset Inventory",
			mcp.WithResourceDescription("Current asset inventory with online/offline status"),
			mcp.WithMIMEType("application/json"),
		),
		deps.handleAssetsResource,
	)

	s.AddResource(
		mcp.NewResource(
			"labtether://alerts/active",
			"Active Alerts",
			mcp.WithResourceDescription("Currently active (unresolved) alert instances as JSON"),
			mcp.WithMIMEType("application/json"),
		),
		deps.handleActiveAlertsResource,
	)

	s.AddResource(
		mcp.NewResource(
			"labtether://groups",
			"Groups",
			mcp.WithResourceDescription("Asset group structure as JSON"),
			mcp.WithMIMEType("application/json"),
		),
		deps.handleGroupsResource,
	)

	// TODO: Expand MCP tools to cover full API surface:
	// - File write/delete/rename/copy/mkdir
	// - Services start/stop (not just restart)
	// - Processes kill
	// - Docker full suite (start/stop/exec/stacks/images/volumes/networks)
	// - Connector-specific tools (Proxmox VM ops, TrueNAS dataset ops, PBS backup ops)
	// - Updates (runs, apply)
	// - Topology blast-radius and upstream-causes queries
	// - Discovery (run, proposals)
	// - Agents (lifecycle, settings)
	// - Collectors, web services
	// - Search, audit, settings
	//
	// TODO: Add MCP resources:
	// - labtether://assets/{id} (per-asset detail)
	// - labtether://metrics/overview (fleet health)

	return s
}

// --- Tool Handlers ---

func (d *Deps) scopeCheck(ctx context.Context, scope string) error {
	scopes := d.GetScopes(ctx)
	if scopes == nil {
		return nil // session auth — full access
	}
	if !apikeys.ScopeAllows(scopes, scope) {
		return fmt.Errorf("insufficient scope: %s required", scope)
	}
	return nil
}

func (d *Deps) assetCheck(ctx context.Context, assetID string) error {
	allowed := d.GetAllowedAssets(ctx)
	if !apikeys.AssetAllowed(allowed, assetID) {
		return fmt.Errorf("access denied to asset: %s", assetID)
	}
	return nil
}

func (d *Deps) handleWhoami(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	scopes := d.GetScopes(ctx)
	allowed := d.GetAllowedAssets(ctx)

	allAssets, _ := d.AssetStore.ListAssets()
	var accessibleAssets []map[string]any
	for _, a := range allAssets {
		if !apikeys.AssetAllowed(allowed, a.ID) {
			continue
		}
		accessibleAssets = append(accessibleAssets, map[string]any{
			"id": a.ID, "name": a.Name, "platform": a.Platform,
			"status": a.Status, "online": a.Status == "online",
		})
	}

	result := map[string]any{
		"scopes":           scopes,
		"allowed_assets":   allowed,
		"available_assets": accessibleAssets,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (d *Deps) handleAssetsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "assets:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		return mcp.NewToolResultError("failed to list assets"), nil
	}

	allowed := d.GetAllowedAssets(ctx)
	statusFilter := req.GetString("status", "")
	platformFilter := req.GetString("platform", "")

	var filtered []map[string]any
	for _, a := range allAssets {
		if !apikeys.AssetAllowed(allowed, a.ID) {
			continue
		}
		if statusFilter != "" && !strings.EqualFold(a.Status, statusFilter) {
			continue
		}
		if platformFilter != "" && !strings.EqualFold(a.Platform, platformFilter) {
			continue
		}
		filtered = append(filtered, map[string]any{
			"id": a.ID, "name": a.Name, "platform": a.Platform,
			"status": a.Status, "type": a.Type, "source": a.Source,
		})
	}

	data, _ := json.MarshalIndent(filtered, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (d *Deps) handleAssetsGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "assets:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	assetID, _ := req.RequireString("asset_id")
	if err := d.assetCheck(ctx, assetID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	asset, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		return mcp.NewToolResultError("failed to load asset"), nil
	}
	if !ok {
		return mcp.NewToolResultError("asset not found: " + assetID), nil
	}

	result := map[string]any{
		"asset":           asset,
		"agent_connected": d.AgentMgr != nil && d.AgentMgr.IsConnected(assetID),
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (d *Deps) handleExec(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "assets:exec"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	assetID, _ := req.RequireString("asset_id")
	command, _ := req.RequireString("command")
	timeout := req.GetInt("timeout", 30)
	if timeout > 300 {
		timeout = 300
	}

	if err := d.assetCheck(ctx, assetID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		asset, ok, _ := d.AssetStore.GetAsset(assetID)
		msg := assetID + " agent is not connected"
		if ok {
			msg = fmt.Sprintf("%s agent is not connected (last seen: %s)", assetID, asset.LastSeenAt.Format(time.RFC3339))
		}
		return mcp.NewToolResultError(msg), nil
	}

	// NOTE: The timeout parameter is validated but not currently enforced at
	// the MCP layer. The agent execution timeout is controlled by the hub's
	// agent manager. This parameter is reserved for future use when the
	// agent protocol supports per-command deadlines.
	cmdResult := d.ExecuteViaAgent(terminal.CommandJob{
		JobID:       idgen.New("mcp"),
		SessionID:   idgen.New("mcps"),
		CommandID:   idgen.New("mcpc"),
		ActorID:     d.GetActorID(ctx),
		Target:      assetID,
		Command:     command,
		Mode:        "structured",
		RequestedAt: time.Now().UTC(),
	})

	exitCode := 0
	if !strings.EqualFold(strings.TrimSpace(cmdResult.Status), "succeeded") {
		exitCode = 1
	}

	result := map[string]any{
		"asset_id":  assetID,
		"exit_code": exitCode,
		"output":    strings.TrimSpace(cmdResult.Output),
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (d *Deps) handleExecMulti(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "assets:exec"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	command, _ := req.RequireString("command")
	timeout := req.GetInt("timeout", 30)
	if timeout > 300 {
		timeout = 300
	}

	// Extract targets array
	args := req.GetArguments()
	targetsRaw, ok := args["targets"]
	if !ok {
		return mcp.NewToolResultError("targets is required"), nil
	}
	targetsSlice, ok := targetsRaw.([]any)
	if !ok {
		return mcp.NewToolResultError("targets must be an array of strings"), nil
	}

	allowed := d.GetAllowedAssets(ctx)
	var targets []string
	var invalidCount int
	for _, t := range targetsSlice {
		s, ok := t.(string)
		if !ok {
			invalidCount++
			continue
		}
		if apikeys.AssetAllowed(allowed, s) {
			targets = append(targets, s)
		}
	}

	if len(targets) == 0 {
		if invalidCount > 0 {
			return mcp.NewToolResultError(fmt.Sprintf("no valid targets: %d entries were not strings", invalidCount)), nil
		}
		return mcp.NewToolResultError("no accessible targets provided"), nil
	}

	results := make(map[string]any)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			if d.AgentMgr == nil || !d.AgentMgr.IsConnected(t) {
				mu.Lock()
				results[t] = map[string]any{"error": "asset_offline"}
				mu.Unlock()
				return
			}
			// NOTE: The timeout parameter is validated but not currently enforced at
			// the MCP layer. The agent execution timeout is controlled by the hub's
			// agent manager. This parameter is reserved for future use when the
			// agent protocol supports per-command deadlines.
			cmdResult := d.ExecuteViaAgent(terminal.CommandJob{
				JobID:       idgen.New("mcp"),
				SessionID:   idgen.New("mcps"),
				CommandID:   idgen.New("mcpc"),
				ActorID:     d.GetActorID(ctx),
				Target:      t,
				Command:     command,
				Mode:        "structured",
				RequestedAt: time.Now().UTC(),
			})
			exitCode := 0
			if !strings.EqualFold(strings.TrimSpace(cmdResult.Status), "succeeded") {
				exitCode = 1
			}
			mu.Lock()
			results[t] = map[string]any{
				"exit_code": exitCode,
				"output":    strings.TrimSpace(cmdResult.Output),
			}
			mu.Unlock()
		}(target)
	}
	wg.Wait()

	data, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- Resource Handlers ---

func (d *Deps) handleAssetsResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		return nil, err
	}

	allowed := d.GetAllowedAssets(ctx)
	var accessible []map[string]any
	for _, a := range allAssets {
		if !apikeys.AssetAllowed(allowed, a.ID) {
			continue
		}
		accessible = append(accessible, map[string]any{
			"id": a.ID, "name": a.Name, "platform": a.Platform,
			"status": a.Status, "last_seen": a.LastSeenAt,
		})
	}

	data, _ := json.Marshal(accessible)
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

func (d *Deps) handleActiveAlertsResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	var payload []map[string]any
	if d.ListAlerts != nil {
		alerts, err := d.ListAlerts()
		if err != nil {
			return nil, err
		}
		payload = alerts
	}
	if payload == nil {
		payload = []map[string]any{}
	}
	data, _ := json.Marshal(payload)
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

func (d *Deps) handleGroupsResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	var payload []map[string]any
	if d.ListGroups != nil {
		groups, err := d.ListGroups()
		if err != nil {
			return nil, err
		}
		payload = groups
	}
	if payload == nil {
		payload = []map[string]any{}
	}
	data, _ := json.Marshal(payload)
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
