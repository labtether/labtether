package operations

import (
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

const (
	// DefaultUpdateAgentTimeout is the default timeout for agent-executed update jobs.
	DefaultUpdateAgentTimeout = 10 * time.Minute
	// DefaultUpdatePreviewTimeout bounds the read-only inventory request used by
	// dry runs independently from the much longer live package-update timeout.
	DefaultUpdatePreviewTimeout = 30 * time.Second
)

// UpdateExecutorDeps holds the narrow set of dependencies required for
// executing update scopes against connected agents. It is separate from
// ExecDeps so that callers needing only update execution do not depend on
// the full exec surface.
type UpdateExecutorDeps struct {
	// AgentMgr checks agent connectivity and is used to gate execution.
	AgentMgr *agentmgr.AgentManager

	// AssetStore resolves asset platform metadata.
	AssetStore persistence.AssetStore

	// ExecuteUpdateViaAgent dispatches an update command to a connected agent.
	// The signature matches agents.Deps.ExecuteUpdateViaAgent.
	ExecuteUpdateViaAgent func(jobID, target, mode string, packages []string, timeout time.Duration, force bool) agentmgr.CommandResultData

	// PreviewOSPackageUpdatesViaAgent requests a read-only, validated package
	// update inventory from the connected agent. It must never dispatch an
	// update.request or apply changes.
	PreviewOSPackageUpdatesViaAgent func(requestID, target string, timeout time.Duration) agentmgr.CommandResultData
}

// ExecuteUpdateScope dispatches a single update scope for a target asset.
func (d *UpdateExecutorDeps) ExecuteUpdateScope(job updates.Job, target, scope string) updates.RunResultEntry {
	entry := updates.RunResultEntry{
		Target: target,
		Scope:  scope,
		Status: updates.StatusFailed,
	}

	normalizedScope := strings.ToLower(strings.TrimSpace(scope))
	switch normalizedScope {
	case updates.ScopeOSPackages:
		return d.executeOSPackageUpdateScope(job, target, scope)
	default:
		entry.Summary = fmt.Sprintf("update scope %q is not supported; no changes applied to %s", normalizedScope, target)
		return entry
	}
}

func (d *UpdateExecutorDeps) executeOSPackageUpdateScope(job updates.Job, target, scope string) updates.RunResultEntry {
	entry := updates.RunResultEntry{
		Target: target,
		Scope:  scope,
		Status: updates.StatusFailed,
	}

	if strings.TrimSpace(target) == "" {
		entry.Summary = "empty update target"
		return entry
	}

	if d.AssetStore != nil {
		asset, found, err := d.AssetStore.GetAsset(target)
		if err != nil {
			entry.Summary = fmt.Sprintf("failed to resolve target %s: %v", target, err)
			return entry
		}
		if found {
			platform := strings.TrimSpace(strings.ToLower(asset.Platform))
			if platform == "" {
				platform = strings.TrimSpace(strings.ToLower(asset.Metadata["platform"]))
			}
			if platform != "" && platform != "linux" {
				entry.Summary = fmt.Sprintf("os_packages updates require linux; target platform is %s", platform)
				return entry
			}
		}
	}

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(target) {
		entry.Summary = "agent not connected"
		return entry
	}

	if job.DryRun {
		if d.PreviewOSPackageUpdatesViaAgent == nil {
			entry.Summary = "package update preview unavailable; no changes applied"
			return entry
		}
		requestID := idgen.New("updpreview")
		result := d.PreviewOSPackageUpdatesViaAgent(requestID, target, DefaultUpdatePreviewTimeout)
		entry.Summary = SummarizeUpdateOutput(result.Output)
		if updates.NormalizeStatus(result.Status) != updates.StatusSucceeded {
			entry.Status = updates.StatusFailed
			if entry.Summary == "" {
				entry.Summary = fmt.Sprintf("package update preview failed on %s; no changes applied", target)
			}
			return entry
		}
		if entry.Summary == "" {
			entry.Status = updates.StatusFailed
			entry.Summary = fmt.Sprintf("agent returned an empty package update preview for %s; no changes applied", target)
			return entry
		}
		entry.Status = updates.StatusSucceeded
		return entry
	}

	if d.ExecuteUpdateViaAgent == nil {
		entry.Summary = "package update executor unavailable; no changes applied"
		return entry
	}
	requestID := idgen.New("updreq")
	result := d.ExecuteUpdateViaAgent(requestID, target, updates.ScopeOSPackages, nil, DefaultUpdateAgentTimeout, false)
	status := updates.NormalizeStatus(result.Status)
	if status == "" {
		status = updates.StatusFailed
	}
	entry.Status = status

	summary := SummarizeUpdateOutput(result.Output)
	if summary == "" {
		if status == updates.StatusSucceeded {
			summary = fmt.Sprintf("applied %s on %s", scope, target)
		} else {
			summary = fmt.Sprintf("failed to apply %s on %s", scope, target)
		}
	}
	entry.Summary = summary
	return entry
}

// SummarizeUpdateOutput extracts a brief summary from raw update command output.
// Exported so agent and worker callers can reuse it without duplicating logic.
func SummarizeUpdateOutput(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	firstLine := strings.TrimSpace(strings.Split(trimmed, "\n")[0])
	if firstLine == "" {
		return ""
	}
	const maxSummaryLen = 220
	if len(firstLine) > maxSummaryLen {
		return firstLine[:maxSummaryLen] + "..."
	}
	return firstLine
}
