package proxmox

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func (c *Connector) Actions() []connectorsdk.ActionDescriptor {
	return []connectorsdk.ActionDescriptor{
		{
			ID:             "vm.start",
			Name:           "Start VM",
			Description:    "Start a virtual machine.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "vm.stop",
			Name:           "Stop VM",
			Description:    "Force stop a virtual machine.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "vm.shutdown",
			Name:           "Shutdown VM",
			Description:    "Gracefully shutdown a VM.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "vm.reboot",
			Name:           "Reboot VM",
			Description:    "Reboot a virtual machine.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "vm.snapshot",
			Name:           "Snapshot VM",
			Description:    "Create a VM snapshot.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "snapshot_name", Label: "Snapshot Name", Required: true, Description: "Snapshot label."},
			},
		},
		{
			ID:             "vm.migrate",
			Name:           "Migrate VM",
			Description:    "Migrate VM to a different node.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "target_node", Label: "Target Node", Required: true, Description: "Destination node name."},
			},
		},
		{
			ID:             "ct.start",
			Name:           "Start Container",
			Description:    "Start a container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "ct.stop",
			Name:           "Stop Container",
			Description:    "Force stop a container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "ct.shutdown",
			Name:           "Shutdown Container",
			Description:    "Gracefully shutdown a container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "ct.reboot",
			Name:           "Reboot Container",
			Description:    "Reboot a container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "ct.snapshot",
			Name:           "Snapshot Container",
			Description:    "Create a container snapshot.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "snapshot_name", Label: "Snapshot Name", Required: true, Description: "Snapshot label."},
			},
		},
		// Phase 2: Extended lifecycle
		{
			ID:             "vm.suspend",
			Name:           "Suspend VM",
			Description:    "Suspend a virtual machine to RAM.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "vm.resume",
			Name:           "Resume VM",
			Description:    "Resume a suspended virtual machine.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "vm.force_stop",
			Name:           "Force Stop VM",
			Description:    "Immediately stop a VM (like pulling the power cord).",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "ct.force_stop",
			Name:           "Force Stop Container",
			Description:    "Immediately stop a container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "vm.snapshot.delete",
			Name:           "Delete VM Snapshot",
			Description:    "Delete a VM snapshot.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "snapshot_name", Label: "Snapshot Name", Required: true, Description: "Snapshot to delete."},
			},
		},
		{
			ID:             "vm.snapshot.rollback",
			Name:           "Rollback VM Snapshot",
			Description:    "Rollback a VM to a snapshot.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "snapshot_name", Label: "Snapshot Name", Required: true, Description: "Snapshot to rollback to."},
			},
		},
		{
			ID:             "ct.snapshot.delete",
			Name:           "Delete CT Snapshot",
			Description:    "Delete a container snapshot.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "snapshot_name", Label: "Snapshot Name", Required: true, Description: "Snapshot to delete."},
			},
		},
		{
			ID:             "ct.snapshot.rollback",
			Name:           "Rollback CT Snapshot",
			Description:    "Rollback a container to a snapshot.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "snapshot_name", Label: "Snapshot Name", Required: true, Description: "Snapshot to rollback to."},
			},
		},
		{
			ID:             "ct.migrate",
			Name:           "Migrate Container",
			Description:    "Migrate container to a different node.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "target_node", Label: "Target Node", Required: true, Description: "Destination node name."},
			},
		},
		// Phase 4: Extended features
		{
			ID:             "vm.backup",
			Name:           "Backup VM",
			Description:    "Trigger an on-demand VM backup via vzdump.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "storage", Label: "Storage", Required: false, Description: "Backup storage ID (e.g. local)."},
				{Key: "mode", Label: "Mode", Required: false, Description: "Backup mode: snapshot, suspend, or stop."},
			},
		},
		{
			ID:             "ct.backup",
			Name:           "Backup Container",
			Description:    "Trigger an on-demand container backup via vzdump.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "storage", Label: "Storage", Required: false, Description: "Backup storage ID (e.g. local)."},
				{Key: "mode", Label: "Mode", Required: false, Description: "Backup mode: snapshot, suspend, or stop."},
			},
		},
		{
			ID:             "vm.clone",
			Name:           "Clone VM",
			Description:    "Create a full clone of a VM.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "new_name", Label: "New VM Name", Required: false, Description: "Name for the cloned VM."},
				{Key: "new_id", Label: "New VMID", Required: true, Description: "VMID for the clone."},
			},
		},
		{
			ID:             "ct.clone",
			Name:           "Clone Container",
			Description:    "Create a full clone of a container.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "new_name", Label: "New Hostname", Required: false, Description: "Hostname for the cloned container."},
				{Key: "new_id", Label: "New VMID", Required: true, Description: "VMID for the clone."},
			},
		},
		{
			ID:             "vm.clone_from_template",
			Name:           "Deploy VM from Template",
			Description:    "Clone a template to create a new VM.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "new_name", Label: "New VM Name", Required: true, Description: "Name for the new VM."},
				{Key: "new_id", Label: "New VMID", Required: true, Description: "VMID for the new VM."},
				{Key: "target_node", Label: "Target Node", Required: false, Description: "Target node (empty = same node)."},
			},
		},
		{
			ID:             "ct.clone_from_template",
			Name:           "Deploy CT from Template",
			Description:    "Clone a template to create a new container.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "new_name", Label: "New Hostname", Required: true, Description: "Name for the new container."},
				{Key: "new_id", Label: "New VMID", Required: true, Description: "VMID for the new container."},
				{Key: "target_node", Label: "Target Node", Required: false, Description: "Target node (empty = same node)."},
			},
		},
		{
			ID:             "vm.disk_resize",
			Name:           "Resize VM Disk",
			Description:    "Increase the size of a VM disk.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "disk", Label: "Disk", Required: true, Description: "Disk name (e.g. scsi0, virtio0)."},
				{Key: "size", Label: "Size", Required: true, Description: "New size (e.g. +10G, 50G)."},
			},
		},
	}
}

func (c *Connector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	node, vmid, err := parseComputeTarget(req, c.defaultNode)
	if err != nil {
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

	if c.clientErr != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: c.clientErr.Error()}, nil
	}
	if !c.isConfigured() {
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: "action executed (stub mode)",
			Output:  fmt.Sprintf("%s on %s", actionID, targetLabel),
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
func parseComputeTarget(req connectorsdk.ActionRequest, defaultNode string) (string, string, error) {
	node := strings.TrimSpace(req.Params["node"])
	vmid := strings.TrimSpace(req.Params["vmid"])
	target := strings.TrimSpace(req.TargetID)

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
	if _, err := strconv.Atoi(vmid); err != nil {
		return "", "", fmt.Errorf("vmid must be numeric")
	}

	return node, vmid, nil
}
