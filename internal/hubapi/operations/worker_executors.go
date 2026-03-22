package operations

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/modelmap"
	"github.com/labtether/labtether/internal/updates"
)

// UpdateScopeExecutor is a function type for executing update scopes.
type UpdateScopeExecutor func(job updates.Job, target, scope string) updates.RunResultEntry

// ExecuteActionInProcess replaces the old HTTP-based executeAction with direct
// in-process connector registry calls for connector actions.
func ExecuteActionInProcess(job actions.Job, registry *connectorsdk.Registry) actions.Result {
	switch actions.NormalizeRunType(job.Type) {
	case actions.RunTypeCommand:
		status := actions.StatusSucceeded
		output := fmt.Sprintf("simulated action command on %s: %s", job.Target, job.Command)
		if strings.Contains(strings.ToLower(job.Command), "fail") {
			status = actions.StatusFailed
			output = "simulated action command failure"
		}

		return actions.Result{
			JobID:       job.JobID,
			RunID:       job.RunID,
			Status:      status,
			Output:      output,
			Steps:       []actions.StepResult{{Name: "execute_command", Status: status, Output: output}},
			CompletedAt: time.Now().UTC(),
		}
	case actions.RunTypeConnectorAction:
		return ExecuteConnectorActionDirect(job, registry)
	default:
		return actions.Result{
			JobID:       job.JobID,
			RunID:       job.RunID,
			Status:      actions.StatusFailed,
			Error:       "unsupported action run type",
			Steps:       []actions.StepResult{{Name: "validate", Status: actions.StatusFailed, Error: "unsupported action type"}},
			CompletedAt: time.Now().UTC(),
		}
	}
}

// ExecuteConnectorActionDirect calls the connector registry directly instead of
// making an HTTP request to the connector-runtime service.
func ExecuteConnectorActionDirect(job actions.Job, registry *connectorsdk.Registry) actions.Result {
	if registry == nil {
		return actions.Result{
			JobID:       job.JobID,
			RunID:       job.RunID,
			Status:      actions.StatusFailed,
			Error:       "connector registry not available",
			Steps:       []actions.StepResult{{Name: "connector_execute", Status: actions.StatusFailed, Error: "registry unavailable"}},
			CompletedAt: time.Now().UTC(),
		}
	}

	connector, ok := registry.Get(job.ConnectorID)
	if !ok {
		return actions.Result{
			JobID:       job.JobID,
			RunID:       job.RunID,
			Status:      actions.StatusFailed,
			Error:       fmt.Sprintf("connector %s not registered", job.ConnectorID),
			Steps:       []actions.StepResult{{Name: "connector_execute", Status: actions.StatusFailed, Error: "connector not found"}},
			CompletedAt: time.Now().UTC(),
		}
	}

	req := connectorsdk.ActionRequest{
		TargetID: job.Target,
		Params:   job.Params,
		DryRun:   job.DryRun,
	}

	resolvedActionID := modelmap.ResolveActionID(job.ActionID, job.Target, connector.Actions())
	result, err := connector.ExecuteAction(context.Background(), resolvedActionID, req)
	if err != nil {
		return actions.Result{
			JobID:       job.JobID,
			RunID:       job.RunID,
			Status:      actions.StatusFailed,
			Error:       "action execution failed",
			Steps:       []actions.StepResult{{Name: "connector_execute", Status: actions.StatusFailed, Error: err.Error()}},
			CompletedAt: time.Now().UTC(),
		}
	}

	status := actions.StatusSucceeded
	output := strings.TrimSpace(result.Output)
	errMessage := strings.TrimSpace(result.Message)

	if strings.EqualFold(result.Status, "failed") {
		status = actions.StatusFailed
		if errMessage == "" {
			errMessage = "connector action failed"
		}
	}

	if output == "" {
		output = errMessage
	}
	return actions.Result{
		JobID:       job.JobID,
		RunID:       job.RunID,
		Status:      status,
		Error:       errMessage,
		Output:      output,
		Steps:       []actions.StepResult{{Name: "connector_execute", Status: status, Output: output}},
		CompletedAt: time.Now().UTC(),
	}
}

// ExecuteUpdate runs an update job using simulated execution.
func ExecuteUpdate(job updates.Job) updates.Result {
	return ExecuteUpdateWithExecutor(job, nil)
}

// ExecuteUpdateWithExecutor runs an update job, delegating to the given executor
// for each target/scope pair, or simulating if executor is nil.
func ExecuteUpdateWithExecutor(job updates.Job, executor UpdateScopeExecutor) updates.Result {
	completedAt := time.Now().UTC()
	results := make([]updates.RunResultEntry, 0, len(job.Plan.Targets)*len(job.Plan.Scopes))
	failures := 0

	targets := append([]string(nil), job.Plan.Targets...)
	if len(targets) == 0 {
		targets = []string{"global"}
	}
	scopes := append([]string(nil), job.Plan.Scopes...)
	if len(scopes) == 0 {
		scopes = append([]string(nil), updates.DefaultScopes...)
	}

	for _, target := range targets {
		for _, scope := range scopes {
			var entry updates.RunResultEntry
			if executor != nil {
				entry = executor(job, target, scope)
				if strings.TrimSpace(entry.Target) == "" {
					entry.Target = target
				}
				if strings.TrimSpace(entry.Scope) == "" {
					entry.Scope = scope
				}
				if updates.NormalizeStatus(entry.Status) == "" {
					entry.Status = updates.StatusFailed
					if strings.TrimSpace(entry.Summary) == "" {
						entry.Summary = "invalid update execution status"
					}
				}
				if strings.TrimSpace(entry.Summary) == "" {
					entry.Summary = fmt.Sprintf("completed %s on %s", scope, target)
				}
			} else {
				entry = updates.RunResultEntry{
					Target: target,
					Scope:  scope,
					Status: updates.StatusSucceeded,
				}
				if strings.Contains(strings.ToLower(target), "fail") || strings.Contains(strings.ToLower(scope), "fail") {
					entry.Status = updates.StatusFailed
					entry.Summary = "simulated update failure"
				} else if job.DryRun {
					entry.Summary = fmt.Sprintf("dry-run validated %s on %s", scope, target)
				} else {
					entry.Summary = fmt.Sprintf("applied %s on %s", scope, target)
				}
			}
			if entry.Status == updates.StatusFailed {
				failures++
			}

			results = append(results, entry)
		}
	}

	status := updates.StatusSucceeded
	summary := fmt.Sprintf("%d update tasks completed", len(results))
	errorMessage := ""
	if failures > 0 {
		status = updates.StatusFailed
		summary = fmt.Sprintf("%d tasks completed, %d failed", len(results), failures)
		errorMessage = "one or more targets failed"
	}

	return updates.Result{
		JobID:       job.JobID,
		RunID:       job.RunID,
		Status:      status,
		Summary:     summary,
		Error:       errorMessage,
		Results:     results,
		CompletedAt: completedAt,
	}
}

// HeartbeatLoop logs a periodic heartbeat.
func HeartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("labtether heartbeat loop stopped")
			return
		case <-ticker.C:
			log.Printf("labtether heartbeat: all subsystems running")
		}
	}
}
