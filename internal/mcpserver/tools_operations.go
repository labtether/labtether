package mcpserver

// tools_operations.go — tools backed by hub-internal operational stores:
// schedules, webhooks, saved actions, credentials, topology edges, and update plans.
// All closures are optional; if nil, the tool returns errNotConfigured.

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Schedules ---

func (d *Deps) handleSchedulesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "schedules:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListSchedules == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	items, err := d.ListSchedules(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to list schedules"), nil
	}
	if err := validateCollectionSize("schedule list", len(items)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(items), nil
}

// --- Webhooks ---

func (d *Deps) handleWebhooksList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "webhooks:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := d.unrestrictedGlobalRead(ctx, "webhooks"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListWebhooks == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	items, err := d.ListWebhooks(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to list webhooks"), nil
	}
	if err := validateCollectionSize("webhook list", len(items)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(items), nil
}

// --- Saved Actions ---

func (d *Deps) handleSavedActionsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "actions:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListSavedActions == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	items, err := d.ListSavedActions(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to list saved actions"), nil
	}
	if err := validateCollectionSize("saved action list", len(items)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(items), nil
}

// --- Credentials ---

func (d *Deps) handleCredentialsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "credentials:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := d.unrestrictedGlobalRead(ctx, "credential profiles"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListCredentialProfiles == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	// Secrets are stripped by the closure before being returned here.
	items, err := d.ListCredentialProfiles(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to list credential profiles"), nil
	}
	if err := validateCollectionSize("credential profile list", len(items)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(items), nil
}

// --- Topology edges ---

func (d *Deps) handleTopologyEdges(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "topology:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.GetEdgesForAsset == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	assetID, inputErr := requireAssetID(req)
	if inputErr != nil {
		return mcp.NewToolResultError(inputErr.Error()), nil
	}
	if err := d.assetCheck(ctx, assetID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	edges, err := d.GetEdgesForAsset(ctx, assetID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get edges for asset %s", assetID)), nil
	}
	if err := validateCollectionSize("topology edge list", len(edges)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(edges), nil
}

// --- Update plans ---

func (d *Deps) handleUpdatesListPlans(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "updates:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if d.ListUpdatePlans == nil {
		return mcp.NewToolResultError(errNotConfigured), nil
	}
	items, err := d.ListUpdatePlans(ctx)
	if err != nil {
		return mcp.NewToolResultError("failed to list update plans"), nil
	}
	if err := validateCollectionSize("update plan list", len(items)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolJSON(items), nil
}
