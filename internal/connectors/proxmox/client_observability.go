package proxmox

import (
	"context"
	"fmt"
	neturl "net/url"
	"strconv"
	"strings"
)

// ClusterStatusEntry represents a node/cluster entry from /cluster/status.
type ClusterStatusEntry struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	NodeID  int    `json:"nodeid"`
	IP      string `json:"ip"`
	Online  int    `json:"online"`
	Local   int    `json:"local"`
	Level   string `json:"level"`
	Quorate int    `json:"quorate"`
	Version int    `json:"version"`
	Nodes   int    `json:"nodes"`
}

func (c *Client) GetClusterStatus(ctx context.Context) ([]ClusterStatusEntry, error) {
	var entries []ClusterStatusEntry
	if err := c.getData(ctx, "/api2/json/cluster/status", &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// CephStatus represents the overall Ceph cluster status.
type CephStatus struct {
	Health struct {
		Status string `json:"status"` // HEALTH_OK, HEALTH_WARN, HEALTH_ERR
	} `json:"health"`
	PGMap struct {
		PGsByState []struct {
			StateName string `json:"state_name"`
			Count     int    `json:"count"`
		} `json:"pgs_by_state"`
		DataBytes  int64 `json:"data_bytes"`
		BytesUsed  int64 `json:"bytes_used"`
		BytesAvail int64 `json:"bytes_avail"`
		BytesTotal int64 `json:"bytes_total"`
	} `json:"pgmap"`
	MonMap struct {
		Mons []struct {
			Name string `json:"name"`
			Rank int    `json:"rank"`
		} `json:"mons"`
	} `json:"monmap"`
}

// CephOSD represents a single Ceph OSD (Object Storage Daemon).
type CephOSD struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Host        string  `json:"host,omitempty"`
	Status      string  `json:"status"` // up, down
	CrushWeight float64 `json:"crush_weight,omitempty"`
	DeviceClass string  `json:"device_class,omitempty"`
}

// ZFSPool represents a ZFS pool on a Proxmox node.
type ZFSPool struct {
	Name   string  `json:"name"`
	Size   int64   `json:"size"`
	Free   int64   `json:"free"`
	Alloc  int64   `json:"alloc"`
	Frag   int     `json:"frag,omitempty"`
	Health string  `json:"health"` // ONLINE, DEGRADED, FAULTED
	Dedup  float64 `json:"dedup,omitempty"`
}

// GetCephStatus returns the overall Ceph cluster status.
func (c *Client) GetCephStatus(ctx context.Context) (*CephStatus, error) {
	var status CephStatus
	if err := c.getData(ctx, "/api2/json/cluster/ceph/status", &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// GetCephOSDs returns the list of Ceph OSDs.
func (c *Client) GetCephOSDs(ctx context.Context) ([]CephOSD, error) {
	var osds []CephOSD
	if err := c.getData(ctx, "/api2/json/cluster/ceph/osd", &osds); err != nil {
		return nil, err
	}
	return osds, nil
}

// GetNodeZFSPools returns ZFS pools for a specific node.
func (c *Client) GetNodeZFSPools(ctx context.Context, node string) ([]ZFSPool, error) {
	var pools []ZFSPool
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/disks/zfs",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	if err := c.getData(ctx, path, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

// RRDDataPoint represents a single time-series point from rrddata.
type RRDDataPoint struct {
	Time    float64  `json:"time"`
	CPU     *float64 `json:"cpu"`
	MaxCPU  *float64 `json:"maxcpu"`
	MemUsed *float64 `json:"memused"`
	MemMax  *float64 `json:"maxmem"`
	NetIn   *float64 `json:"netin"`
	NetOut  *float64 `json:"netout"`
	DiskIn  *float64 `json:"diskread"`
	DiskOut *float64 `json:"diskwrite"`
}

func (c *Client) GetNodeRRDData(ctx context.Context, node, timeframe string) ([]RRDDataPoint, error) {
	if timeframe = strings.TrimSpace(timeframe); timeframe == "" {
		timeframe = "hour"
	}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/rrddata?timeframe=%s",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.QueryEscape(timeframe),
	)
	var points []RRDDataPoint
	if err := c.getData(ctx, path, &points); err != nil {
		return nil, err
	}
	return points, nil
}

func (c *Client) GetQemuRRDData(ctx context.Context, node, vmid, timeframe string) ([]RRDDataPoint, error) {
	if timeframe = strings.TrimSpace(timeframe); timeframe == "" {
		timeframe = "hour"
	}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/rrddata?timeframe=%s",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
		neturl.QueryEscape(timeframe),
	)
	var points []RRDDataPoint
	if err := c.getData(ctx, path, &points); err != nil {
		return nil, err
	}
	return points, nil
}

func (c *Client) GetLXCRRDData(ctx context.Context, node, vmid, timeframe string) ([]RRDDataPoint, error) {
	if timeframe = strings.TrimSpace(timeframe); timeframe == "" {
		timeframe = "hour"
	}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/rrddata?timeframe=%s",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
		neturl.QueryEscape(timeframe),
	)
	var points []RRDDataPoint
	if err := c.getData(ctx, path, &points); err != nil {
		return nil, err
	}
	return points, nil
}

func (c *Client) GetQemuAgentOSInfo(ctx context.Context, node, vmid string) (map[string]any, error) {
	info := map[string]any{}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/agent/get-osinfo",
		neturl.PathEscape(strings.TrimSpace(node)),
		neturl.PathEscape(strings.TrimSpace(vmid)),
	)
	if err := c.getData(ctx, path, &info); err != nil {
		return nil, err
	}
	return info, nil
}

