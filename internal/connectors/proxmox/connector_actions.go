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
	if err := validateTaskAction(actionID, req.Params); err != nil {
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

	upid, invokeErr := c.executeTaskAction(ctx, actionID, node, vmid, req.Params)
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

func (c *Connector) executeTaskAction(ctx context.Context, actionID, node, vmid string, params map[string]string) (string, error) {
	if err := validateTaskAction(actionID, params); err != nil {
		return "", err
	}
	switch actionID {
	case "vm.start":
		return c.client.StartVM(ctx, node, vmid)
	case "vm.stop":
		return c.client.StopVM(ctx, node, vmid)
	case "vm.shutdown":
		return c.client.ShutdownVM(ctx, node, vmid)
	case "vm.reboot":
		return c.client.RebootVM(ctx, node, vmid)
	case "vm.snapshot":
		name := strings.TrimSpace(params["snapshot_name"])
		if name == "" {
			name = fmt.Sprintf("labtether-%d", time.Now().UTC().Unix())
		}
		return c.client.SnapshotVM(ctx, node, vmid, name)
	case "vm.migrate":
		targetNode := strings.TrimSpace(params["target_node"])
		if targetNode == "" {
			return "", fmt.Errorf("target_node is required")
		}
		return c.client.MigrateVM(ctx, node, vmid, targetNode)
	case "ct.start":
		return c.client.StartCT(ctx, node, vmid)
	case "ct.stop":
		return c.client.StopCT(ctx, node, vmid)
	case "ct.shutdown":
		return c.client.ShutdownCT(ctx, node, vmid)
	case "ct.reboot":
		return c.client.RebootCT(ctx, node, vmid)
	case "ct.snapshot":
		name := strings.TrimSpace(params["snapshot_name"])
		if name == "" {
			name = fmt.Sprintf("labtether-%d", time.Now().UTC().Unix())
		}
		return c.client.SnapshotCT(ctx, node, vmid, name)
	// Phase 2: Extended lifecycle
	case "vm.suspend":
		return c.client.SuspendVM(ctx, node, vmid)
	case "vm.resume":
		return c.client.ResumeVM(ctx, node, vmid)
	case "vm.force_stop":
		return c.client.StopVM(ctx, node, vmid)
	case "ct.force_stop":
		return c.client.StopCT(ctx, node, vmid)
	case "vm.snapshot.delete":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return c.client.DeleteQemuSnapshot(ctx, node, vmid, snapName)
	case "vm.snapshot.rollback":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return c.client.RollbackQemuSnapshot(ctx, node, vmid, snapName)
	case "ct.snapshot.delete":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return c.client.DeleteLXCSnapshot(ctx, node, vmid, snapName)
	case "ct.snapshot.rollback":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return c.client.RollbackLXCSnapshot(ctx, node, vmid, snapName)
	case "ct.migrate":
		targetNode := strings.TrimSpace(params["target_node"])
		if targetNode == "" {
			return "", fmt.Errorf("target_node is required")
		}
		return c.client.MigrateCT(ctx, node, vmid, targetNode)
	// Phase 4: Extended features
	case "vm.backup", "ct.backup":
		return c.client.TriggerBackup(ctx, node, vmid, params["storage"], params["mode"])
	case "vm.clone":
		newIDStr := strings.TrimSpace(params["new_id"])
		if newIDStr == "" {
			return "", fmt.Errorf("new_id is required")
		}
		newID, err := strconv.Atoi(newIDStr)
		if err != nil {
			return "", fmt.Errorf("new_id must be numeric")
		}
		return c.client.CloneVM(ctx, node, vmid, params["new_name"], newID)
	case "ct.clone":
		newIDStr := strings.TrimSpace(params["new_id"])
		if newIDStr == "" {
			return "", fmt.Errorf("new_id is required")
		}
		newID, err := strconv.Atoi(newIDStr)
		if err != nil {
			return "", fmt.Errorf("new_id must be numeric")
		}
		return c.client.CloneCT(ctx, node, vmid, params["new_name"], newID)
	case "vm.clone_from_template":
		// Same as vm.clone — Proxmox clone works identically on templates.
		newIDStr := strings.TrimSpace(params["new_id"])
		if newIDStr == "" {
			return "", fmt.Errorf("new_id is required")
		}
		newID, err := strconv.Atoi(newIDStr)
		if err != nil {
			return "", fmt.Errorf("new_id must be numeric")
		}
		return c.client.CloneVM(ctx, node, vmid, params["new_name"], newID)
	case "ct.clone_from_template":
		// Same as ct.clone — Proxmox clone works identically on templates.
		newIDStr := strings.TrimSpace(params["new_id"])
		if newIDStr == "" {
			return "", fmt.Errorf("new_id is required")
		}
		newID, err := strconv.Atoi(newIDStr)
		if err != nil {
			return "", fmt.Errorf("new_id must be numeric")
		}
		return c.client.CloneCT(ctx, node, vmid, params["new_name"], newID)
	case "vm.disk_resize":
		disk := strings.TrimSpace(params["disk"])
		size := strings.TrimSpace(params["size"])
		if disk == "" || size == "" {
			return "", fmt.Errorf("disk and size are required")
		}
		return "", c.client.ResizeVMDisk(ctx, node, vmid, disk, size)
	default:
		return "", fmt.Errorf("unsupported action")
	}
}

func validateTaskAction(actionID string, params map[string]string) error {
	param := func(key string) string { return strings.TrimSpace(params[key]) }
	require := func(key string) error {
		if param(key) == "" {
			return fmt.Errorf("%s is required", key)
		}
		return nil
	}
	validateNewID := func() error {
		if err := require("new_id"); err != nil {
			return err
		}
		newID, err := strconv.Atoi(param("new_id"))
		if err != nil || newID <= 0 {
			return fmt.Errorf("new_id must be a positive integer")
		}
		return nil
	}

	switch actionID {
	case "vm.start", "vm.stop", "vm.shutdown", "vm.reboot",
		"ct.start", "ct.stop", "ct.shutdown", "ct.reboot",
		"vm.snapshot", "ct.snapshot", "vm.suspend", "vm.resume",
		"vm.force_stop", "ct.force_stop":
		return nil
	case "vm.migrate", "ct.migrate":
		return require("target_node")
	case "vm.snapshot.delete", "vm.snapshot.rollback", "ct.snapshot.delete", "ct.snapshot.rollback":
		return require("snapshot_name")
	case "vm.backup", "ct.backup":
		mode := strings.ToLower(param("mode"))
		if mode != "" && mode != "snapshot" && mode != "suspend" && mode != "stop" {
			return fmt.Errorf("mode must be snapshot, suspend, or stop")
		}
		return nil
	case "vm.clone", "ct.clone":
		return validateNewID()
	case "vm.clone_from_template", "ct.clone_from_template":
		if err := validateNewID(); err != nil {
			return err
		}
		return require("new_name")
	case "vm.disk_resize":
		if err := require("disk"); err != nil {
			return err
		}
		return require("size")
	default:
		return fmt.Errorf("unsupported action")
	}
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
