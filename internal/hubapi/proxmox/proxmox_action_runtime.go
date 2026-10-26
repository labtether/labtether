package proxmox

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assetid"
	proxmoxconnector "github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/hubcollector"
)

const (
	proxmoxActionPollInterval = 2 * time.Second
	proxmoxActionWaitTimeout  = 5 * time.Minute
)

type ProxmoxActionExecution struct {
	Status   string
	Message  string
	Output   string
	Metadata map[string]string
}

func (d *Deps) ExecuteActionInProcess(job actions.Job) actions.Result {
	return d.executeActionInProcess(context.Background(), job, true)
}

func (d *Deps) ExecuteActionInProcessContext(ctx context.Context, job actions.Job) actions.Result {
	return d.executeActionInProcess(ctx, job, false)
}

func (d *Deps) executeActionInProcess(ctx context.Context, job actions.Job, legacy bool) actions.Result {
	if ctx.Err() != nil {
		return failedConnectorActionResult(job, "action canceled before dispatch; no changes applied")
	}
	runType := actions.NormalizeRunType(job.Type)
	if runType == actions.RunTypeCommand {
		if d.ExecuteCommandActionContext != nil {
			return d.ExecuteCommandActionContext(ctx, job)
		}
		if legacy && d.ExecuteCommandAction != nil {
			return d.ExecuteCommandAction(job)
		}
		return failedConnectorActionResult(job, "context-aware command executor unavailable; no changes applied")
	}
	if runType == actions.RunTypeConnectorAction && strings.EqualFold(strings.TrimSpace(job.ConnectorID), "proxmox") {
		// A raw provider reference has no durable asset owner to recover for
		// jobs queued before collector binding was stored.
		if strings.Contains(job.Target, "/") && strings.TrimSpace(job.Params["collector_id"]) == "" {
			return failedConnectorActionResult(job, "Proxmox action has no saved collector binding; resubmit the action")
		}
		// 6-minute timeout: allows 30s invoke + 5m10s task wait + margin.
		actionCtx, actionCancel := context.WithTimeout(ctx, 6*time.Minute)
		defer actionCancel()
		execResult, err := d.ExecuteProxmoxAction(actionCtx, job.ActionID, job.Target, job.Params, job.DryRun)
		if err != nil {
			msg := ProxmoxActionErrorMessage(err)
			if actionCtx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				msg = "action interrupted; outcome unknown: " + msg
			}
			return actions.Result{
				JobID:       job.JobID,
				RunID:       job.RunID,
				Status:      actions.StatusFailed,
				Error:       msg,
				Output:      msg,
				Steps:       []actions.StepResult{{Name: "connector_execute", Status: actions.StatusFailed, Error: msg}},
				CompletedAt: time.Now().UTC(),
			}
		}

		status := actions.StatusSucceeded
		if strings.EqualFold(strings.TrimSpace(execResult.Status), "failed") {
			status = actions.StatusFailed
		}
		errMessage := strings.TrimSpace(execResult.Message)
		output := ProxmoxActionOutput(execResult)
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
	if runType == actions.RunTypeConnectorAction {
		if configured, ok := d.ConfiguredConnectorActions[strings.ToLower(strings.TrimSpace(job.ConnectorID))]; ok {
			bindingParam := configured.BindingParam
			if bindingParam == "" {
				bindingParam = "collector_id"
			}
			if configured.Execute == nil || strings.TrimSpace(job.Params[bindingParam]) == "" {
				return failedConnectorActionResult(job, "connector action has no saved runtime binding; resubmit the action")
			}
			actionCtx := ctx
			if !configured.UseCallerTimeout {
				var cancel context.CancelFunc
				actionCtx, cancel = context.WithTimeout(ctx, time.Minute)
				defer cancel()
			}
			if actionCtx.Err() != nil {
				return failedConnectorActionResult(job, "action canceled before dispatch; no changes applied")
			}
			// Execute consumes the saved resource and checks its current maintenance
			// owners. Rebinding here could silently change a queued action's target.
			result, err := configured.Execute(actionCtx, job.ActionID, connectorsdk.ActionRequest{TargetID: job.Target, Params: job.Params, DryRun: job.DryRun})
			if actionCtx.Err() != nil {
				err = actionCtx.Err()
			}
			var blocked *maintenanceguard.BlockedActionError
			if errors.As(err, &blocked) {
				return failedConnectorActionResult(job, blocked.Error())
			}
			return operations.ConnectorExecutionResult(job, result, err)
		}
	}
	if runType == actions.RunTypeConnectorAction {
		if err := maintenanceguard.CheckConnectorExecution(job.ConnectorID, job.ActionID, connectorsdk.ActionRequest{TargetID: job.Target, Params: job.Params, DryRun: job.DryRun}, d.EvaluateConnectorGuardrails); err != nil {
			return failedConnectorActionResult(job, err.Error())
		}
		if err := d.validateGenericConnectorActionRoute(job); err != nil {
			return failedConnectorActionResult(job, err.Error())
		}
	}

	if ctx.Err() != nil {
		return failedConnectorActionResult(job, "action canceled before dispatch; no changes applied")
	}
	if d.ExecuteActionInProcessContextFn != nil {
		return d.ExecuteActionInProcessContextFn(ctx, job, d.ConnectorRegistry)
	}
	if legacy && d.ExecuteActionInProcessFn != nil {
		return d.ExecuteActionInProcessFn(job, d.ConnectorRegistry)
	}
	return operations.ExecuteActionInProcessContext(ctx, job, d.ConnectorRegistry)
}

