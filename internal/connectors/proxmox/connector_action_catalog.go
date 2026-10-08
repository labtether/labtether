package proxmox

import (
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
				{Key: "snapshot_name", Label: "Snapshot Name", Description: "Snapshot label; an automatic LabTether timestamp label is used when omitted."},
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
				{Key: "snapshot_name", Label: "Snapshot Name", Description: "Snapshot label; an automatic LabTether timestamp label is used when omitted."},
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
