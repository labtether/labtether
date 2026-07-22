package mcpserver

// tools_connectors.go — connector health and typed Docker coordinator tools.
// Docker operations deliberately use injected coordinator closures rather than
// raw endpoint commands.

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Connector health ---

func (d *Deps) handleConnectorsHealth(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "connectors:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := d.unrestrictedGlobalRead(ctx, "connector health checks"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ConnectorsHealth == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	statuses, err := d.ConnectorsHealth(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to get connector health"), nil
	}
	if err := validateCollectionSize("connector health", len(statuses)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(statuses), nil
}

// --- Docker container logs ---

func (d *Deps) handleDockerContainerLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "docker:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := d.assetCheck(ctx, assetID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	containerID, err := requireBoundedString(req, "container_id", maxMCPIdentifierBytes)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := validateSafeShellAtom("container_id", containerID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	tail := req.GetInt("tail", 100)
	if tail < 1 {
		tail = 1
	}
	if tail > 10000 {
		tail = 10000
	}
	if d.DockerContainerLogs == nil {
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}
	output, err := d.DockerContainerLogs(ctx, assetID, containerID, tail)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return boundedToolText(output), nil
}

// --- Docker container stats ---

func (d *Deps) handleDockerContainerStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "docker:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := d.assetCheck(ctx, assetID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	containerID, err := requireBoundedString(req, "container_id", maxMCPIdentifierBytes)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := validateSafeShellAtom("container_id", containerID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.DockerContainerStats == nil {
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}
	output, err := d.DockerContainerStats(ctx, assetID, containerID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(output), nil
}
