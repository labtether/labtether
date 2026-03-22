package proxmox

import (
	"context"
	"fmt"
	neturl "net/url"
	"strconv"
	"strings"
)

func (c *Client) GetVersion(ctx context.Context) (string, error) {
	var payload struct {
		Release string `json:"release"`
		Version string `json:"version"`
	}
	if err := c.getData(ctx, "/api2/json/version", &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Release) != "" {
		return strings.TrimSpace(payload.Release), nil
	}
	return strings.TrimSpace(payload.Version), nil
}

func (c *Client) GetClusterResources(ctx context.Context) ([]Resource, error) {
	var resources []Resource
	if err := c.getData(ctx, "/api2/json/cluster/resources", &resources); err != nil {
		return nil, err
	}
	return resources, nil
}

func (c *Client) ListStorageBackups(ctx context.Context, node, storage string) ([]StorageBackup, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/storage/%s/content?content=backup",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(storage)),
	)
	var backups []StorageBackup
	if err := c.getData(ctx, path, &backups); err != nil {
		return nil, err
	}
	return backups, nil
}

// GetStorageContent returns the content list for a storage pool on a node.
func (c *Client) GetStorageContent(ctx context.Context, node, storage string) ([]StorageContent, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/storage/%s/content",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(storage)),
	)
	var content []StorageContent
	if err := c.getData(ctx, path, &content); err != nil {
		return nil, err
	}
	return content, nil
}

func (c *Client) GetStorageStatus(ctx context.Context, node, storage string) (map[string]any, error) {
	status := map[string]any{}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/storage/%s/status",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(storage)),
	)
	if err := c.getData(ctx, path, &status); err != nil {
		return nil, err
	}
	return status, nil
}

func (c *Client) GetNodeStatus(ctx context.Context, node string) (map[string]any, error) {
	status := map[string]any{}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/status",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	if err := c.getData(ctx, path, &status); err != nil {
		return nil, err
	}
	return status, nil
}

func (c *Client) GetQemuConfig(ctx context.Context, node, vmid string) (map[string]any, error) {
	config := map[string]any{}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/config",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
	)
	if err := c.getData(ctx, path, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func (c *Client) GetLXCConfig(ctx context.Context, node, vmid string) (map[string]any, error) {
	config := map[string]any{}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/config",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
	)
	if err := c.getData(ctx, path, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func (c *Client) ListQemuSnapshots(ctx context.Context, node, vmid string) ([]Snapshot, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/snapshot",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
	)
	snapshots := make([]Snapshot, 0)
	if err := c.getData(ctx, path, &snapshots); err != nil {
		return nil, err
	}
	return snapshots, nil
}

func (c *Client) ListLXCSnapshots(ctx context.Context, node, vmid string) ([]Snapshot, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/snapshot",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
	)
	snapshots := make([]Snapshot, 0)
	if err := c.getData(ctx, path, &snapshots); err != nil {
		return nil, err
	}
	return snapshots, nil
}

