package mcpserver

// tools_agent.go — agent-based tools: services, files, system info, and power.
// These tools use exec-as-transport to run commands on the target asset via its connected agent.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/terminal"
)

// execOnAsset is the shared transport used by agent-based tools.
// It validates scope, asset access, and agent connectivity, then runs cmd on the asset.
func (d *Deps) execOnAsset(ctx context.Context, scope, assetID, cmd string) (string, error) {
	if err := d.scopeCheck(ctx, scope); err != nil {
		return "", err
	}
	if err := d.assetCheck(ctx, assetID); err != nil {
		return "", err
	}
	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		asset, ok, _ := d.AssetStore.GetAsset(assetID)
		msg := assetID + " agent is not connected"
		if ok {
			msg = fmt.Sprintf("%s agent is not connected (last seen: %s)", assetID, asset.LastSeenAt.Format(time.RFC3339))
		}
		return "", fmt.Errorf("%s", msg)
	}
	result := d.ExecuteViaAgent(terminal.CommandJob{
		JobID:       idgen.New("mcp"),
		SessionID:   idgen.New("mcps"),
		CommandID:   idgen.New("mcpc"),
		ActorID:     d.GetActorID(ctx),
		Target:      assetID,
		Command:     cmd,
		Mode:        "structured",
		RequestedAt: time.Now().UTC(),
	})
	output := strings.TrimSpace(result.Output)
	if !strings.EqualFold(strings.TrimSpace(result.Status), "succeeded") {
		if output == "" {
			return "", fmt.Errorf("command failed on %s", assetID)
		}
		return "", fmt.Errorf("command failed on %s: %s", assetID, output)
	}
	return output, nil
}

// --- Services ---

func (d *Deps) handleServicesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	output, err := d.execOnAsset(ctx, "services:read", assetID,
		"systemctl list-units --type=service --state=running --no-pager --plain 2>/dev/null || service --status-all 2>&1 | head -50")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

func (d *Deps) handleServicesRestart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	serviceName, _ := req.RequireString("service_name")
	if strings.ContainsAny(serviceName, " \t\n\r;|&`$(){}") {
		return mcp.NewToolResultError("invalid service_name: contains disallowed characters"), nil
	}
	output, err := d.execOnAsset(ctx, "services:write", assetID,
		fmt.Sprintf("systemctl restart %s 2>&1 && echo 'restarted'", serviceName))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

// --- Files ---

func (d *Deps) handleFilesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	path, _ := req.RequireString("path")
	if strings.ContainsAny(path, ";\t`$(){}") {
		return mcp.NewToolResultError("invalid path: contains disallowed characters"), nil
	}
	output, err := d.execOnAsset(ctx, "files:read", assetID, fmt.Sprintf("ls -la %q", path))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

func (d *Deps) handleFilesRead(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	path, _ := req.RequireString("path")
	if strings.ContainsAny(path, ";\t`$(){}") {
		return mcp.NewToolResultError("invalid path: contains disallowed characters"), nil
	}
	output, err := d.execOnAsset(ctx, "files:read", assetID, fmt.Sprintf("cat %q", path))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

// --- System Info ---

func (d *Deps) handleSystemProcesses(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	output, err := d.execOnAsset(ctx, "processes:read", assetID,
		"ps aux --sort=-%cpu 2>/dev/null | head -50 || ps aux | head -50")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

func (d *Deps) handleSystemNetwork(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	output, err := d.execOnAsset(ctx, "network:read", assetID,
		"ip -j addr show 2>/dev/null || ifconfig 2>/dev/null || ipconfig 2>/dev/null")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

func (d *Deps) handleSystemDisks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	output, err := d.execOnAsset(ctx, "disks:read", assetID,
		"df -h --output=source,size,used,avail,pcent,target 2>/dev/null || df -h 2>/dev/null")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

func (d *Deps) handleSystemPackages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	output, err := d.execOnAsset(ctx, "packages:read", assetID,
		"dpkg -l 2>/dev/null || rpm -qa 2>/dev/null || pkg info 2>/dev/null || brew list 2>/dev/null")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

// --- Power ---

func (d *Deps) handleAssetReboot(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	output, err := d.execOnAsset(ctx, "assets:power", assetID,
		"shutdown -r now 2>&1 || reboot 2>&1 || echo 'reboot command issued'")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

func (d *Deps) handleAssetShutdown(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assetID, _ := req.RequireString("asset_id")
	output, err := d.execOnAsset(ctx, "assets:power", assetID,
		"shutdown -h now 2>&1 || poweroff 2>&1 || halt 2>&1 || echo 'shutdown command issued'")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

func (d *Deps) handleAssetWake(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "assets:power"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	assetID, _ := req.RequireString("asset_id")
	if err := d.assetCheck(ctx, assetID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	asset, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil || !ok {
		return mcp.NewToolResultError("asset not found: " + assetID), nil
	}
	// WoL is triggered from the hub side (or a neighbouring online asset).
	// Without a dedicated WoL dependency we return the asset's MAC so the
	// caller can act on it, and note that WoL requires a peer on the same LAN.
	mac := ""
	if asset.Metadata != nil {
		mac = asset.Metadata["mac_address"]
	}
	msg := fmt.Sprintf("Wake-on-LAN for asset %s (%s): MAC=%q — use `wakeonlan` or equivalent from a host on the same network.", assetID, asset.Name, mac)
	return mcp.NewToolResultText(msg), nil
}
