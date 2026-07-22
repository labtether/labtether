package mcpserver

// tools_agent.go — typed agent tools: services, files, system info, and power.

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func (d *Deps) typedAgentRead(
	ctx context.Context,
	scope string,
	assetID string,
	call func(context.Context, string) (any, error),
) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, scope); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := d.assetCheck(ctx, assetID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		return mcp.NewToolResultError("agent is not connected"), nil
	}
	if call == nil {
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}
	value, err := call(ctx, assetID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolValue(value), nil
}

// --- Services ---

func (d *Deps) handleServicesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return d.typedAgentRead(ctx, "services:read", assetID, d.ListServices)
}

func (d *Deps) handleServicesRestart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "services:write"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	serviceName, err := requireBoundedString(req, "service_name", maxMCPIdentifierBytes)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := validateSafeShellAtom("service_name", serviceName); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := d.assetCheck(ctx, assetID); err != nil {
		d.auditMutation(ctx, "services_restart", assetID, "denied", errorReason(err), map[string]any{"service": serviceName})
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		d.auditMutation(ctx, "services_restart", assetID, "failed", "asset_offline", map[string]any{"service": serviceName})
		return mcp.NewToolResultError("agent is not connected"), nil
	}
	if d.RestartService == nil {
		d.auditMutation(ctx, "services_restart", assetID, "failed", "dependency_unavailable", map[string]any{"service": serviceName})
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}
	if err := d.checkMutation(ctx, "services_restart", assetID); err != nil {
		d.auditMutation(ctx, "services_restart", assetID, "denied", errorReason(err), map[string]any{"service": serviceName})
		return mcp.NewToolResultError(err.Error()), nil
	}
	value, err := d.RestartService(ctx, assetID, serviceName)
	if err != nil {
		d.auditMutation(ctx, "services_restart", assetID, "failed", errorReason(err), map[string]any{"service": serviceName})
		return mcp.NewToolResultError(err.Error()), nil
	}
	d.auditMutation(ctx, "services_restart", assetID, "succeeded", "", map[string]any{"service": serviceName})
	return toolValue(value), nil
}

// --- Files ---

func (d *Deps) handleFilesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path, err := requirePath(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return d.typedAgentRead(ctx, "files:read", assetID, func(ctx context.Context, assetID string) (any, error) {
		if d.ListFiles == nil {
			return nil, errMCPDependencyUnavailable
		}
		return d.ListFiles(ctx, assetID, path)
	})
}

func (d *Deps) handleFilesRead(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path, err := requirePath(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return d.typedAgentRead(ctx, "files:read", assetID, func(ctx context.Context, assetID string) (any, error) {
		if d.ReadFile == nil {
			return nil, errMCPDependencyUnavailable
		}
		return d.ReadFile(ctx, assetID, path)
	})
}

// --- System Info ---

func (d *Deps) handleSystemProcesses(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return d.typedAgentRead(ctx, "processes:read", assetID, d.ListProcesses)
}

func (d *Deps) handleSystemNetwork(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return d.typedAgentRead(ctx, "network:read", assetID, d.ListNetwork)
}

func (d *Deps) handleSystemDisks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return d.typedAgentRead(ctx, "disks:read", assetID, d.ListDisks)
}

func (d *Deps) handleSystemPackages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return d.typedAgentRead(ctx, "packages:read", assetID, d.ListPackages)
}

// --- Power ---

func (d *Deps) handleAssetReboot(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	output, err := d.executeTypedPowerAction(ctx, assetID, "reboot")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

func (d *Deps) handleAssetShutdown(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	output, err := d.executeTypedPowerAction(ctx, assetID, "shutdown")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

func (d *Deps) executeTypedPowerAction(ctx context.Context, assetID, action string) (string, error) {
	if err := d.scopeCheck(ctx, "assets:power"); err != nil {
		return "", err
	}
	if err := d.assetCheck(ctx, assetID); err != nil {
		d.auditMutation(ctx, "asset_"+action, assetID, "denied", errorReason(err), map[string]any{"action": action})
		return "", err
	}
	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		d.auditMutation(ctx, "asset_"+action, assetID, "failed", "asset_offline", map[string]any{"action": action})
		return "", fmt.Errorf("agent is not connected")
	}
	if d.ExecutePowerAction == nil {
		d.auditMutation(ctx, "asset_"+action, assetID, "failed", "dependency_unavailable", map[string]any{"action": action})
		return "", fmt.Errorf("typed power actions are not configured")
	}
	tool := "asset_" + action
	if err := d.checkMutation(ctx, tool, assetID); err != nil {
		d.auditMutation(ctx, tool, assetID, "denied", errorReason(err), map[string]any{"action": action})
		return "", err
	}
	result, err := d.ExecutePowerAction(ctx, assetID, action)
	if err != nil {
		d.auditMutation(ctx, tool, assetID, "failed", errorReason(err), map[string]any{"action": action})
		return "", err
	}
	d.auditMutation(ctx, tool, assetID, "succeeded", "", map[string]any{"action": action})
	return result, nil
}

func (d *Deps) handleAssetWake(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "assets:power"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	assetID, inputErr := requireAssetID(req)
	if inputErr != nil {
		return mcp.NewToolResultError(inputErr.Error()), nil
	}
	if err := d.assetCheck(ctx, assetID); err != nil {
		d.auditMutation(ctx, "asset_wake", assetID, "denied", errorReason(err), map[string]any{"action": "wake"})
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.WakeAsset == nil {
		d.auditMutation(ctx, "asset_wake", assetID, "failed", "dependency_unavailable", map[string]any{"action": "wake"})
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}
	if err := d.checkMutation(ctx, "asset_wake", assetID); err != nil {
		d.auditMutation(ctx, "asset_wake", assetID, "denied", errorReason(err), map[string]any{"action": "wake"})
		return mcp.NewToolResultError(err.Error()), nil
	}
	result, err := d.WakeAsset(ctx, assetID)
	if err != nil {
		d.auditMutation(ctx, "asset_wake", assetID, "failed", errorReason(err), map[string]any{"action": "wake"})
		return mcp.NewToolResultError(err.Error()), nil
	}
	d.auditMutation(ctx, "asset_wake", assetID, "succeeded", "", map[string]any{"action": "wake"})
	return toolJSON(result), nil
}
