package mcpserver

// tools_hub.go — tools backed by hub-internal dependencies: Docker, alerts, groups, metrics.
// These tools require optional Deps fields; if nil they return a "not configured" error.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
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
	hosts, err := d.ListDockerHosts()
	if err != nil {
		return mcp.NewToolResultError("failed to list docker hosts: " + err.Error()), nil
	}
	data, _ := json.MarshalIndent(hosts, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (d *Deps) handleDockerContainers(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "docker:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListDockerContainers == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	hostID, _ := req.RequireString("host_id")
	containers, err := d.ListDockerContainers(hostID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list containers for host %s: %s", hostID, err.Error())), nil
	}
	data, _ := json.MarshalIndent(containers, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (d *Deps) handleDockerContainerRestart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "docker:write"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.RestartDockerContainer == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	containerID, _ := req.RequireString("container_id")
	if err := d.RestartDockerContainer(containerID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to restart container %s: %s", containerID, err.Error())), nil
	}
	data, _ := json.Marshal(map[string]any{"container_id": containerID, "restarted": true})
	return mcp.NewToolResultText(string(data)), nil
}

// --- Alerts ---

func (d *Deps) handleAlertsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "alerts:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListAlerts == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	alerts, err := d.ListAlerts()
	if err != nil {
		return mcp.NewToolResultError("failed to list alerts: " + err.Error()), nil
	}
	data, _ := json.MarshalIndent(alerts, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (d *Deps) handleAlertsAcknowledge(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "alerts:write"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.AcknowledgeAlert == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	alertID, _ := req.RequireString("alert_id")
	if err := d.AcknowledgeAlert(alertID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to acknowledge alert %s: %s", alertID, err.Error())), nil
	}
	data, _ := json.Marshal(map[string]any{"alert_id": alertID, "acknowledged": true})
	return mcp.NewToolResultText(string(data)), nil
}

// --- Groups ---

func (d *Deps) handleGroupsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "groups:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListGroups == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	groups, err := d.ListGroups()
	if err != nil {
		return mcp.NewToolResultError("failed to list groups: " + err.Error()), nil
	}
	data, _ := json.MarshalIndent(groups, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- Metrics ---

func (d *Deps) handleMetricsOverview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "metrics:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.MetricsOverview == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	overview, err := d.MetricsOverview()
	if err != nil {
		return mcp.NewToolResultError("failed to get metrics overview: " + err.Error()), nil
	}
	data, _ := json.MarshalIndent(overview, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