func (d *Deps) validateGenericConnectorActionRoute(job actions.Job) error {
	collectorType := collectorTypeForConnectorAction(job.ConnectorID)
	if collectorType == "" || collectorType == hubcollector.CollectorTypeProxmox {
		return nil
	}

	target := strings.TrimSpace(job.Target)
	assetCollectorID := ""
	targetIsScoped := false
	if _, ok := assetid.CollectorScopeFromAssetID(target); ok {
		targetIsScoped = true
	}
	if d.AssetStore != nil && target != "" {
		asset, ok, err := d.AssetStore.GetAsset(target)
		if err != nil {
			return fmt.Errorf("cannot verify connector action target ownership")
		}
		if ok {
			assetCollectorID = strings.TrimSpace(asset.Metadata["collector_id"])
			if sourceType := collectorTypeForConnectorAction(asset.Source); sourceType != "" && sourceType != collectorType {
				return fmt.Errorf("connector action target belongs to %s, not %s", sourceType, collectorType)
			}
		}
	}

	if d.HubCollectorStore == nil {
		if targetIsScoped || assetCollectorID != "" {
			return fmt.Errorf("cannot verify collector-aware action routing for %s", collectorType)
		}
		return nil
	}
	collectors, err := d.HubCollectorStore.ListHubCollectors(200, true)
	if err != nil {
		if targetIsScoped || assetCollectorID != "" {
			return fmt.Errorf("cannot verify collector-aware action routing for %s", collectorType)
		}
		return nil
	}

	matching := make([]hubcollector.Collector, 0, 2)
	for _, collector := range collectors {
		if hubcollector.NormalizeCollectorType(collector.CollectorType) == collectorType {
			matching = append(matching, collector)
		}
	}
	if len(matching) > 1 {
		return fmt.Errorf("multiple active %s collectors are configured; generic connector actions are disabled until collector-aware runtime dispatch is available", collectorType)
	}
	if assetCollectorID != "" {
		if len(matching) == 0 || strings.TrimSpace(matching[0].ID) != assetCollectorID {
			return fmt.Errorf("connector action target references an inactive or mismatched %s collector", collectorType)
		}
	}
	return nil
}

func collectorTypeForConnectorAction(connectorID string) string {
	switch strings.ToLower(strings.TrimSpace(connectorID)) {
	case "home-assistant", hubcollector.CollectorTypeHomeAssistant:
		return hubcollector.CollectorTypeHomeAssistant
	case hubcollector.CollectorTypePortainer:
		return hubcollector.CollectorTypePortainer
	case hubcollector.CollectorTypePBS:
		return hubcollector.CollectorTypePBS
	case hubcollector.CollectorTypeTrueNAS:
		return hubcollector.CollectorTypeTrueNAS
	case hubcollector.CollectorTypeProxmox:
		return hubcollector.CollectorTypeProxmox
	case hubcollector.CollectorTypeDocker:
		return hubcollector.CollectorTypeDocker
	default:
		return ""
	}
}

func failedConnectorActionResult(job actions.Job, message string) actions.Result {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "connector action routing failed"
	}
	return actions.Result{
		JobID:       job.JobID,
		RunID:       job.RunID,
		Status:      actions.StatusFailed,
		Error:       message,
		Output:      message,
		Steps:       []actions.StepResult{{Name: "connector_route", Status: actions.StatusFailed, Error: message}},
		CompletedAt: time.Now().UTC(),
	}
}

func ProxmoxActionErrorMessage(err error) string {
	if err == nil {
		return "proxmox action execution failed"
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "proxmox action execution failed"
	}
	return msg
}

func ProxmoxActionOutput(result ProxmoxActionExecution) string {
	output := strings.TrimSpace(result.Output)
	if output != "" {
		return output
	}
	return strings.TrimSpace(result.Message)
}

func (d *Deps) ExecuteProxmoxActionDirect(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	result, err := d.ExecuteProxmoxAction(ctx, actionID, req.TargetID, req.Params, req.DryRun)
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	return connectorsdk.ActionResult{
		Status:   strings.TrimSpace(result.Status),
		Message:  strings.TrimSpace(result.Message),
		Output:   strings.TrimSpace(result.Output),
		Metadata: result.Metadata,
	}, nil
}