func (c *Client) ListClusterTasks(ctx context.Context, node, vmid string, limit int) ([]Task, error) {
	trimmedNode := strings.TrimSpace(node)

	// /cluster/tasks does not accept query parameters. When we have a node,
	// use /nodes/{node}/tasks which supports limit and vmid filtering.
	var path string
	if trimmedNode != "" {
		query := neturl.Values{}
		if trimmedVMID := strings.TrimSpace(vmid); trimmedVMID != "" {
			query.Set("vmid", trimmedVMID)
		}
		if limit > 0 {
			query.Set("limit", strconv.Itoa(limit))
		}
		path = fmt.Sprintf("/api2/json/nodes/%s/tasks", neturl.PathEscape(trimmedNode))
		if encoded := query.Encode(); encoded != "" {
			path += "?" + encoded
		}
	} else {
		path = "/api2/json/cluster/tasks"
	}

	tasks := make([]Task, 0)
	if err := c.getData(ctx, path, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (c *Client) ListHAResources(ctx context.Context) ([]HAResource, error) {
	resources := make([]HAResource, 0)
	if err := c.getData(ctx, "/api2/json/cluster/ha/resources", &resources); err != nil {
		return nil, err
	}
	return resources, nil
}

// GetClusterFirewallRules returns cluster-level firewall rules.
func (c *Client) GetClusterFirewallRules(ctx context.Context) ([]FirewallRule, error) {
	var rules []FirewallRule
	if err := c.getData(ctx, "/api2/json/cluster/firewall/rules", &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// GetBackupSchedules returns all configured cluster-level backup schedules.
func (c *Client) GetBackupSchedules(ctx context.Context) ([]BackupSchedule, error) {
	var schedules []BackupSchedule
	if err := c.getData(ctx, "/api2/json/cluster/backup", &schedules); err != nil {
		return nil, err
	}
	return schedules, nil
}

// GetNodeFirewallRules returns host-level firewall rules for a node.
func (c *Client) GetNodeFirewallRules(ctx context.Context, node string) ([]FirewallRule, error) {
	var rules []FirewallRule
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/firewall/rules",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	if err := c.getData(ctx, path, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// GetVMFirewallRules returns VM or CT-level firewall rules.
func (c *Client) GetVMFirewallRules(ctx context.Context, node, vmid, kind string) ([]FirewallRule, error) {
	var rules []FirewallRule
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/%s/%s/firewall/rules",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(kind)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
	)
	if err := c.getData(ctx, path, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// CreateNodeFirewallRule creates a new host-level firewall rule for a node.
func (c *Client) CreateNodeFirewallRule(ctx context.Context, node string, rule FirewallRule) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/firewall/rules",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	values := firewallRuleToValues(rule)
	_, err := c.postTask(ctx, path, values)
	return err
}

// UpdateNodeFirewallRule updates an existing host-level firewall rule by position.
func (c *Client) UpdateNodeFirewallRule(ctx context.Context, node string, pos int, rule FirewallRule) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/firewall/rules/%d",
		neturl.PathEscape(strings.TrimSpace(node)),
		pos,
	)
	values := firewallRuleToValues(rule)
	_, err := c.requestRaw(ctx, "PUT", path, values)
	return err
}

// DeleteNodeFirewallRule removes a host-level firewall rule by position.
func (c *Client) DeleteNodeFirewallRule(ctx context.Context, node string, pos int) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/firewall/rules/%d",
		neturl.PathEscape(strings.TrimSpace(node)),
		pos,
	)
	_, err := c.requestRaw(ctx, "DELETE", path, nil)
	return err
}

// CreateVMFirewallRule creates a new VM or CT-level firewall rule.
// kind must be "qemu" or "lxc".
func (c *Client) CreateVMFirewallRule(ctx context.Context, node, vmid, kind string, rule FirewallRule) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/%s/%s/firewall/rules",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(kind)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
	)
	values := firewallRuleToValues(rule)
	_, err := c.postTask(ctx, path, values)
	return err
}

// UpdateVMFirewallRule updates a VM or CT-level firewall rule by position.
func (c *Client) UpdateVMFirewallRule(ctx context.Context, node, vmid, kind string, pos int, rule FirewallRule) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/%s/%s/firewall/rules/%d",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(kind)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
		pos,
	)
	values := firewallRuleToValues(rule)
	_, err := c.requestRaw(ctx, "PUT", path, values)
	return err
}

// DeleteVMFirewallRule removes a VM or CT-level firewall rule by position.
func (c *Client) DeleteVMFirewallRule(ctx context.Context, node, vmid, kind string, pos int) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/%s/%s/firewall/rules/%d",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(kind)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
		pos,
	)
	_, err := c.requestRaw(ctx, "DELETE", path, nil)
	return err
}

// CreateNodeNetwork creates a new network interface on a node.
func (c *Client) CreateNodeNetwork(ctx context.Context, node string, config map[string]any) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/network",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	_, err := c.postTask(ctx, path, mapToValues(config))
	return err
}

// UpdateNodeNetwork updates an existing network interface on a node.
func (c *Client) UpdateNodeNetwork(ctx context.Context, node, iface string, config map[string]any) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/network/%s",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(iface)),
	)
	values := mapToValues(config)
	_, err := c.requestRaw(ctx, "PUT", path, values)
	return err
}

// ApplyNodeNetworkChanges commits pending network config changes on a node.
func (c *Client) ApplyNodeNetworkChanges(ctx context.Context, node string) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/network",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	_, err := c.requestRaw(ctx, "PUT", path, neturl.Values{})
	return err
}

