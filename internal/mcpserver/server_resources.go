package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/apikeys"
	"github.com/mark3labs/mcp-go/mcp"
	"strings"
)

// --- Tool Handlers ---

func (d *Deps) scopeCheck(ctx context.Context, scope string) error {
	if d == nil || d.GetScopes == nil {
		return errors.New("MCP authorization context is unavailable")
	}
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
	if d == nil || d.GetAllowedAssets == nil {
		return errors.New("MCP asset authorization context is unavailable")
	}
	if strings.TrimSpace(assetID) == "" || len(assetID) > maxMCPIdentifierBytes {
		return errors.New("invalid asset_id")
	}
	allowed := d.GetAllowedAssets(ctx)
	if !apikeys.AssetAllowed(allowed, assetID) {
		return fmt.Errorf("access denied to asset: %s", assetID)
	}
	return nil
}

func (d *Deps) unrestrictedGlobalRead(ctx context.Context, object string) error {
	if d == nil || d.GetAllowedAssets == nil {
		return errors.New("MCP asset authorization context is unavailable")
	}
	if len(d.GetAllowedAssets(ctx)) > 0 {
		return fmt.Errorf("asset-restricted API keys cannot access global %s", object)
	}
	return nil
}

func (d *Deps) handleWhoami(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if d == nil || d.GetScopes == nil || d.GetAllowedAssets == nil || d.AssetStore == nil {
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}
	scopes := d.GetScopes(ctx)
	allowed := d.GetAllowedAssets(ctx)

	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		return mcp.NewToolResultError("failed to list accessible assets"), nil
	}
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
	if err := validateCollectionSize("accessible asset inventory", len(accessibleAssets)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result := map[string]any{
		"scopes":           scopes,
		"allowed_assets":   allowed,
		"available_assets": accessibleAssets,
	}
	return toolJSON(result), nil
}

func (d *Deps) handleAssetsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "assets:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if d.AssetStore == nil || d.GetAllowedAssets == nil {
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}
	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		return mcp.NewToolResultError("failed to list assets"), nil
	}
	allowed := d.GetAllowedAssets(ctx)
	statusFilter := strings.TrimSpace(req.GetString("status", ""))
	platformFilter := strings.TrimSpace(req.GetString("platform", ""))
	if len(statusFilter) > 32 || len(platformFilter) > 32 {
		return mcp.NewToolResultError("status and platform filters are limited to 32 bytes"), nil
	}
	if statusFilter != "" && !strings.EqualFold(statusFilter, "online") && !strings.EqualFold(statusFilter, "offline") {
		return mcp.NewToolResultError("status must be online or offline"), nil
	}

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
	if err := validateCollectionSize("filtered asset inventory", len(filtered)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return toolJSON(filtered), nil
}

func (d *Deps) handleAssetsGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "assets:read"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	assetID, inputErr := requireAssetID(req)
	if inputErr != nil {
		return mcp.NewToolResultError(inputErr.Error()), nil
	}
	if err := d.assetCheck(ctx, assetID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if d.AssetStore == nil {
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
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
	return toolJSON(result), nil
}

// --- Resource Handlers ---

func (d *Deps) handleAssetsResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	if err := d.scopeCheck(ctx, "assets:read"); err != nil {
		return nil, err
	}
	if d.AssetStore == nil || d.GetAllowedAssets == nil {
		return nil, errMCPDependencyUnavailable
	}
	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		return nil, errors.New("failed to list assets")
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
	if err := validateCollectionSize("accessible asset inventory", len(accessible)); err != nil {
		return nil, err
	}

	data, err := marshalBoundedJSON(accessible, false)
	if err != nil {
		return nil, err
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

func (d *Deps) handleActiveAlertsResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	if err := d.scopeCheck(ctx, "alerts:read"); err != nil {
		return nil, err
	}
	if d.ListAlerts == nil {
		return nil, errMCPDependencyUnavailable
	}
	payload, err := d.ListAlerts(ctx)
	if err != nil {
		return nil, errors.New("failed to list alerts")
	}
	if payload == nil {
		payload = []map[string]any{}
	}
	if err := validateCollectionSize("alert list", len(payload)); err != nil {
		return nil, err
	}
	data, err := marshalBoundedJSON(payload, false)
	if err != nil {
		return nil, err
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

func (d *Deps) handleGroupsResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	if err := d.scopeCheck(ctx, "groups:read"); err != nil {
		return nil, err
	}
	if d.ListGroups == nil {
		return nil, errMCPDependencyUnavailable
	}
	payload, err := d.ListGroups(ctx)
	if err != nil {
		return nil, errors.New("failed to list groups")
	}
	if payload == nil {
		payload = []map[string]any{}
	}
	if err := validateCollectionSize("group list", len(payload)); err != nil {
		return nil, err
	}
	data, err := marshalBoundedJSON(payload, false)
	if err != nil {
		return nil, err
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
