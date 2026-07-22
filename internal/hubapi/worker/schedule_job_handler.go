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
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/schedules"
)

const (
	maxResolvedScheduleTargets  = 500
	scheduleExecutionWorkers    = 4
	scheduleExecutionTimeout    = 5 * time.Minute
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
	return func(handlerContext interface{ Deadline() (time.Time, bool) }, queueJob *jobqueue.Job) error {
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

		baseContext := context.Background()
		if typed, ok := handlerContext.(context.Context); ok {
			baseContext = typed
		}
		current, found, err := d.ScheduleStore.GetScheduledTask(baseContext, execution.ScheduleID)
		if err != nil {
			return fmt.Errorf("load current schedule: %w", err)
		}
		if !found || !current.Enabled {
			if err := d.completeScheduleExecution(execution, "skipped", "schedule was disabled or deleted before execution"); err != nil {
				return err
			}
			processed.Add(1)
			return nil
		}
		if current.LastRunJobID != execution.JobID {
			// The definition was disabled/re-enabled or a later occurrence replaced
			// this job. Never execute a stale command snapshot.
			processed.Add(1)
			return nil
		}
		switch current.LastRunStatus {
		case actions.StatusSucceeded, actions.StatusFailed, "partial", "blocked", "skipped", "cancelled":
			// The handler finished but queue completion was interrupted. Acknowledge
			// the redelivery without replaying an arbitrary command.
			processed.Add(1)
			return nil
		case "running":
			if queueJob.Attempts > 1 {
				if err := d.completeScheduleExecution(execution, actions.StatusFailed, "previous execution attempt was interrupted; commands were not replayed"); err != nil {
					return err
				}
				processed.Add(1)
				return nil
			}
			return fmt.Errorf("schedule occurrence is already running")
		case "queued":
		default:
			return fmt.Errorf("schedule occurrence has invalid state %q", current.LastRunStatus)
		}

		targets, err := d.resolveScheduleExecutionTargets(execution)
		if err != nil {
			var definitionErr *scheduleDefinitionError
			if errors.As(err, &definitionErr) {
				if completeErr := d.completeScheduleExecution(execution, "failed", definitionErr.Error()); completeErr != nil {
					return completeErr
				}
				processed.Add(1)
				return nil
			}
			return fmt.Errorf("resolve schedule targets: %w", err)
		}
		began, err := d.ScheduleExecutionStore.BeginScheduledTaskExecution(baseContext, execution.ScheduleID, execution.JobID)
		if err != nil {
			return fmt.Errorf("begin schedule execution: %w", err)
		}
		if !began {
			// State changed after the read (most commonly an operator disabled the
			// schedule). The CAS is the authority; do not dispatch.
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
						results[index] = scheduleTargetResult{status: "failed", reason: "schedule execution deadline exceeded"}
						continue
					}
					results[index] = d.executeScheduleTarget(runContext, execution, targets[index])
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

		if err := d.completeScheduleExecution(execution, status, errorMessage); err != nil {
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

func (d *Deps) executeScheduleTarget(ctx context.Context, execution schedules.ExecutionJob, target string) scheduleTargetResult {
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
	if d.EvaluateAssetGuardrails == nil {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "maintenance guardrails are unavailable"}
	}
	guardrails, err := d.EvaluateAssetGuardrails(target, time.Now().UTC())
	if err != nil {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "maintenance guardrail evaluation failed"}
	}
	if guardrails.BlockActions {
		return scheduleTargetResult{status: "blocked", reason: "blocked by active maintenance window"}
	}

	run, err := d.ActionStore.CreateActionRun(actions.ExecuteRequest{
		ActorID: execution.ActorID,
		Type:    actions.RunTypeCommand,
		Target:  target,
		Command: execution.Command,
		Params: map[string]string{
			"schedule_id":     execution.ScheduleID,
			"schedule_job_id": execution.JobID,
			"scheduled_for":   execution.ScheduledFor.UTC().Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "failed to create action run"}
	}
	actionJob := actions.Job{
		JobID:       idgen.New("schedact"),
		RunID:       run.ID,
		Type:        actions.RunTypeCommand,
		ActorID:     execution.ActorID,
		Target:      target,
		Command:     execution.Command,
		Params:      run.Params,
		TimeoutSec:  scheduleTargetTimeoutSecond,
		RequestedAt: time.Now().UTC(),
	}

	var result actions.Result
	if d.AgentMgr == nil || d.ExecuteViaAgent == nil || !d.AgentMgr.IsConnected(target) {
		result = actions.Result{
			JobID:       actionJob.JobID,
			RunID:       run.ID,
			Status:      actions.StatusFailed,
			Error:       "agent not connected",
			Steps:       []actions.StepResult{{Name: "execute_command", Status: actions.StatusFailed, Error: "agent not connected"}},
			CompletedAt: time.Now().UTC(),
		}
	} else {
		result = d.ExecuteCommandAction(actionJob)
	}
	if err := d.ActionStore.ApplyActionResult(result); err != nil {
		log.Printf("schedule runner: failed to persist action result for %s: %v", target, err)
		return scheduleTargetResult{status: actions.StatusFailed, reason: "failed to persist action result"}
	}
	if result.Status != actions.StatusSucceeded {
		return scheduleTargetResult{status: actions.StatusFailed, reason: "command execution failed"}
	}
	return scheduleTargetResult{status: actions.StatusSucceeded}
}

func (d *Deps) resolveScheduleExecutionTargets(execution schedules.ExecutionJob) ([]string, error) {
	if d.AssetStore == nil || d.GroupStore == nil {
		return nil, fmt.Errorf("asset or group store unavailable")
	}
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return nil, fmt.Errorf("list schedule assets: %w", err)
	}
	groupList, err := d.GroupStore.ListGroups()
	if err != nil {
		return nil, fmt.Errorf("list schedule groups: %w", err)
	}
	assetsByID := make(map[string]struct{}, len(assetList))
	for _, asset := range assetList {
		assetsByID[strings.TrimSpace(asset.ID)] = struct{}{}
	}
	groupParents := make(map[string]string, len(groupList))
	for _, group := range groupList {
		groupParents[strings.TrimSpace(group.ID)] = strings.TrimSpace(group.ParentGroupID)
	}

	directAssets := make(map[string]struct{}, len(execution.Targets))
	selectedGroups := make(map[string]struct{}, len(execution.Targets)+1)
	selectors := append([]string(nil), execution.Targets...)
	if strings.TrimSpace(execution.GroupID) != "" {
		selectors = append(selectors, execution.GroupID)
	}
	for _, selector := range selectors {
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

func (d *Deps) completeScheduleExecution(execution schedules.ExecutionJob, status, errorMessage string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.ScheduleExecutionStore.CompleteScheduledTaskExecution(
		ctx,
		execution.ScheduleID,
		execution.JobID,
		status,
		errorMessage,
		time.Now().UTC(),
	); err != nil {
		// Returning an error retries only durable finalization. A redelivery sees
		// the running occurrence and explicitly refuses to replay any command.
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