// ListReplication returns the replication job list for a node.
func (c *Client) ListReplication(ctx context.Context, node string) ([]map[string]any, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/replication",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	var result []map[string]any
	if err := c.getData(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RunReplication triggers a replication job immediately and returns the task UPID.
func (c *Client) RunReplication(ctx context.Context, node, id string) (string, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/replication/%s/schedule_now",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(id)),
	)
	return c.postTask(ctx, path, neturl.Values{})
}

// ListUpdates returns available package updates for a node.
func (c *Client) ListUpdates(ctx context.Context, node string) ([]map[string]any, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/apt/update",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	var result []map[string]any
	if err := c.getData(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RefreshUpdates refreshes the apt package cache on a node and returns the task UPID.
func (c *Client) RefreshUpdates(ctx context.Context, node string) (string, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/apt/update",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	return c.postTask(ctx, path, neturl.Values{})
}

// ListCertificates returns TLS certificate info for a node.
func (c *Client) ListCertificates(ctx context.Context, node string) ([]map[string]any, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/certificates/info",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	var result []map[string]any
	if err := c.getData(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RenewACMECert renews the ACME certificate on a node and returns the task UPID.
func (c *Client) RenewACMECert(ctx context.Context, node string) (string, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/certificates/acme/certificate",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	return c.postTask(ctx, path, neturl.Values{})
}

// UpdateHAResource updates HA resource configuration by SID.
func (c *Client) UpdateHAResource(ctx context.Context, sid string, config map[string]any) error {
	path := fmt.Sprintf(
		"/api2/json/cluster/ha/resources/%s",
		neturl.PathEscape(strings.TrimSpace(sid)),
	)
	values := mapToValues(config)
	_, err := c.requestRaw(ctx, "PUT", path, values)
	return err
}

// DownloadStorageURL downloads a file from a remote URL into storage and returns the task UPID.
func (c *Client) DownloadStorageURL(ctx context.Context, node, storage, filename, url string) (string, error) {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/storage/%s/download-url",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(storage)),
	)
	values := neturl.Values{}
	values.Set("filename", strings.TrimSpace(filename))
	values.Set("url", strings.TrimSpace(url))
	return c.postTask(ctx, path, values)
}

// DeleteStorageContent removes a volume from storage.
func (c *Client) DeleteStorageContent(ctx context.Context, node, storage, volid string) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/storage/%s/content/%s",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(storage)),
		neturl.PathEscape(strings.TrimSpace(volid)),
	)
	_, err := c.requestRaw(ctx, "DELETE", path, nil)
	return err
}

// ListCephPools returns the list of Ceph pools from cluster-level endpoint.
func (c *Client) ListCephPools(ctx context.Context) ([]map[string]any, error) {
	var result []map[string]any
	if err := c.getData(ctx, "/api2/json/cluster/ceph/pools", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SetCephOSDState sets the in/out state of a Ceph OSD on a node.
// state must be "in" or "out".
func (c *Client) SetCephOSDState(ctx context.Context, node string, osdID int, state string) error {
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/ceph/osd/%d/%s",
		neturl.PathEscape(strings.TrimSpace(node)),
		osdID,
		neturl.PathEscape(strings.TrimSpace(state)),
	)
	_, err := c.postTask(ctx, path, neturl.Values{})
	return err
}

// firewallRuleToValues converts a FirewallRule to URL form values for POST/PUT.
func firewallRuleToValues(rule FirewallRule) neturl.Values {
	v := neturl.Values{}
	if rule.Type != "" {
		v.Set("type", rule.Type)
	}
	if rule.Action != "" {
		v.Set("action", rule.Action)
	}
	if rule.Source != "" {
		v.Set("source", rule.Source)
	}
	if rule.Dest != "" {
		v.Set("dest", rule.Dest)
	}
	if rule.Proto != "" {
		v.Set("proto", rule.Proto)
	}
	if rule.Dport != "" {
		v.Set("dport", rule.Dport)
	}
	if rule.Sport != "" {
		v.Set("sport", rule.Sport)
	}
	if rule.Comment != "" {
		v.Set("comment", rule.Comment)
	}
	if rule.Macro != "" {
		v.Set("macro", rule.Macro)
	}
	if rule.IFace != "" {
		v.Set("iface", rule.IFace)
	}
	v.Set("enable", strconv.Itoa(rule.Enable))
	return v
}

// mapToValues converts a map[string]any to URL form values, using string representation for all values.
func mapToValues(m map[string]any) neturl.Values {
	v := neturl.Values{}
	for key, val := range m {
		if val == nil {
			continue
		}
		v.Set(key, fmt.Sprintf("%v", val))
	}
	return v
}
