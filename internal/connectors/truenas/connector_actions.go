package truenas

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func (c *Connector) Actions() []connectorsdk.ActionDescriptor {
	return []connectorsdk.ActionDescriptor{
		{
			ID:             "pool.scrub",
			Name:           "Run Pool Scrub",
			Description:    "Start a ZFS scrub on the specified storage pool.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{
					Key:         "pool_name",
					Label:       "Pool Name",
					Required:    true,
					Description: "Name of the pool to scrub (e.g. tank).",
				},
			},
		},
		{
			ID:             "snapshot.create",
			Name:           "Create Snapshot",
			Description:    "Create a ZFS snapshot of the specified dataset.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{
					Key:         "dataset",
					Label:       "Dataset",
					Required:    true,
					Description: "Dataset path (e.g. tank/data).",
				},
				{
					Key:         "name",
					Label:       "Snapshot Name",
					Required:    true,
					Description: "Snapshot name (e.g. auto-2025-01-01).",
				},
			},
		},
		{
			ID:             "snapshot.delete",
			Name:           "Delete Snapshot",
			Description:    "Delete a ZFS snapshot by its full identifier.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{
					Key:         "snapshot_id",
					Label:       "Snapshot ID",
					Required:    true,
					Description: "Full snapshot identifier (e.g. tank/data@auto-2025-01-01).",
				},
			},
		},
		{
			ID:             "snapshot.rollback",
			Name:           "Rollback Snapshot",
			Description:    "Roll back a dataset to a previous ZFS snapshot.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{
					Key:         "snapshot_id",
					Label:       "Snapshot ID",
					Required:    true,
					Description: "Full snapshot identifier to roll back to.",
				},
			},
		},
		{
			ID:             "service.restart",
			Name:           "Restart Service",
			Description:    "Restart a TrueNAS service (e.g. smb, nfs, ssh).",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{
					Key:         "service",
					Label:       "Service Name",
					Required:    true,
					Description: "Service name as reported by TrueNAS (e.g. smb, nfs, ssh).",
				},
			},
		},
		{
			ID:             "smart.test",
			Name:           "Run SMART Test",
			Description:    "Run a SMART test on the specified disk.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{
					Key:         "disk",
					Label:       "Disk Identifier",
					Required:    true,
					Description: "Disk identifier (e.g. sda).",
				},
				{
					Key:         "type",
					Label:       "Test Type",
					Required:    false,
					Description: "SMART test type: SHORT, LONG, CONVEYANCE. Defaults to SHORT.",
				},
			},
		},
		{
			ID:             "vm.start",
			Name:           "Start VM",
			Description:    "Start a virtual machine by its numeric ID. (TrueNAS SCALE only.)",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{
					Key:         "vm_id",
					Label:       "VM ID",
					Required:    true,
					Description: "Numeric VM identifier.",
				},
			},
		},
		{
			ID:             "vm.stop",
			Name:           "Stop VM",
			Description:    "Stop a virtual machine by its numeric ID. (TrueNAS SCALE only.)",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{
					Key:         "vm_id",
					Label:       "VM ID",
					Required:    true,
					Description: "Numeric VM identifier.",
				},
			},
		},
		{
			ID:             "system.reboot",
			Name:           "Reboot NAS",
			Description:    "Reboot the TrueNAS system.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters:     nil,
		},
		{
			ID:             "service.start",
			Name:           "Start Service",
			Description:    "Start a TrueNAS service.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "service", Label: "Service Name", Required: true, Description: "Service name (e.g. cifs, ssh, nfs)."},
			},
		},
		{
			ID:             "service.stop",
			Name:           "Stop Service",
			Description:    "Stop a TrueNAS service.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "service", Label: "Service Name", Required: true, Description: "Service name (e.g. cifs, ssh, nfs)."},
			},
		},
		{
			ID:             "app.start",
			Name:           "Start App",
			Description:    "Start a TrueNAS Docker application.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "app_name", Label: "App Name", Required: true, Description: "Application name."},
			},
		},
		{
			ID:             "app.stop",
			Name:           "Stop App",
			Description:    "Stop a TrueNAS Docker application.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "app_name", Label: "App Name", Required: true, Description: "Application name."},
			},
		},
		{
			ID:             "app.restart",
			Name:           "Restart App",
			Description:    "Restart a TrueNAS Docker application.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "app_name", Label: "App Name", Required: true, Description: "Application name."},
			},
		},
	}
}

