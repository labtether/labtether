package mcpserver

// tools_connectors.go — tools for connector health and Docker exec-based operations.
// ConnectorsHealth uses a hub-internal closure; docker_container_logs and
// docker_container_stats use the execOnAsset (agent exec) pattern.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Connector health ---

func (d *Deps) handleConnectorsHealth(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "connectors:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ConnectorsHealth == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	statuses, err := d.ConnectorsHealth(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to get connector health: " + err.Error()), nil
	}
	data, _ := json.MarshalIndent(statuses, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- Docker container logs (agent exec) ---

func (d *Deps) handleDockerContainerLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	containerID, _ := req.RequireString("container_id")
	if strings.ContainsAny(containerID, " \t\n\r;|&`$(){}") {
		return mcp.NewToolResultError("invalid container_id: contains disallowed characters"), nil
	}
	tail := req.GetInt("tail", 100)
	if tail < 1 {
		tail = 1
	}
	if tail > 10000 {
		tail = 10000
	}
	output, err := d.execOnAsset(ctx, "docker:read", assetID,
		fmt.Sprintf("docker logs --tail %d %s 2>&1", tail, containerID))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

// --- Docker container stats (agent exec) ---

func (d *Deps) handleDockerContainerStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	containerID, _ := req.RequireString("container_id")
	if strings.ContainsAny(containerID, " \t\n\r;|&`$(){}") {
		return mcp.NewToolResultError("invalid container_id: contains disallowed characters"), nil
	}
	output, err := d.execOnAsset(ctx, "docker:read", assetID,
		fmt.Sprintf("docker stats --no-stream %s 2>&1", containerID))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}
