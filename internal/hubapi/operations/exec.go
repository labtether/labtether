package operations

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

const (
	DefaultExecTimeout = 30
	MaxExecTimeout     = 300
)

// ExecRequest is the JSON body for a single-asset exec request.
type ExecRequest struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// ExecMultiRequest is the JSON body for a multi-asset exec request.
type ExecMultiRequest struct {
	Targets []string `json:"targets,omitempty"`
	Group   string   `json:"group,omitempty"`
	Command string   `json:"command"`
	Timeout int      `json:"timeout,omitempty"`
}

// ExecResult is the per-asset execution result returned to callers.
type ExecResult struct {
	AssetID    string `json:"asset_id"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	Message    string `json:"message,omitempty"`
}

// HandleAssetExec handles POST /api/v2/assets/{id}/exec.
func (d *ExecDeps) HandleAssetExec(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(d.ScopesFromContext(r.Context()), "assets:exec") {
		apiv2.WriteScopeForbidden(w, "assets:exec")
		return
	}

	var req ExecRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.Command = strings.TrimSpace(req.Command)
	if req.Command == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "command is required")
		return
	}
	if req.Timeout <= 0 {
		req.Timeout = DefaultExecTimeout
	}
	if req.Timeout > MaxExecTimeout {
		req.Timeout = MaxExecTimeout
	}

	result := d.ExecOnAsset(r, assetID, req.Command, req.Timeout)
	if result.Error != "" {
		status := http.StatusInternalServerError
		errorCode := "exec_failed"
		if result.Error == "asset_offline" {
			status = http.StatusConflict
			errorCode = "asset_offline"
		}
		apiv2.WriteError(w, status, errorCode, result.Message)
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, result)
	// TODO: Comprehensive per-request audit logging for all API v2 endpoints is planned.
	d.AppendAuditEventBestEffort(audit.Event{
		Type:      "api.exec",
		ActorID:   d.PrincipalActorID(r.Context()),
		Target:    assetID,
		Details:   map[string]any{"command": req.Command, "exit_code": result.ExitCode},
		Timestamp: time.Now().UTC(),
	}, "v2 exec on "+assetID)
}

// ExecOnAsset runs a command on a single asset via the agent manager.
func (d *ExecDeps) ExecOnAsset(r *http.Request, assetID, command string, timeoutSec int) ExecResult {
	// NOTE: timeoutSec is validated by callers but not enforced at the exec layer.
	// The agent execution timeout is managed by the hub's agent manager internally.
	// This parameter is reserved for future per-command deadline support.
	_ = timeoutSec

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		// Check if asset exists to give a better error message.
		asset, ok, _ := d.AssetStore.GetAsset(assetID)
		msg := assetID + " agent is not connected"
		if ok {
			msg = fmt.Sprintf("%s agent is not connected (last seen: %s)",
				assetID, asset.LastSeenAt.Format(time.RFC3339))
		}
		return ExecResult{AssetID: assetID, Error: "asset_offline", Message: msg}
	}

	start := time.Now()
	cmdResult := d.ExecuteViaAgent(terminal.CommandJob{
		JobID:       idgen.New("v2exec"),
		SessionID:   idgen.New("v2sess"),
		CommandID:   idgen.New("v2cmd"),
		ActorID:     d.PrincipalActorID(r.Context()),
		Target:      assetID,
		Command:     command,
		Mode:        "structured",
		RequestedAt: time.Now().UTC(),
	})
	elapsed := time.Since(start)

	exitCode := 0
	if !strings.EqualFold(strings.TrimSpace(cmdResult.Status), "succeeded") {
		exitCode = 1
	}

	return ExecResult{
		AssetID:    assetID,
		ExitCode:   exitCode,
		Stdout:     strings.TrimSpace(cmdResult.Output),
		DurationMs: elapsed.Milliseconds(),
	}
}

// HandleExecMulti handles POST /api/v2/exec (multi-asset fan-out).
func (d *ExecDeps) HandleExecMulti(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(d.ScopesFromContext(r.Context()), "assets:exec") {
		apiv2.WriteScopeForbidden(w, "assets:exec")
		return
	}

	var req ExecMultiRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.Command = strings.TrimSpace(req.Command)
	if req.Command == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "command is required")
		return
	}
	if req.Timeout <= 0 {
		req.Timeout = DefaultExecTimeout
	}
	if req.Timeout > MaxExecTimeout {
		req.Timeout = MaxExecTimeout
	}

	// Resolve targets.
	targets := req.Targets
	if req.Group != "" && len(targets) == 0 {
		// Resolve group members via GroupAssetStore if available.
		if groupAssetStore, ok := d.AssetStore.(persistence.GroupAssetStore); ok {
			members, err := groupAssetStore.ListAssetsByGroup(req.Group)
			if err == nil {
				for _, m := range members {
					targets = append(targets, m.ID)
				}
			}
		}
	}
	if len(targets) == 0 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "targets or group required")
		return
	}

	// Filter by asset allowlist.
	allowed := d.AllowedAssetsFromContext(r.Context())
	var filteredTargets []string
	for _, t := range targets {
		if apiv2.AssetCheck(allowed, t) {
			filteredTargets = append(filteredTargets, t)
		}
	}

	if len(filteredTargets) == 0 {
		apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "none of the requested targets are accessible with this API key")
		return
	}

	// Fan-out execution in parallel.
	results := make(map[string]ExecResult)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, target := range filteredTargets {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			result := d.ExecOnAsset(r, t, req.Command, req.Timeout)
			mu.Lock()
			results[t] = result
			mu.Unlock()
		}(target)
	}
	wg.Wait()

	succeeded := 0
	failed := 0
	for _, res := range results {
		if res.Error != "" {
			failed++
		} else {
			succeeded++
		}
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"summary": map[string]int{
			"total":     len(results),
			"succeeded": succeeded,
			"failed":    failed,
		},
	})
}
