package proxmox

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/connectorsdk"
	"strconv"
	"strings"
	"time"
)

func (c *Connector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	if c.clientErr != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: c.clientErr.Error()}, nil
	}
	if !c.isConfigured() {
		return connectorsdk.ActionResult{
			Status:  "failed",
			Message: "proxmox connector is not configured; actions are unavailable",
		}, nil
	}

	node, vmid, err := parseComputeTarget(req, c.defaultNode)
	if err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
	}
	if err := ValidateTaskAction(actionID, req.Params); err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
	}
	started := time.Now().UTC()

	targetLabel := node + "/" + vmid
	if req.DryRun {
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: "dry-run: action validated",
			Output:  fmt.Sprintf("would execute %s on %s", actionID, targetLabel),
		}, nil
	}

	upid, invokeErr := c.client.ExecuteTaskAction(ctx, actionID, node, vmid, req.Params)
	if invokeErr != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: invokeErr.Error()}, nil
	}
	if strings.TrimSpace(upid) == "" {
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: "action completed",
			Output:  fmt.Sprintf("%s on %s", actionID, targetLabel),
			Metadata: map[string]string{
				"target":     targetLabel,
				"elapsed_ms": fmt.Sprintf("%d", time.Since(started).Milliseconds()),
			},
		}, nil
	}

	taskStatus, waitErr := c.client.WaitForTask(ctx, node, upid, defaultActionPollInterval, defaultActionWaitTimeout)
	if waitErr != nil {
		return connectorsdk.ActionResult{
			Status:  "failed",
			Message: waitErr.Error(),
			Output:  fmt.Sprintf("%s on %s (upid=%s)", actionID, targetLabel, upid),
			Metadata: map[string]string{
				"target":     targetLabel,
				"upid":       upid,
				"elapsed_ms": fmt.Sprintf("%d", time.Since(started).Milliseconds()),
			},
		}, nil
	}

	exitStatus := strings.TrimSpace(taskStatus.ExitStatus)
	if exitStatus == "" {
		exitStatus = "OK"
	}

	result := connectorsdk.ActionResult{
		Status:  "succeeded",
		Message: "action completed",
		Output:  fmt.Sprintf("%s on %s (upid=%s, exit=%s)", actionID, targetLabel, upid, exitStatus),
		Metadata: map[string]string{
			"target":     targetLabel,
			"upid":       upid,
			"exitstatus": exitStatus,
			"elapsed_ms": fmt.Sprintf("%d", time.Since(started).Milliseconds()),
		},
	}
	if !strings.EqualFold(exitStatus, "OK") {
		result.Status = "failed"
		result.Message = fmt.Sprintf("task finished with exitstatus %s", exitStatus)
	}
	return result, nil
}

func parseComputeTarget(req connectorsdk.ActionRequest, defaultNode string) (string, string, error) {
	node := strings.TrimSpace(req.Params["node"])
	vmid := strings.TrimSpace(req.Params["vmid"])
	target := assetid.NativeCollectorAssetID(req.TargetID)

	if target != "" {
		switch {
		case strings.Contains(target, "/"):
			parts := strings.Split(target, "/")
			if len(parts) == 2 {
				node = strings.TrimSpace(parts[0])
				vmid = strings.TrimSpace(parts[1])
			}
		case strings.HasPrefix(target, "proxmox-vm-"):
			vmid = strings.TrimSpace(strings.TrimPrefix(target, "proxmox-vm-"))
		case strings.HasPrefix(target, "proxmox-ct-"):
			vmid = strings.TrimSpace(strings.TrimPrefix(target, "proxmox-ct-"))
		case strings.HasPrefix(target, "qemu/"):
			vmid = strings.TrimSpace(strings.TrimPrefix(target, "qemu/"))
		case strings.HasPrefix(target, "lxc/"):
			vmid = strings.TrimSpace(strings.TrimPrefix(target, "lxc/"))
		default:
			if _, err := strconv.Atoi(target); err == nil {
				vmid = target
			}
		}
	}

	if node == "" {
		node = strings.TrimSpace(defaultNode)
	}
	if node == "" || vmid == "" {
		return "", "", fmt.Errorf("target must be node/vmid (or provide node + vmid params)")
	}
	parsedVMID, err := strconv.Atoi(vmid)
	if err != nil || parsedVMID <= 0 {
		return "", "", fmt.Errorf("vmid must be a positive integer")
	}

	return node, vmid, nil
}
