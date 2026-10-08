package mcpserver

import (
	"context"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
	// ExecutePowerAction uses the dedicated typed agent power protocol. It must
	// never be implemented through ExecuteViaAgent/raw shell.
	ExecutePowerAction func(ctx context.Context, assetID, action string) (string, error)
	// Scope/asset context for the current request.
	GetScopes        func(ctx context.Context) []string
	GetAllowedAssets func(ctx context.Context) []string
	GetActorID       func(ctx context.Context) string
	// AuthorizeMutation enforces maintenance and bounded admission immediately
	// before an MCP mutation is dispatched. Mutations fail closed when it is nil.
	AuthorizeMutation func(ctx context.Context, tool, target string) error
	// EvaluateCommandPolicy applies the Hub's live structured-command policy.
	// Raw MCP command tools fail closed when it is nil.
	EvaluateCommandPolicy func(ctx context.Context, assetID, command string) policy.CheckResponse
	// AuditMutation records redacted, principal-attributed mutation outcomes.
	AuditMutation func(ctx context.Context, tool, target, decision, reason string, details map[string]any)

	// Typed agent operations. These deliberately avoid ExecuteViaAgent because
	// the endpoint command policy rejects shell expressions and because each
	// capability has its own correlated wire protocol.
	ListServices   func(ctx context.Context, assetID string) (any, error)
	RestartService func(ctx context.Context, assetID, serviceName string) (any, error)
	ListFiles      func(ctx context.Context, assetID, path string) (any, error)
	ReadFile       func(ctx context.Context, assetID, path string) (any, error)
	ListProcesses  func(ctx context.Context, assetID string) (any, error)
	ListNetwork    func(ctx context.Context, assetID string) (any, error)
	ListDisks      func(ctx context.Context, assetID string) (any, error)
	ListPackages   func(ctx context.Context, assetID string) (any, error)

	// Optional hub-internal dependencies. When nil, the relevant tool returns
	// errNotConfigured rather than panicking.
	ListDockerHosts        func(ctx context.Context) ([]map[string]any, error)
	ListDockerContainers   func(ctx context.Context, hostID string) ([]map[string]any, error)
	RestartDockerContainer func(ctx context.Context, containerID string) error
	ListAlerts             func(ctx context.Context) ([]map[string]any, error)
	AcknowledgeAlert       func(ctx context.Context, alertID string) error
	ListGroups             func(ctx context.Context) ([]map[string]any, error)
	MetricsOverview        func(ctx context.Context) (map[string]any, error)
	WakeAsset              func(ctx context.Context, assetID string) (map[string]any, error)
	DockerContainerLogs    func(ctx context.Context, assetID, containerID string, tail int) (string, error)
	DockerContainerStats   func(ctx context.Context, assetID, containerID string) (map[string]any, error)

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

	// MCP is intentionally a curated automation surface rather than a mirror of
	// every REST mutation. The broader operations below remain REST/console-only
	// until they have equally strict MCP schemas, authorization and safety gates:
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
	// Candidate future read-only resources, subject to the same authorization contract:
	// - labtether://assets/{id} (per-asset detail)
	// - labtether://metrics/overview (fleet health)

	return s
}