func (d *Deps) ExecuteProxmoxAction(ctx context.Context, actionID, target string, params map[string]string, dryRun bool) (ProxmoxActionExecution, error) {
	if err := ctx.Err(); err != nil {
		return ProxmoxActionExecution{}, err
	}
	actionID = resolveProxmoxActionID(actionID, target)
	if err := proxmoxconnector.ValidateTaskAction(actionID, params); err != nil {
		return ProxmoxActionExecution{}, err
	}
	node, vmid, collectorID, err := d.ResolveProxmoxActionDestination(actionID, target, params)
	if err != nil {
		return ProxmoxActionExecution{}, err
	}
	// Bind the dispatch check to the exact destination already resolved above.
	checkedParams := maps.Clone(params)
	if checkedParams == nil {
		checkedParams = make(map[string]string)
	}
	checkedParams["collector_id"] = collectorID
	if err := maintenanceguard.CheckConnectorExecution("proxmox", actionID, connectorsdk.ActionRequest{TargetID: node + "/" + vmid, Params: checkedParams, DryRun: dryRun}, d.EvaluateConnectorGuardrails); err != nil {
		return ProxmoxActionExecution{}, err
	}
	invokeParams := StripInternalProxmoxActionParams(params)

	if dryRun {
		targetLabel := node + "/" + vmid
		return ProxmoxActionExecution{
			Status:  "succeeded",
			Message: "dry-run: action validated",
			Output:  fmt.Sprintf("would execute %s on %s", actionID, targetLabel),
			Metadata: map[string]string{
				"target":       targetLabel,
				"collector_id": strings.TrimSpace(collectorID),
				"elapsed_ms":   "0",
			},
		}, nil
	}

	runtime, err := d.LoadProxmoxRuntime(collectorID)
	if err != nil {
		return ProxmoxActionExecution{}, err
	}

	targetLabel := node + "/" + vmid
	started := time.Now()
	invokeCtx, cancelInvoke := context.WithTimeout(ctx, 30*time.Second)
	defer cancelInvoke()

	upid, err := InvokeProxmoxAction(invokeCtx, runtime, actionID, node, vmid, invokeParams)
	if err != nil {
		return ProxmoxActionExecution{}, err
	}

	metadata := map[string]string{
		"target":       targetLabel,
		"collector_id": strings.TrimSpace(runtime.collectorID),
	}
	if strings.TrimSpace(upid) != "" {
		metadata["upid"] = strings.TrimSpace(upid)
	}

	if strings.TrimSpace(upid) == "" {
		metadata["elapsed_ms"] = fmt.Sprintf("%d", time.Since(started).Milliseconds())
		return ProxmoxActionExecution{
			Status:   "succeeded",
			Message:  "action completed",
			Output:   fmt.Sprintf("%s on %s", actionID, targetLabel),
			Metadata: metadata,
		}, nil
	}

	waitCtx, cancelWait := context.WithTimeout(ctx, proxmoxActionWaitTimeout+10*time.Second)
	defer cancelWait()
	taskStatus, err := runtime.client.WaitForTask(waitCtx, node, upid, proxmoxActionPollInterval, proxmoxActionWaitTimeout)
	elapsed := time.Since(started).Milliseconds()
	metadata["elapsed_ms"] = fmt.Sprintf("%d", elapsed)
	if err != nil {
		return ProxmoxActionExecution{
			Status:   "failed",
			Message:  "task status unavailable; outcome unknown: " + err.Error(),
			Output:   fmt.Sprintf("%s on %s (upid=%s)", actionID, targetLabel, upid),
			Metadata: metadata,
		}, nil
	}

	exitStatus := strings.TrimSpace(taskStatus.ExitStatus)
	if exitStatus == "" {
		exitStatus = "OK"
	}
	metadata["exitstatus"] = exitStatus

	result := ProxmoxActionExecution{
		Status:   "succeeded",
		Message:  "action completed",
		Output:   fmt.Sprintf("%s on %s (upid=%s, exit=%s)", actionID, targetLabel, upid, exitStatus),
		Metadata: metadata,
	}
	if !strings.EqualFold(exitStatus, "OK") {
		result.Status = "failed"
		result.Message = fmt.Sprintf("task finished with exitstatus %s", exitStatus)
	}
	return result, nil
}

func StripInternalProxmoxActionParams(params map[string]string) map[string]string {
	if len(params) == 0 {
		return nil
	}

	filtered := make(map[string]string, len(params))
	for key, value := range params {
		if strings.EqualFold(strings.TrimSpace(key), "collector_id") {
			continue
		}
		filtered[key] = value
	}
	return filtered
}

func InvokeProxmoxAction(ctx context.Context, runtime *ProxmoxRuntime, actionID, node, vmid string, params map[string]string) (string, error) {
	if runtime == nil || runtime.client == nil {
		return "", fmt.Errorf("proxmox runtime unavailable")
	}
	return runtime.client.ExecuteTaskAction(ctx, actionID, node, vmid, params)
}
