package proxmox

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ExecuteTaskAction validates and dispatches provider actions for both Hub and connector callers.
func (c *Client) ExecuteTaskAction(ctx context.Context, actionID, node, vmid string, params map[string]string) (string, error) {
	if err := ValidateTaskAction(actionID, params); err != nil {
		return "", err
	}
	switch actionID {
	case "vm.start":
		return c.StartVM(ctx, node, vmid)
	case "vm.stop":
		return c.StopVM(ctx, node, vmid)
	case "vm.shutdown":
		return c.ShutdownVM(ctx, node, vmid)
	case "vm.reboot":
		return c.RebootVM(ctx, node, vmid)
	case "vm.snapshot":
		name := strings.TrimSpace(params["snapshot_name"])
		if name == "" {
			name = fmt.Sprintf("labtether-%d", time.Now().UTC().Unix())
		}
		return c.SnapshotVM(ctx, node, vmid, name)
	case "vm.migrate":
		targetNode := strings.TrimSpace(params["target_node"])
		if targetNode == "" {
			return "", fmt.Errorf("target_node is required")
		}
		return c.MigrateVM(ctx, node, vmid, targetNode)
	case "ct.start":
		return c.StartCT(ctx, node, vmid)
	case "ct.stop":
		return c.StopCT(ctx, node, vmid)
	case "ct.shutdown":
		return c.ShutdownCT(ctx, node, vmid)
	case "ct.reboot":
		return c.RebootCT(ctx, node, vmid)
	case "ct.snapshot":
		name := strings.TrimSpace(params["snapshot_name"])
		if name == "" {
			name = fmt.Sprintf("labtether-%d", time.Now().UTC().Unix())
		}
		return c.SnapshotCT(ctx, node, vmid, name)
	// Phase 2: Extended lifecycle
	case "vm.suspend":
		return c.SuspendVM(ctx, node, vmid)
	case "vm.resume":
		return c.ResumeVM(ctx, node, vmid)
	case "vm.force_stop":
		return c.StopVM(ctx, node, vmid)
	case "ct.force_stop":
		return c.StopCT(ctx, node, vmid)
	case "vm.snapshot.delete":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return c.DeleteQemuSnapshot(ctx, node, vmid, snapName)
	case "vm.snapshot.rollback":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return c.RollbackQemuSnapshot(ctx, node, vmid, snapName)
	case "ct.snapshot.delete":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return c.DeleteLXCSnapshot(ctx, node, vmid, snapName)
	case "ct.snapshot.rollback":
		snapName := strings.TrimSpace(params["snapshot_name"])
		if snapName == "" {
			return "", fmt.Errorf("snapshot_name is required")
		}
		return c.RollbackLXCSnapshot(ctx, node, vmid, snapName)
	case "ct.migrate":
		targetNode := strings.TrimSpace(params["target_node"])
		if targetNode == "" {
			return "", fmt.Errorf("target_node is required")
		}
		return c.MigrateCT(ctx, node, vmid, targetNode)
	// Phase 4: Extended features
	case "vm.backup", "ct.backup":
		return c.TriggerBackup(ctx, node, vmid, params["storage"], params["mode"])
	case "vm.clone", "vm.clone_from_template":
		newID, _ := strconv.Atoi(strings.TrimSpace(params["new_id"])) // Validated above.
		return c.CloneVM(ctx, node, vmid, params["new_name"], newID, ActionTargetNode(actionID, params))
	case "ct.clone", "ct.clone_from_template":
		newID, _ := strconv.Atoi(strings.TrimSpace(params["new_id"])) // Validated above.
		return c.CloneCT(ctx, node, vmid, params["new_name"], newID, ActionTargetNode(actionID, params))
	case "vm.disk_resize":
		disk := strings.TrimSpace(params["disk"])
		size := strings.TrimSpace(params["size"])
		if disk == "" || size == "" {
			return "", fmt.Errorf("disk and size are required")
		}
		return "", c.ResizeVMDisk(ctx, node, vmid, disk, size)
	default:
		return "", fmt.Errorf("unsupported action")
	}
}

// ValidateTaskAction also validates dry runs before a provider runtime is loaded.
func ValidateTaskAction(actionID string, params map[string]string) error {
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

// ActionTargetNode names the secondary node actually used by provider dispatch.
// Ordinary clones have no destination parameter and stay on the source node.
func ActionTargetNode(actionID string, params map[string]string) string {
	switch actionID {
	case "vm.migrate", "ct.migrate", "vm.clone_from_template", "ct.clone_from_template":
		return strings.TrimSpace(params["target_node"])
	default:
		return ""
	}
}
