package proxmox

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/connectorsdk"
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
	runType := actions.NormalizeRunType(job.Type)
	if runType == actions.RunTypeCommand {
		return d.ExecuteCommandAction(job)
	}
	if runType == actions.RunTypeConnectorAction && strings.EqualFold(strings.TrimSpace(job.ConnectorID), "proxmox") {
		// 6-minute timeout: allows 30s invoke + 5m10s task wait + margin.
		actionCtx, actionCancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer actionCancel()
		execResult, err := d.ExecuteProxmoxAction(actionCtx, job.ActionID, job.Target, job.Params, job.DryRun)
		if err != nil {
			msg := ProxmoxActionErrorMessage(err)
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

	return d.ExecuteActionInProcessFn(job, d.ConnectorRegistry)
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
	actionID = strings.TrimSpace(actionID)
	node, vmid, collectorID, err := d.ResolveProxmoxActionTarget(actionID, target)
	if err != nil {
		return ProxmoxActionExecution{}, err
	}
	if strings.TrimSpace(collectorID) == "" {
		collectorID = strings.TrimSpace(params["collector_id"])
	}
	collectorID, err = d.resolveProxmoxActionCollectorID(collectorID)
	if err != nil {
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
			Message:  err.Error(),
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

func (d *Deps) resolveProxmoxActionCollectorID(collectorID string) (string, error) {
	collectorID = strings.TrimSpace(collectorID)
	if collectorID != "" {
		return collectorID, nil
	}
	if d.HubCollectorStore == nil {
		return "", nil
	}

	collectors, err := d.HubCollectorStore.ListHubCollectors(200, true)
	if err != nil {
		return "", fmt.Errorf("failed to list hub collectors: %w", err)
	}

	proxmoxCollectorID := ""
	proxmoxCollectorCount := 0
	for _, collector := range collectors {
		if collector.CollectorType != hubcollector.CollectorTypeProxmox {
			continue
		}
		proxmoxCollectorCount++
		if proxmoxCollectorID == "" {
			proxmoxCollectorID = strings.TrimSpace(collector.ID)
		}
		if proxmoxCollectorCount > 1 {
			return "", fmt.Errorf("collector_id is required when multiple proxmox collectors are configured")
		}
	}

	return proxmoxCollectorID, nil
}

func (d *Deps) ResolveProxmoxActionTarget(actionID, target string) (node, vmid, collectorID string, err error) {
	actionID = strings.TrimSpace(actionID)
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", "", fmt.Errorf("target is required")
	}

	if strings.Contains(target, "/") {
		parts := strings.SplitN(target, "/", 2)
		node = strings.TrimSpace(parts[0])
		vmid = strings.TrimSpace(parts[1])
		if node == "" || vmid == "" {
			return "", "", "", fmt.Errorf("target must be node/vmid")
		}
		return node, vmid, "", nil
	}

	expectedKind := ""
	switch {
	case strings.HasPrefix(actionID, "vm."):
		expectedKind = "qemu"
	case strings.HasPrefix(actionID, "ct."):
		expectedKind = "lxc"
	default:
		return "", "", "", fmt.Errorf("unsupported proxmox action prefix: %s", actionID)
	}

	resolved, ok, resolveErr := d.ResolveProxmoxSessionTarget(target)
	if resolveErr != nil {
		return "", "", "", resolveErr
	}
	if !ok {
		return "", "", "", fmt.Errorf("target %s is not a proxmox asset", target)
	}
	if err := ValidateResolvedProxmoxActionTarget(resolved, expectedKind, actionID); err != nil {
		return "", "", "", err
	}
	return strings.TrimSpace(resolved.Node), strings.TrimSpace(resolved.VMID), strings.TrimSpace(resolved.CollectorID), nil
}

func ValidateResolvedProxmoxActionTarget(resolved ProxmoxSessionTarget, expectedKind, actionID string) error {
	if strings.ToLower(strings.TrimSpace(resolved.Kind)) != expectedKind {
		return fmt.Errorf("target kind %s does not match action %s", resolved.Kind, actionID)
	}
	if strings.TrimSpace(resolved.Node) == "" || strings.TrimSpace(resolved.VMID) == "" {
		return fmt.Errorf("proxmox asset target is incomplete")
	}
	return nil
}

func InvokeProxmoxAction(ctx context.Context, runtime *ProxmoxRuntime, actionID, node, vmid string, params map[string]string) (string, error) {
	if runtime == nil || runtime.client == nil {
		return "", fmt.Errorf("proxmox runtime unavailable")
	}

	switch actionID {
	case "vm.start":
		return runtime.client.StartVM(ctx, node, vmid)
	case "vm.stop":
		return runtime.client.StopVM(ctx, node, vmid)
	case "vm.shutdown":
		return runtime.client.ShutdownVM(ctx, node, vmid)
	case "vm.reboot":
		return runtime.client.RebootVM(ctx, node, vmid)
	case "vm.snapshot":
		name := strings.TrimSpace(params["snapshot_name"])
		if name == "" {
			name = fmt.Sprintf("labtether-%d", time.Now().UTC().Unix())
		}
		return runtime.client.SnapshotVM(ctx, node, vmid, name)
	case "vm.migrate":
		targetNode := strings.TrimSpace(params["target_node"])
		if targetNode == "" {
			return "", fmt.Errorf("target_node is required")
		}
		return runtime.client.MigrateVM(ctx, node, vmid, targetNode)
	case "ct.start":
		return runtime.client.StartCT(ctx, node, vmid)
	case "ct.stop":
		return runtime.client.StopCT(ctx, node, vmid)
	case "ct.shutdown":
		return runtime.client.ShutdownCT(ctx, node, vmid)
	case "ct.reboot":
		return runtime.client.RebootCT(ctx, node, vmid)
	case "ct.snapshot":
		name := strings.TrimSpace(params["snapshot_name"])
		if name == "" {
			name = fmt.Sprintf("labtether-%d", time.Now().UTC().Unix())
		}
		return runtime.client.SnapshotCT(ctx, node, vmid, name)
	// Phase 2: Extended lifecycle
	case "vm.suspend":
		return runtime.client.SuspendVM(ctx, node, vmid)
	case "vm.resume":
		return runtime.client.ResumeVM(ctx, node, vmid)
	case "vm.force_stop":
		return runtime.client.StopVM(ctx, node, vmid)
	case "ct.force_stop":
		return runtime.client.StopCT(ctx, node, vmid)
	case "vm.snapshot.delete":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return runtime.client.DeleteQemuSnapshot(ctx, node, vmid, snapName)
	case "vm.snapshot.rollback":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return runtime.client.RollbackQemuSnapshot(ctx, node, vmid, snapName)
	case "ct.snapshot.delete":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return runtime.client.DeleteLXCSnapshot(ctx, node, vmid, snapName)
	case "ct.snapshot.rollback":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return runtime.client.RollbackLXCSnapshot(ctx, node, vmid, snapName)
	case "ct.migrate":
		targetNode := strings.TrimSpace(params["target_node"])
		if targetNode == "" {
			return "", fmt.Errorf("target_node is required")
		}
		return runtime.client.MigrateCT(ctx, node, vmid, targetNode)
	// Phase 4: Extended features
	case "vm.backup", "ct.backup":
		return runtime.client.TriggerBackup(ctx, node, vmid, params["storage"], params["mode"])
	case "vm.clone":
		newIDStr := strings.TrimSpace(params["new_id"])
		if newIDStr == "" {
			return "", fmt.Errorf("new_id is required")
		}
		newID, parseErr := strconv.Atoi(newIDStr)
		if parseErr != nil {
			return "", fmt.Errorf("new_id must be numeric")
		}
		return runtime.client.CloneVM(ctx, node, vmid, params["new_name"], newID)
	case "ct.clone":
		newIDStr := strings.TrimSpace(params["new_id"])
		if newIDStr == "" {
			return "", fmt.Errorf("new_id is required")
		}
		newID, parseErr := strconv.Atoi(newIDStr)
		if parseErr != nil {
			return "", fmt.Errorf("new_id must be numeric")
		}
		return runtime.client.CloneCT(ctx, node, vmid, params["new_name"], newID)
	case "vm.disk_resize":
		disk := strings.TrimSpace(params["disk"])
		size := strings.TrimSpace(params["size"])
		if disk == "" || size == "" {
			return "", fmt.Errorf("disk and size are required")
		}
		return "", runtime.client.ResizeVMDisk(ctx, node, vmid, disk, size)
	default:
		return "", fmt.Errorf("unsupported proxmox action: %s", actionID)
	}
}
