package proxmox

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"
)

func (c *Client) GetTaskStatus(ctx context.Context, node, upid string) (TaskStatus, error) {
	var status TaskStatus
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/tasks/%s/status",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(upid)),
	)
	if err := c.getData(ctx, path, &status); err != nil {
		return TaskStatus{}, err
	}
	return status, nil
}

func (c *Client) WaitForTask(ctx context.Context, node, upid string, pollInterval, timeout time.Duration) (TaskStatus, error) {
	if strings.TrimSpace(upid) == "" {
		return TaskStatus{Status: "stopped", ExitStatus: "OK"}, nil
	}

	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		status, err := c.GetTaskStatus(waitCtx, node, upid)
		if err != nil {
			return TaskStatus{}, err
		}
		if !strings.EqualFold(strings.TrimSpace(status.Status), "running") {
			return status, nil
		}

		select {
		case <-waitCtx.Done():
			return TaskStatus{}, fmt.Errorf("task %s timed out: %w", upid, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (c *Client) StartVM(ctx context.Context, node, vmid string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/status/start",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) StopVM(ctx context.Context, node, vmid string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/status/stop",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) ShutdownVM(ctx context.Context, node, vmid string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/status/shutdown",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) RebootVM(ctx context.Context, node, vmid string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/status/reboot",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) SnapshotVM(ctx context.Context, node, vmid, snapshotName string) (string, error) {
	values := neturl.Values{}
	values.Set("snapname", strings.TrimSpace(snapshotName))
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/snapshot",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), values)
}

func (c *Client) MigrateVM(ctx context.Context, node, vmid, targetNode string) (string, error) {
	values := neturl.Values{}
	values.Set("target", strings.TrimSpace(targetNode))
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/migrate",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), values)
}

func (c *Client) StartCT(ctx context.Context, node, vmid string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/status/start",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) StopCT(ctx context.Context, node, vmid string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/status/stop",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) ShutdownCT(ctx context.Context, node, vmid string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/status/shutdown",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) RebootCT(ctx context.Context, node, vmid string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/status/reboot",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) SnapshotCT(ctx context.Context, node, vmid, snapshotName string) (string, error) {
	values := neturl.Values{}
	values.Set("snapname", strings.TrimSpace(snapshotName))
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/snapshot",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), values)
}

func (c *Client) SuspendVM(ctx context.Context, node, vmid string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/status/suspend",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) ResumeVM(ctx context.Context, node, vmid string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/status/resume",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) DeleteQemuSnapshot(ctx context.Context, node, vmid, snapname string) (string, error) {
	return c.deleteTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/snapshot/%s",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
		neturl.PathEscape(strings.TrimSpace(snapname)),
	))
}

func (c *Client) RollbackQemuSnapshot(ctx context.Context, node, vmid, snapname string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/snapshot/%s/rollback",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
		neturl.PathEscape(strings.TrimSpace(snapname)),
	), neturl.Values{})
}

func (c *Client) DeleteLXCSnapshot(ctx context.Context, node, vmid, snapname string) (string, error) {
	return c.deleteTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/snapshot/%s",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
		neturl.PathEscape(strings.TrimSpace(snapname)),
	))
}

func (c *Client) RollbackLXCSnapshot(ctx context.Context, node, vmid, snapname string) (string, error) {
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/snapshot/%s/rollback",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
		neturl.PathEscape(strings.TrimSpace(snapname)),
	), neturl.Values{})
}

func (c *Client) MigrateCT(ctx context.Context, node, vmid, targetNode string) (string, error) {
	values := neturl.Values{}
	values.Set("target", strings.TrimSpace(targetNode))
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/migrate",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), values)
}

// TaskLogLine represents a single line from a task log.
type TaskLogLine struct {
	LineNo int    `json:"n"`
	Text   string `json:"t"`
}

func (c *Client) GetTaskLog(ctx context.Context, node, upid string, limit int) (string, error) {
	if limit <= 0 {
		limit = 500
	}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/tasks/%s/log?limit=%d",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(upid)),
		limit,
	)
	var lines []TaskLogLine
	if err := c.getData(ctx, path, &lines); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(line.Text)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func (c *Client) StopTask(ctx context.Context, node, upid string) error {
	_, err := c.requestRaw(ctx, http.MethodDelete, fmt.Sprintf(
		"/api2/json/nodes/%s/tasks/%s",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(upid)),
	), nil)
	return err
}

func (c *Client) TriggerBackup(ctx context.Context, node, vmid, storage, mode string) (string, error) {
	values := neturl.Values{}
	values.Set("vmid", strings.TrimSpace(vmid))
	if storage = strings.TrimSpace(storage); storage != "" {
		values.Set("storage", storage)
	}
	if mode = strings.TrimSpace(mode); mode != "" {
		values.Set("mode", mode)
	}
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/vzdump",
		neturl.PathEscape(strings.TrimSpace(node)),
	), values)
}

func (c *Client) CloneVM(ctx context.Context, node, vmid, newName string, newID int) (string, error) {
	values := neturl.Values{}
	values.Set("newid", strconv.Itoa(newID))
	if newName = strings.TrimSpace(newName); newName != "" {
		values.Set("name", newName)
	}
	values.Set("full", "1")
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/clone",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), values)
}

func (c *Client) CloneCT(ctx context.Context, node, vmid, newName string, newID int) (string, error) {
	values := neturl.Values{}
	values.Set("newid", strconv.Itoa(newID))
	if newName = strings.TrimSpace(newName); newName != "" {
		values.Set("hostname", newName)
	}
	values.Set("full", "1")
	return c.postTask(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/clone",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), values)
}

func (c *Client) ResizeVMDisk(ctx context.Context, node, vmid, disk, size string) error {
	values := neturl.Values{}
	values.Set("disk", strings.TrimSpace(disk))
	values.Set("size", strings.TrimSpace(size))
	_, err := c.requestRaw(ctx, http.MethodPut, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/resize",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), values)
	return err
}
