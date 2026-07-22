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
	DefaultExecTimeout        = 30
	MaxExecTimeout            = 300
	maxExecMultiRawTargets    = 64
	maxExecMultiUniqueTargets = 64
	maxExecMultiConcurrency   = 8
)

func normalizeExecTimeoutSeconds(timeoutSec int) int {
	if timeoutSec <= 0 {
		return DefaultExecTimeout
	}
	if timeoutSec > MaxExecTimeout {
		return MaxExecTimeout
	}
	return timeoutSec
}

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

func normalizeExecMultiTargets(rawTargets []string) ([]string, error) {
	seen := make(map[string]struct{}, min(len(rawTargets), maxExecMultiUniqueTargets+1))
	targets := make([]string, 0, min(len(rawTargets), maxExecMultiUniqueTargets))
	for _, rawTarget := range rawTargets {
		if err := appendExecMultiTarget(&targets, seen, rawTarget); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

func appendExecMultiTarget(targets *[]string, seen map[string]struct{}, rawTarget string) error {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		return nil
	}
	if _, exists := seen[target]; exists {
		return nil
	}
	if len(*targets) >= maxExecMultiUniqueTargets {
		return fmt.Errorf("too many targets: maximum is %d", maxExecMultiUniqueTargets)
	}
	seen[target] = struct{}{}
	*targets = append(*targets, target)
	return nil
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
	if d.AllowedAssetsFromContext == nil || !apiv2.AssetCheck(d.AllowedAssetsFromContext(r.Context()), assetID) {
		apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "asset is not accessible with this API key")
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
	req.Timeout = normalizeExecTimeoutSeconds(req.Timeout)

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
	d.AppendAuditEventBestEffort(audit.Event{
		Type:      "api.exec",
		ActorID:   d.PrincipalActorID(r.Context()),
		Target:    assetID,
		Details:   map[string]any{"command_bytes": len([]byte(req.Command)), "exit_code": result.ExitCode},
		Timestamp: time.Now().UTC(),
	}, "v2 exec on "+assetID)
}

// ExecOnAsset runs a command on a single asset via the agent manager.
func (d *ExecDeps) ExecOnAsset(r *http.Request, assetID, command string, timeoutSec int) ExecResult {
	timeoutSec = normalizeExecTimeoutSeconds(timeoutSec)

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
		TimeoutSec:  timeoutSec,
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
	req.Timeout = normalizeExecTimeoutSeconds(req.Timeout)
	req.Group = strings.TrimSpace(req.Group)

	if len(req.Targets) > maxExecMultiRawTargets {
		apiv2.WriteError(w, http.StatusBadRequest, "validation",
			fmt.Sprintf("too many targets: maximum is %d", maxExecMultiRawTargets))
		return
	}

	// Resolve targets.
	targets, err := normalizeExecMultiTargets(req.Targets)
	if err != nil {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	if req.Group != "" && len(targets) == 0 {
		// Resolve group members via GroupAssetStore if available.
		if groupAssetStore, ok := d.AssetStore.(persistence.GroupAssetStore); ok {
			members, err := groupAssetStore.ListAssetsByGroup(req.Group)
			if err == nil {
				expandedTargets := make([]string, 0, min(len(members), maxExecMultiUniqueTargets+1))
				seenExpandedTargets := make(map[string]struct{}, min(len(members), maxExecMultiUniqueTargets+1))
				for _, m := range members {
					if err := appendExecMultiTarget(&expandedTargets, seenExpandedTargets, m.ID); err != nil {
						apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
						return
					}
				}
				targets = expandedTargets
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
	if len(filteredTargets) > maxExecMultiUniqueTargets {
		apiv2.WriteError(w, http.StatusBadRequest, "validation",
			fmt.Sprintf("too many accessible targets: maximum is %d", maxExecMultiUniqueTargets))
		return
	}

	if len(filteredTargets) == 0 {
		apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "none of the requested targets are accessible with this API key")
		return
	}

	// Fan out through a fixed-size worker pool. Results are written to stable
	// request-order slots and reduced only after all workers finish.
	orderedResults := make([]ExecResult, len(filteredTargets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workerCount := min(maxExecMultiConcurrency, len(filteredTargets))
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				orderedResults[index] = d.ExecOnAsset(r, filteredTargets[index], req.Command, req.Timeout)
			}
		}()
	}
	for index := range filteredTargets {
		jobs <- index
	}
	close(jobs)
	wg.Wait()

	results := make(map[string]ExecResult, len(filteredTargets))
	succeeded := 0
	failed := 0
	for index, target := range filteredTargets {
		res := orderedResults[index]
		results[target] = res
		if res.Error != "" || res.ExitCode != 0 {
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
