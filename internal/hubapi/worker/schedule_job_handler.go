package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/schedules"
)

const (
	maxResolvedScheduleTargets  = 500
	scheduleExecutionWorkers    = 4
	scheduleExecutionTimeout    = 5 * time.Minute
	scheduleLookupTimeout       = 5 * time.Second
	scheduleTargetTimeoutSecond = 30
)

type scheduleTargetResult struct {
	status string
	reason string
}

type scheduleDefinitionError struct {
	message string
}

func (e *scheduleDefinitionError) Error() string { return e.message }

// HandleScheduleRunJob executes one durable schedule occurrence. Command
// outcomes are terminal (not retried) because replaying an arbitrary command
// can be destructive; queue retries are reserved for failures before target
// execution begins.
func (d *Deps) HandleScheduleRunJob(processed *atomic.Uint64) jobqueue.HandlerFunc {
	return func(baseContext context.Context, queueJob *jobqueue.Job) error {
		var execution schedules.ExecutionJob
		if err := json.Unmarshal(queueJob.Payload, &execution); err != nil {
			return fmt.Errorf("decode schedule execution job: %w", err)
		}
		if strings.TrimSpace(execution.JobID) == "" || execution.JobID != queueJob.ID || strings.TrimSpace(execution.ScheduleID) == "" {
			return fmt.Errorf("schedule execution job identity mismatch")
		}
		if strings.TrimSpace(execution.Command) == "" || strings.TrimSpace(execution.ActorID) == "" || execution.ScheduledFor.IsZero() {
			return fmt.Errorf("schedule execution payload is incomplete")
		}
		if d.ScheduleStore == nil || d.ScheduleExecutionStore == nil || d.ActionStore == nil {
			return fmt.Errorf("schedule execution stores are unavailable")
		}

		// Read-only preparation must not consume the durable dispatch marker. A
		// transient lookup failure can then use the queue's bounded retry budget.
		targets, resolveErr := d.resolveScheduleExecutionTargets(baseContext, execution)
		var definitionErr *scheduleDefinitionError
		if resolveErr != nil && !errors.As(resolveErr, &definitionErr) {
			return fmt.Errorf("resolve schedule targets: %w", resolveErr)
		}
		if err := baseContext.Err(); err != nil {
			return err
		}
		// This remains the authoritative claim/definition fence, including when
		// preparation found an invalid target. Finish requires a running owner.
		began, err := d.beginExecution(baseContext, queueJob)
		if err != nil {
			return err
		}
		if !began {
			processed.Add(1)
			return nil
		}
		if definitionErr != nil {
			if err := d.completeScheduleExecution(baseContext, queueJob, execution, "failed", definitionErr.Error()); err != nil {
				return err
			}
			processed.Add(1)
			return nil
		}
		runContext, cancel := context.WithTimeout(baseContext, scheduleExecutionTimeout)
		defer cancel()
		results := make([]scheduleTargetResult, len(targets))
		work := make(chan int, len(targets))
		for index := range targets {
			work <- index
		}
		close(work)

		workerCount := min(scheduleExecutionWorkers, len(targets))
		var wg sync.WaitGroup
		wg.Add(workerCount)
		for range workerCount {
			go func() {
				defer wg.Done()
				for index := range work {
					if runContext.Err() != nil {
						results[index] = scheduleTargetResult{status: "failed", reason: "schedule cancelled; unstarted target was not dispatched"}
						continue
					}
					results[index] = d.executeScheduleTarget(runContext, queueJob, execution, targets[index])
				}
			}()
		}
		wg.Wait()

		succeeded := 0
		blocked := 0
		failed := 0
		for _, result := range results {
			switch result.status {
			case actions.StatusSucceeded:
				succeeded++
			case "blocked":
				blocked++
			default:
				failed++
			}
		}
		status := actions.StatusSucceeded
		errorMessage := ""
		switch {
		case succeeded == len(results):
		case blocked == len(results):
			status = "blocked"
			errorMessage = "all target commands were blocked by active maintenance windows"
		case succeeded > 0:
			status = "partial"
			errorMessage = fmt.Sprintf("%d of %d target commands failed or were blocked", failed+blocked, len(results))
		default:
			status = actions.StatusFailed
			errorMessage = fmt.Sprintf("all %d target commands failed", len(results))
		}

		if runContext.Err() != nil {
			status = actions.StatusFailed
			errorMessage = "schedule cancelled; outcome unknown for commands already dispatched"
		}
		if err := d.completeScheduleExecution(baseContext, queueJob, execution, status, errorMessage); err != nil {
			return err
		}
		d.appendScheduleAudit(execution, status, len(results), succeeded, failed, blocked)
		if d.Broadcast != nil {
			d.Broadcast("schedule.completed", map[string]any{
				"schedule_id":  execution.ScheduleID,
				"status":       status,
				"target_count": len(results),
			})
		}
		processed.Add(1)
		return nil
	}
}

