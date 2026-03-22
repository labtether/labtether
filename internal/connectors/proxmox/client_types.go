package proxmox

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Resource represents one row returned from /cluster/resources.
type Resource struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	Node       string  `json:"node"`
	VMID       float64 `json:"vmid"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	CPU        float64 `json:"cpu"`
	MaxCPU     float64 `json:"maxcpu"`
	Mem        float64 `json:"mem"`
	MaxMem     float64 `json:"maxmem"`
	Disk       float64 `json:"disk"`
	MaxDisk    float64 `json:"maxdisk"`
	NetIn      float64 `json:"netin"`
	NetOut     float64 `json:"netout"`
	Uptime     float64 `json:"uptime"`
	Template   any     `json:"template"`
	HAState    string  `json:"hastate"`
	PlugInType string  `json:"plugintype"`
	Content    string  `json:"content"`
}

// StorageBackup represents one backup volume entry from storage/content.
type StorageBackup struct {
	VolID   string  `json:"volid"`
	Content string  `json:"content"`
	VMID    float64 `json:"vmid"`
	CTime   float64 `json:"ctime"`
	Size    float64 `json:"size"`
}

// StorageContent represents a file/volume in Proxmox storage.
type StorageContent struct {
	VolID   string `json:"volid"`
	Format  string `json:"format"` // qcow2, raw, subvol, iso, vztmpl
	Size    int64  `json:"size"`
	CTime   int64  `json:"ctime,omitempty"` // creation time (unix epoch)
	Content string `json:"content"`         // images, iso, vztmpl, backup, rootdir
	VMID    int    `json:"vmid,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// Snapshot represents a VM/CT snapshot entry.
type Snapshot struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Parent      string  `json:"parent"`
	SnapTime    float64 `json:"snaptime"`
	VMState     string  `json:"vmstate"`
}

// Task represents one row from /cluster/tasks.
type Task struct {
	UPID       string  `json:"upid"`
	Node       string  `json:"node"`
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	User       string  `json:"user"`
	Status     string  `json:"status"`
	ExitStatus string  `json:"exitstatus"`
	StartTime  float64 `json:"starttime"`
	EndTime    float64 `json:"endtime"`
}

// HAResource represents one row from /cluster/ha/resources.
type HAResource struct {
	SID         string  `json:"sid"`
	State       string  `json:"state"`
	Status      string  `json:"status"`
	Group       string  `json:"group"`
	Node        string  `json:"node"`
	Comment     string  `json:"comment"`
	MaxRestart  float64 `json:"max_restart"`
	MaxRelocate float64 `json:"max_relocate"`
}

// FirewallRule represents a single Proxmox firewall rule.
type FirewallRule struct {
	Pos     int    `json:"pos"`
	Type    string `json:"type"`   // in, out, group
	Action  string `json:"action"` // ACCEPT, DROP, REJECT
	Source  string `json:"source,omitempty"`
	Dest    string `json:"dest,omitempty"`
	Proto   string `json:"proto,omitempty"`
	Dport   string `json:"dport,omitempty"`
	Sport   string `json:"sport,omitempty"`
	Enable  int    `json:"enable"`
	Comment string `json:"comment,omitempty"`
	Macro   string `json:"macro,omitempty"`
	IFace   string `json:"iface,omitempty"`
}

// BackupSchedule represents a Proxmox backup job configuration.
type BackupSchedule struct {
	ID       string `json:"id"`
	Schedule string `json:"schedule"` // cron-like schedule e.g. "sat 02:00"
	Storage  string `json:"storage"`
	Mode     string `json:"mode"`     // snapshot, stop, suspend
	Compress string `json:"compress"` // zstd, lzo, gzip, none
	MailTo   string `json:"mailto,omitempty"`
	Enabled  int    `json:"enabled"`
	VMIDs    string `json:"vmid,omitempty"`    // comma-separated VMIDs or "all"
	Exclude  string `json:"exclude,omitempty"` // excluded VMIDs
	MaxFiles int    `json:"maxfiles,omitempty"`
	Pool     string `json:"pool,omitempty"`
	Comment  string `json:"comment,omitempty"`
	Node     string `json:"node,omitempty"`
}

// TaskStatus represents /nodes/{node}/tasks/{upid}/status data.
type TaskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
	Type       string `json:"type,omitempty"`
}

// ProxyTicket represents termproxy/vncproxy data used for WebSocket bridging.
type ProxyTicket struct {
	Port     flexInt `json:"port"`
	Ticket   string  `json:"ticket"`
	User     string  `json:"user"`
	UPID     string  `json:"upid"`
	Cert     string  `json:"cert,omitempty"`
	Password string  `json:"password,omitempty"` // #nosec G117 -- Connector payload field carries runtime credential material.
}

// SPICETicket represents /spiceproxy credentials for browser-side SPICE clients.
type SPICETicket struct {
	Host     string `json:"host"`
	TLSPort  int    `json:"tls-port"`
	Password string `json:"password"` // #nosec G117 -- Connector payload field carries runtime credential material.
	CA       string `json:"ca,omitempty"`
	Type     string `json:"type,omitempty"`
	Proxy    string `json:"proxy,omitempty"`
}

// flexInt handles JSON numbers that may arrive as either int or string.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	// Try int first.
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*f = flexInt(n)
		return nil
	}
	// Fall back to string.
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("flexInt: cannot unmarshal %s", string(b))
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fmt.Errorf("flexInt: invalid number string %q", s)
	}
	*f = flexInt(parsed)
	return nil
}

func (f flexInt) Int() int { return int(f) }