// ExecuteAction dispatches the requested action to the TrueNAS JSON-RPC API.
// All actions support dry run — when DryRun is true no API call is made.
func (c *Connector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	if !c.isConfigured() {
		return connectorsdk.ActionResult{
			Status:  "failed",
			Message: "truenas connector not configured (missing base URL or API key)",
		}, nil
	}

	switch actionID {
	case "pool.scrub":
		poolName := paramOrTarget(req, "pool_name")
		if poolName == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "pool_name is required"}, nil
		}
		if req.DryRun {
			return connectorsdk.ActionResult{
				Status:  "succeeded",
				Message: "dry-run: pool scrub would be started",
				Output:  fmt.Sprintf("would run pool.scrub.run on pool %q", poolName),
			}, nil
		}
		if err := c.client.Call(ctx, "pool.scrub.run", []any{poolName}, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: fmt.Sprintf("scrub started on pool %q", poolName),
		}, nil

	case "snapshot.create":
		dataset := strings.TrimSpace(req.Params["dataset"])
		snapName := strings.TrimSpace(req.Params["name"])
		if dataset == "" || snapName == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "dataset and name are required"}, nil
		}
		if req.DryRun {
			return connectorsdk.ActionResult{
				Status:  "succeeded",
				Message: "dry-run: snapshot would be created",
				Output:  fmt.Sprintf("would create snapshot %s@%s", dataset, snapName),
			}, nil
		}
		payload := []any{map[string]any{"dataset": dataset, "name": snapName}}
		if err := c.client.Call(ctx, "zfs.snapshot.create", payload, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: fmt.Sprintf("snapshot %s@%s created", dataset, snapName),
		}, nil

	case "snapshot.delete":
		snapshotID := paramOrTarget(req, "snapshot_id")
		if snapshotID == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "snapshot_id is required"}, nil
		}
		if req.DryRun {
			return connectorsdk.ActionResult{
				Status:  "succeeded",
				Message: "dry-run: snapshot would be deleted",
				Output:  fmt.Sprintf("would delete snapshot %q", snapshotID),
			}, nil
		}
		if err := c.client.Call(ctx, "zfs.snapshot.delete", []any{snapshotID}, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: fmt.Sprintf("snapshot %q deleted", snapshotID),
		}, nil

	case "snapshot.rollback":
		snapshotID := paramOrTarget(req, "snapshot_id")
		if snapshotID == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "snapshot_id is required"}, nil
		}
		if req.DryRun {
			return connectorsdk.ActionResult{
				Status:  "succeeded",
				Message: "dry-run: snapshot rollback would be executed",
				Output:  fmt.Sprintf("would roll back to snapshot %q", snapshotID),
			}, nil
		}
		if err := c.client.Call(ctx, "zfs.snapshot.rollback", []any{snapshotID}, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: fmt.Sprintf("dataset rolled back to snapshot %q", snapshotID),
		}, nil

	case "service.restart":
		serviceName := paramOrTarget(req, "service")
		if serviceName == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "service is required"}, nil
		}
		if req.DryRun {
			return connectorsdk.ActionResult{
				Status:  "succeeded",
				Message: "dry-run: service would be restarted",
				Output:  fmt.Sprintf("would restart service %q", serviceName),
			}, nil
		}
		if err := c.client.Call(ctx, "service.restart", []any{serviceName}, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: fmt.Sprintf("service %q restarted", serviceName),
		}, nil

	case "smart.test":
		disk := strings.TrimSpace(req.Params["disk"])
		if disk == "" {
			disk = strings.TrimSpace(req.TargetID)
		}
		if disk == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "disk is required"}, nil
		}
		testType := strings.ToUpper(strings.TrimSpace(req.Params["type"]))
		if testType == "" {
			testType = "SHORT"
		}
		if req.DryRun {
			return connectorsdk.ActionResult{
				Status:  "succeeded",
				Message: "dry-run: SMART test would be started",
				Output:  fmt.Sprintf("would run %s SMART test on disk %q", testType, disk),
			}, nil
		}
		payload := []any{[]any{map[string]any{"identifier": disk, "type": testType}}}
		if err := c.client.Call(ctx, "smart.test.manual_test", payload, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: fmt.Sprintf("%s SMART test started on disk %q", testType, disk),
		}, nil

	case "vm.start":
		vmIDStr := paramOrTarget(req, "vm_id")
		if vmIDStr == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "vm_id is required"}, nil
		}
		vmID, err := strconv.Atoi(vmIDStr)
		if err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: fmt.Sprintf("vm_id must be an integer: %s", vmIDStr)}, nil
		}
		if req.DryRun {
			return connectorsdk.ActionResult{
				Status:  "succeeded",
				Message: "dry-run: VM would be started",
				Output:  fmt.Sprintf("would start VM id=%d", vmID),
			}, nil
		}
		if err := c.client.Call(ctx, "vm.start", []any{vmID}, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: fmt.Sprintf("VM id=%d started", vmID),
		}, nil

	case "vm.stop":
		vmIDStr := paramOrTarget(req, "vm_id")
		if vmIDStr == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "vm_id is required"}, nil
		}
		vmID, err := strconv.Atoi(vmIDStr)
		if err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: fmt.Sprintf("vm_id must be an integer: %s", vmIDStr)}, nil
		}
		if req.DryRun {
			return connectorsdk.ActionResult{
				Status:  "succeeded",
				Message: "dry-run: VM would be stopped",
				Output:  fmt.Sprintf("would stop VM id=%d", vmID),
			}, nil
		}
		if err := c.client.Call(ctx, "vm.stop", []any{vmID}, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: fmt.Sprintf("VM id=%d stopped", vmID),
		}, nil

	case "system.reboot":
		if req.DryRun {
			return connectorsdk.ActionResult{
				Status:  "succeeded",
				Message: "dry-run: system reboot would be issued",
				Output:  "would call system.reboot",
			}, nil
		}
		if err := c.client.Call(ctx, "system.reboot", nil, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: "system reboot initiated",
		}, nil

	case "service.start":
		serviceName := paramOrTarget(req, "service")
		if serviceName == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "service is required"}, nil
		}
		if req.DryRun {
			return connectorsdk.ActionResult{Status: "succeeded", Message: "dry-run: service would be started", Output: fmt.Sprintf("would start service %q", serviceName)}, nil
		}
		if err := c.client.Call(ctx, "service.start", []any{serviceName}, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{Status: "succeeded", Message: fmt.Sprintf("service %q started", serviceName)}, nil

	case "service.stop":
		serviceName := paramOrTarget(req, "service")
		if serviceName == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "service is required"}, nil
		}
		if req.DryRun {
			return connectorsdk.ActionResult{Status: "succeeded", Message: "dry-run: service would be stopped", Output: fmt.Sprintf("would stop service %q", serviceName)}, nil
		}
		if err := c.client.Call(ctx, "service.stop", []any{serviceName}, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{Status: "succeeded", Message: fmt.Sprintf("service %q stopped", serviceName)}, nil

	case "app.start", "app.stop", "app.restart":
		appName := paramOrTarget(req, "app_name")
		if appName == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "app_name is required"}, nil
		}
		verb := strings.TrimPrefix(actionID, "app.")
		if req.DryRun {
			return connectorsdk.ActionResult{Status: "succeeded", Message: fmt.Sprintf("dry-run: app would be %sed", verb), Output: fmt.Sprintf("would %s app %q", verb, appName)}, nil
		}
		if err := c.client.Call(ctx, "app."+verb, []any{appName}, nil); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{Status: "succeeded", Message: fmt.Sprintf("app %q %sed", appName, verb)}, nil

	default:
		return connectorsdk.ActionResult{Status: "failed", Message: fmt.Sprintf("unsupported action: %s", actionID)}, nil
	}
}

// --- utility functions ---

// anyToString converts an arbitrary JSON-decoded value to its string representation.