func (d *Deps) executeScheduleTarget(ctx context.Context, queueJob *jobqueue.Job, execution schedules.ExecutionJob, target string) scheduleTargetResult {
	if d.AuthorizeScheduleTarget == nil {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "schedule authorization is unavailable"}
	}
	if err := d.AuthorizeScheduleTarget(ctx, execution.ActorID, target); err != nil {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "schedule creator is no longer authorized"}
	}
	if d.GetPolicyConfig == nil {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "command policy is unavailable"}
	}
	check := policy.Evaluate(policy.CheckRequest{
		ActorID: execution.ActorID,
		Target:  target,
		Mode:    "structured",
		Action:  "command_execute",
		Command: execution.Command,
	}, d.GetPolicyConfig())
	if !check.Allowed {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "command denied by current policy"}
	}
	if d.EvaluateAssetGuardrailsContext == nil {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "maintenance guardrails are unavailable"}
	}
	lookupContext, cancel := context.WithTimeout(ctx, scheduleLookupTimeout)
	guardrails, err := d.EvaluateAssetGuardrailsContext(lookupContext, target, time.Now().UTC())
	cancel()
	if err != nil {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "maintenance guardrail evaluation failed"}
	}
	if guardrails.BlockActions {
		return scheduleTargetResult{status: "blocked", reason: "blocked by active maintenance window"}
	}

	actionJob, err := d.JobExecutionStore.StartScheduledAction(ctx, queueJob, target)
	if err != nil {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "failed to claim scheduled child"}
	}

	var result actions.Result
	if d.AgentMgr == nil || d.ExecuteViaAgentContext == nil || !d.AgentMgr.IsConnected(target) {
		result = actions.Result{
			JobID:       actionJob.JobID,
			RunID:       actionJob.RunID,
			Status:      actions.StatusFailed,
			Error:       "agent not connected",
			Steps:       []actions.StepResult{{Name: "execute_command", Status: actions.StatusFailed, Error: "agent not connected"}},
			CompletedAt: time.Now().UTC(),
		}
	} else {
		result = d.ExecuteCommandActionContext(ctx, actionJob)
	}
	if err := d.JobExecutionStore.FinishScheduledAction(ctx, queueJob, actionJob, result); err != nil {
		log.Printf("schedule runner: failed to persist action result for %s: %v", target, err)
		return scheduleTargetResult{status: actions.StatusFailed, reason: "failed to persist action result"}
	}
	if result.Status != actions.StatusSucceeded {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "command execution failed"}
	}
	return scheduleTargetResult{status: actions.StatusSucceeded}
}

