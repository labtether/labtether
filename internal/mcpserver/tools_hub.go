package mcpserver

// tools_hub.go — tools backed by hub-internal dependencies: Docker, alerts, groups, metrics.
// These tools require optional Deps fields; if nil they return a "not configured" error.

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/labtether/labtether/internal/apikeys"
)

const errNotConfigured = "This tool requires hub dependencies that are not configured."

// --- Docker ---

func (d *Deps) handleDockerHosts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "docker:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListDockerHosts == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	if d.GetAllowedAssets == nil {
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}
	hosts, err := d.ListDockerHosts(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to list docker hosts"), nil
	}
	hosts = filterDockerHostsForContext(ctx, d, hosts)
	if err := validateCollectionSize("docker host list", len(hosts)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(hosts), nil
}

func filterDockerHostsForContext(ctx context.Context, d *Deps, hosts []map[string]any) []map[string]any {
	if d == nil || d.GetAllowedAssets == nil {
		return nil
	}
	allowed := d.GetAllowedAssets(ctx)
	if allowed == nil {
		return hosts
	}
	filtered := make([]map[string]any, 0, len(hosts))
	for _, host := range hosts {
		agentID, _ := host["agent_id"].(string)
		if apikeys.AssetAllowed(allowed, agentID) {
			filtered = append(filtered, host)
		}
	}
	return filtered
}

func (d *Deps) handleDockerContainers(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "docker:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListDockerContainers == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	hostID, inputErr := requireBoundedString(req, "host_id", maxMCPIdentifierBytes)
	if inputErr != nil {
		return mcp.NewToolResultError(inputErr.Error()), nil
	}
	containers, err := d.ListDockerContainers(ctx, hostID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list containers for host %s", hostID)), nil
	}
	if err := validateCollectionSize("docker container list", len(containers)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(containers), nil
}

func (d *Deps) handleDockerContainerRestart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "docker:write"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	containerID, inputErr := requireBoundedString(req, "container_id", maxMCPIdentifierBytes)
	if inputErr != nil {
		return mcp.NewToolResultError(inputErr.Error()), nil
	}
	if err := validateSafeShellAtom("container_id", containerID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.RestartDockerContainer == nil {
		d.auditMutation(ctx, "docker_container_restart", containerID, "failed", "dependency_unavailable", nil)
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	if err := d.checkMutation(ctx, "docker_container_restart", containerID); err != nil {
		d.auditMutation(ctx, "docker_container_restart", containerID, "denied", errorReason(err), nil)
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := d.RestartDockerContainer(ctx, containerID); err != nil {
		d.auditMutation(ctx, "docker_container_restart", containerID, "failed", errorReason(err), nil)
		return mcp.NewToolResultError(fmt.Sprintf("failed to restart container %s", containerID)), nil
	}
	d.auditMutation(ctx, "docker_container_restart", containerID, "succeeded", "", nil)
	return toolJSON(map[string]any{"container_id": containerID, "restarted": true}), nil
}

// --- Alerts ---

func (d *Deps) handleAlertsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "alerts:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListAlerts == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	alerts, err := d.ListAlerts(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to list alerts"), nil
	}
	if err := validateCollectionSize("alert list", len(alerts)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(alerts), nil
}

func (d *Deps) handleAlertsAcknowledge(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "alerts:write"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	alertID, inputErr := requireBoundedString(req, "alert_id", maxMCPIdentifierBytes)
	if inputErr != nil {
		return mcp.NewToolResultError(inputErr.Error()), nil
	}
	if d.AcknowledgeAlert == nil {
		d.auditMutation(ctx, "alerts_acknowledge", alertID, "failed", "dependency_unavailable", nil)
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	if err := d.checkMutation(ctx, "alerts_acknowledge", alertID); err != nil {
		d.auditMutation(ctx, "alerts_acknowledge", alertID, "denied", errorReason(err), nil)
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := d.AcknowledgeAlert(ctx, alertID); err != nil {
		d.auditMutation(ctx, "alerts_acknowledge", alertID, "failed", errorReason(err), nil)
		return mcp.NewToolResultError(fmt.Sprintf("failed to acknowledge alert %s", alertID)), nil
	}
	d.auditMutation(ctx, "alerts_acknowledge", alertID, "succeeded", "", nil)
	return toolJSON(map[string]any{"alert_id": alertID, "acknowledged": true}), nil
}

// --- Groups ---

func (d *Deps) handleGroupsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "groups:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListGroups == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	groups, err := d.ListGroups(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to list groups"), nil
	}
	if err := validateCollectionSize("group list", len(groups)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(groups), nil
}

// --- Metrics ---

func (d *Deps) handleMetricsOverview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "metrics:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.MetricsOverview == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	overview, err := d.MetricsOverview(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to get metrics overview"), nil
	}
	return toolJSON(overview), nil
}