func (c *Client) GetNodeNetwork(ctx context.Context, node string) ([]map[string]any, error) {
	var interfaces []map[string]any
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/network",
		neturl.PathEscape(strings.TrimSpace(node)),
	)
	if err := c.getData(ctx, path, &interfaces); err != nil {
		return nil, err
	}
	return interfaces, nil
}

func (c *Client) OpenNodeTermProxy(ctx context.Context, node string) (ProxyTicket, error) {
	return c.postProxy(ctx, fmt.Sprintf("/api2/json/nodes/%s/termproxy", neturl.PathEscape(node)), neturl.Values{})
}

func (c *Client) OpenQemuTermProxy(ctx context.Context, node, vmid string) (ProxyTicket, error) {
	return c.postProxy(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/termproxy",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) OpenLXCTermProxy(ctx context.Context, node, vmid string) (ProxyTicket, error) {
	return c.postProxy(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/termproxy",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{})
}

func (c *Client) OpenQemuVNCProxy(ctx context.Context, node, vmid string) (ProxyTicket, error) {
	values := neturl.Values{}
	values.Set("websocket", "1")
	values.Set("generate-password", "1")
	return c.postProxy(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/vncproxy",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), values)
}

func (c *Client) OpenLXCVNCProxy(ctx context.Context, node, vmid string) (ProxyTicket, error) {
	values := neturl.Values{}
	values.Set("websocket", "1")
	values.Set("generate-password", "1")
	return c.postProxy(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/lxc/%s/vncproxy",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), values)
}

func (c *Client) OpenQemuSPICEProxy(ctx context.Context, node, vmid string) (SPICETicket, error) {
	var ticket SPICETicket
	if err := c.postData(ctx, fmt.Sprintf(
		"/api2/json/nodes/%s/qemu/%s/spiceproxy",
		neturl.PathEscape(node),
		neturl.PathEscape(vmid),
	), neturl.Values{}, &ticket); err != nil {
		return SPICETicket{}, err
	}
	return ticket, nil
}

func (c *Client) BuildVNCWebSocketURL(node, kind, vmid string, port int, ticket string) (string, error) {
	base, err := neturl.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(base.Scheme) {
	case "https":
		base.Scheme = "wss"
	case "http":
		base.Scheme = "ws"
	default:
		base.Scheme = "wss"
	}

	// Build the correct resource-specific WebSocket path.
	// The vncwebsocket endpoint must match the resource that vncproxy was called on:
	//   node:  /api2/json/nodes/{node}/vncwebsocket
	//   qemu:  /api2/json/nodes/{node}/qemu/{vmid}/vncwebsocket
	//   lxc:   /api2/json/nodes/{node}/lxc/{vmid}/vncwebsocket
	trimmedNode := neturl.PathEscape(strings.TrimSpace(node))
	trimmedVMID := neturl.PathEscape(strings.TrimSpace(vmid))
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "qemu":
		base.Path = fmt.Sprintf("/api2/json/nodes/%s/qemu/%s/vncwebsocket", trimmedNode, trimmedVMID)
	case "lxc":
		base.Path = fmt.Sprintf("/api2/json/nodes/%s/lxc/%s/vncwebsocket", trimmedNode, trimmedVMID)
	default:
		base.Path = fmt.Sprintf("/api2/json/nodes/%s/vncwebsocket", trimmedNode)
	}

	query := neturl.Values{}
	query.Set("port", strconv.Itoa(port))
	query.Set("vncticket", strings.TrimSpace(ticket))
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func (c *Client) postProxy(ctx context.Context, path string, values neturl.Values) (ProxyTicket, error) {
	var ticket ProxyTicket
	if err := c.postData(ctx, path, values, &ticket); err != nil {
		return ProxyTicket{}, err
	}
	return ticket, nil
}

// GetNodeSyslog returns syslog lines for a node with optional limit and since filter.
// since should be a timestamp string in a format Proxmox accepts (e.g. "2006-01-02T15:04:05").
func (c *Client) GetNodeSyslog(ctx context.Context, node string, limit int, since string) ([]map[string]any, error) {
	query := neturl.Values{}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	since = strings.TrimSpace(since)
	if since != "" {
		query.Set("since", since)
	}
	path := fmt.Sprintf("/api2/json/nodes/%s/syslog", neturl.PathEscape(strings.TrimSpace(node)))
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var result []map[string]any
	if err := c.getData(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}
