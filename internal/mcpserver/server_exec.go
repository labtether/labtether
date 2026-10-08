package mcpserver

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/mark3labs/mcp-go/mcp"
	"strings"
	"sync"
	"time"
)

const (
	defaultExecTimeoutSeconds = 30
	maxExecTimeoutSeconds     = 300
	maxExecMultiTargets       = 64
	maxExecMultiConcurrency   = 8
)

func normalizeExecTimeoutSeconds(timeout int) int {
	if timeout <= 0 {
		return defaultExecTimeoutSeconds
	}
	if timeout > maxExecTimeoutSeconds {
		return maxExecTimeoutSeconds
	}
	return timeout
}

func (d *Deps) handleExec(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "assets:exec"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	assetID, err := requireAssetID(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	command, err := requireCommand(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	timeout := normalizeExecTimeoutSeconds(req.GetInt("timeout", defaultExecTimeoutSeconds))

	if err := d.assetCheck(ctx, assetID); err != nil {
		d.auditMutation(ctx, "exec", assetID, "denied", errorReason(err), map[string]any{"command_bytes": len([]byte(command))})
		return mcp.NewToolResultError(err.Error()), nil
	}

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		var asset assets.Asset
		var ok bool
		if d.AssetStore != nil {
			asset, ok, _ = d.AssetStore.GetAsset(assetID)
		}
		msg := assetID + " agent is not connected"
		if ok {
			msg = fmt.Sprintf("%s agent is not connected (last seen: %s)", assetID, asset.LastSeenAt.Format(time.RFC3339))
		}
		d.auditMutation(ctx, "exec", assetID, "failed", "asset_offline", map[string]any{"command_bytes": len([]byte(command))})
		return mcp.NewToolResultError(msg), nil
	}
	if d.ExecuteViaAgent == nil || d.GetActorID == nil {
		d.auditMutation(ctx, "exec", assetID, "failed", "dependency_unavailable", map[string]any{"command_bytes": len([]byte(command))})
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}
	if err := d.checkMutation(ctx, "exec", assetID); err != nil {
		d.auditMutation(ctx, "exec", assetID, "denied", errorReason(err), map[string]any{"command_bytes": len([]byte(command))})
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := d.checkCommandPolicy(ctx, assetID, command); err != nil {
		d.auditMutation(ctx, "exec", assetID, "denied", errorReason(err), map[string]any{"command_bytes": len([]byte(command))})
		return mcp.NewToolResultError(err.Error()), nil
	}

	cmdResult := d.ExecuteViaAgent(terminal.CommandJob{
		JobID:       idgen.New("mcp"),
		SessionID:   idgen.New("mcps"),
		CommandID:   idgen.New("mcpc"),
		ActorID:     d.GetActorID(ctx),
		Target:      assetID,
		Command:     command,
		Mode:        "structured",
		TimeoutSec:  timeout,
		RequestedAt: time.Now().UTC(),
	})

	succeeded := strings.EqualFold(strings.TrimSpace(cmdResult.Status), "succeeded")
	exitCode := 0
	if !succeeded {
		exitCode = 1
	}
	// Reserve enough room for worst-case JSON escaping plus response metadata.
	output, truncated := truncateUTF8(strings.TrimSpace(cmdResult.Output), maxMCPJSONBytes/8)

	result := map[string]any{
		"asset_id":  assetID,
		"exit_code": exitCode,
		"output":    output,
	}
	if truncated {
		result["output_truncated"] = true
	}
	decision := "succeeded"
	if !succeeded {
		decision = "failed"
	}
	reason := ""
	if !succeeded {
		reason = "command_failed"
	}
	d.auditMutation(ctx, "exec", assetID, decision, reason, map[string]any{
		"command_bytes": len([]byte(command)),
		"exit_code":     exitCode,
		"truncated":     truncated,
	})
	data, encodeErr := marshalBoundedJSON(result, true)
	if encodeErr != nil {
		return mcp.NewToolResultError(encodeErr.Error()), nil
	}
	if !succeeded {
		return mcp.NewToolResultError(string(data)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (d *Deps) handleExecMulti(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := d.scopeCheck(ctx, "assets:exec"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	command, err := requireCommand(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	timeout := normalizeExecTimeoutSeconds(req.GetInt("timeout", defaultExecTimeoutSeconds))

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
	if len(targetsSlice) > maxExecMultiTargets {
		return mcp.NewToolResultError(fmt.Sprintf("targets must contain at most %d entries", maxExecMultiTargets)), nil
	}

	if d.GetAllowedAssets == nil {
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}
	allowed := d.GetAllowedAssets(ctx)
	targets := make([]string, 0, len(targetsSlice))
	seen := make(map[string]struct{}, len(targetsSlice))
	var invalidCount int
	for _, t := range targetsSlice {
		s, ok := t.(string)
		if !ok {
			invalidCount++
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" || len(s) > maxMCPIdentifierBytes || strings.ContainsAny(s, "/\\\r\n") || strings.IndexByte(s, 0) >= 0 {
			invalidCount++
			continue
		}
		if _, duplicate := seen[s]; duplicate {
			continue
		}
		seen[s] = struct{}{}
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
	if d.AgentMgr == nil || d.ExecuteViaAgent == nil || d.GetActorID == nil {
		for _, target := range targets {
			d.auditMutation(ctx, "exec_multi", target, "failed", "dependency_unavailable", map[string]any{"command_bytes": len([]byte(command))})
		}
		return mcp.NewToolResultError(errMCPDependencyUnavailable.Error()), nil
	}

	type execMultiResult struct {
		target string
		value  any
	}
	resultSlots := make([]execMultiResult, len(targets))
	for index, target := range targets {
		resultSlots[index].target = target
	}
	perTargetOutputLimit := maxMCPJSONBytes / max(8, len(targets)*8)
	jobs := make(chan int)
	var wg sync.WaitGroup
	workerCount := min(len(targets), maxExecMultiConcurrency)
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				t := targets[index]
				if d.AgentMgr == nil || !d.AgentMgr.IsConnected(t) {
					resultSlots[index].value = map[string]any{"error": "asset_offline"}
					d.auditMutation(ctx, "exec_multi", t, "failed", "asset_offline", map[string]any{"command_bytes": len([]byte(command))})
					continue
				}
				if err := ctx.Err(); err != nil {
					resultSlots[index].value = map[string]any{"error": "request_canceled"}
					continue
				}
				// Authorize immediately before dispatch so maintenance changes and
				// admission limits cannot be bypassed by time spent in the queue.
				if err := d.checkMutation(ctx, "exec_multi", t); err != nil {
					resultSlots[index].value = map[string]any{"error": errorReason(err)}
					d.auditMutation(ctx, "exec_multi", t, "denied", errorReason(err), map[string]any{"command_bytes": len([]byte(command))})
					continue
				}
				if err := d.checkCommandPolicy(ctx, t, command); err != nil {
					reason := errorReason(err)
					resultSlots[index].value = map[string]any{"error": reason}
					d.auditMutation(ctx, "exec_multi", t, "denied", reason, map[string]any{"command_bytes": len([]byte(command))})
					continue
				}
				cmdResult := d.ExecuteViaAgent(terminal.CommandJob{
					JobID:       idgen.New("mcp"),
					SessionID:   idgen.New("mcps"),
					CommandID:   idgen.New("mcpc"),
					ActorID:     d.GetActorID(ctx),
					Target:      t,
					Command:     command,
					Mode:        "structured",
					TimeoutSec:  timeout,
					RequestedAt: time.Now().UTC(),
				})
				exitCode := 0
				if !strings.EqualFold(strings.TrimSpace(cmdResult.Status), "succeeded") {
					exitCode = 1
				}
				output, truncated := truncateUTF8(strings.TrimSpace(cmdResult.Output), perTargetOutputLimit)
				value := map[string]any{
					"exit_code": exitCode,
					"output":    output,
				}
				if truncated {
					value["output_truncated"] = true
				}
				resultSlots[index].value = value
				decision := "succeeded"
				if exitCode != 0 {
					decision = "failed"
				}
				reason := ""
				if exitCode != 0 {
					reason = "command_failed"
				}
				d.auditMutation(ctx, "exec_multi", t, decision, reason, map[string]any{
					"command_bytes": len([]byte(command)),
					"exit_code":     exitCode,
					"truncated":     truncated,
				})
			}
		}()
	}
	for index := range targets {
		select {
		case jobs <- index:
		case <-ctx.Done():
			resultSlots[index].value = map[string]any{"error": "request_canceled"}
		}
	}
	close(jobs)
	wg.Wait()

	results := make(map[string]any, len(resultSlots))
	succeededCount := 0
	for _, result := range resultSlots {
		results[result.target] = result.value
		if value, ok := result.value.(map[string]any); ok {
			if exitCode, ok := value["exit_code"].(int); ok && exitCode == 0 {
				succeededCount++
			}
		}
	}
	data, err := marshalBoundedJSON(results, true)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if succeededCount == 0 {
		return mcp.NewToolResultError(string(data)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}