func (d *Deps) resolveScheduleExecutionTargets(parent context.Context, execution schedules.ExecutionJob) ([]string, error) {
	ctx, cancel := context.WithTimeout(parent, scheduleLookupTimeout)
	defer cancel()
	assetStore, assetsOK := d.AssetStore.(persistence.AlertAssetContextStore)
	groupStore, groupsOK := d.GroupStore.(persistence.AlertGroupContextStore)
	if !assetsOK || !groupsOK {
		return nil, fmt.Errorf("bounded asset or group inventory unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	assetList, err := assetStore.ListAssetsContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list schedule assets: %w", err)
	}
	groupList, err := groupStore.ListGroupsContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list schedule groups: %w", err)
	}
	assetsByID := make(map[string]struct{}, len(assetList))
	for _, asset := range assetList {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		assetsByID[strings.TrimSpace(asset.ID)] = struct{}{}
	}
	groupParents := make(map[string]string, len(groupList))
	for _, group := range groupList {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		groupParents[strings.TrimSpace(group.ID)] = strings.TrimSpace(group.ParentGroupID)
	}

	directAssets := make(map[string]struct{}, len(execution.Targets))
	selectedGroups := make(map[string]struct{}, len(execution.Targets)+1)
	selectors := append([]string(nil), execution.Targets...)
	if strings.TrimSpace(execution.GroupID) != "" {
		selectors = append(selectors, execution.GroupID)
	}
	for _, selector := range selectors {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		if _, ok := assetsByID[selector]; ok {
			directAssets[selector] = struct{}{}
			continue
		}
		if _, ok := groupParents[selector]; ok {
			selectedGroups[selector] = struct{}{}
			continue
		}
		return nil, &scheduleDefinitionError{message: fmt.Sprintf("schedule target %q no longer exists", selector)}
	}
	if len(directAssets) == 0 && len(selectedGroups) == 0 {
		return nil, &scheduleDefinitionError{message: "schedule has no concrete target or group"}
	}

	resolved := make(map[string]struct{}, len(directAssets))
	for assetID := range directAssets {
		resolved[assetID] = struct{}{}
	}
	for _, asset := range assetList {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		groupID := strings.TrimSpace(asset.GroupID)
		if groupID == "" || !groupIsSelected(groupID, selectedGroups, groupParents) {
			continue
		}
		resolved[strings.TrimSpace(asset.ID)] = struct{}{}
		if len(resolved) > maxResolvedScheduleTargets {
			return nil, &scheduleDefinitionError{message: fmt.Sprintf("schedule resolves to more than %d assets", maxResolvedScheduleTargets)}
		}
	}
	if len(resolved) == 0 {
		return nil, &scheduleDefinitionError{message: "schedule target groups contain no assets"}
	}
	targets := make([]string, 0, len(resolved))
	for target := range resolved {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func groupIsSelected(groupID string, selected map[string]struct{}, parents map[string]string) bool {
	visited := make(map[string]struct{}, len(parents))
	for groupID != "" {
		if _, ok := selected[groupID]; ok {
			return true
		}
		if _, seen := visited[groupID]; seen {
			return false
		}
		visited[groupID] = struct{}{}
		groupID = parents[groupID]
	}
	return false
}

func (d *Deps) completeScheduleExecution(ctx context.Context, job *jobqueue.Job, execution schedules.ExecutionJob, status, errorMessage string) error {
	if err := d.JobExecutionStore.FinishJobExecution(ctx, job, persistence.JobExecutionResult{
		Schedule: &persistence.ScheduleExecutionResult{Status: status, Error: errorMessage},
	}); err != nil {
		return fmt.Errorf("persist schedule completion: %w", err)
	}
	return nil
}

func (d *Deps) appendScheduleAudit(execution schedules.ExecutionJob, status string, targetCount, succeeded, failed, blocked int) {
	if d.AuditStore == nil {
		return
	}
	event := audit.NewEvent("schedules.run.completed")
	event.ID = "audit_" + execution.JobID
	event.ActorID = execution.ActorID
	event.SessionID = execution.ScheduleID
	event.Decision = status
	event.Details = map[string]any{
		"job_id":        execution.JobID,
		"scheduled_for": execution.ScheduledFor.UTC(),
		"target_count":  targetCount,
		"succeeded":     succeeded,
		"failed":        failed,
		"blocked":       blocked,
	}
	if err := d.AuditStore.Append(event); err != nil {
		log.Printf("schedule runner: failed to append completion audit: %v", err)
	}
}
