package proxmox

import (
	"errors"

	"github.com/labtether/labtether/internal/connectors/proxmox"
)

var ErrProxmoxMissingNode = errors.New("proxmox asset missing node metadata")

type ProxmoxAssetDetailsResponse struct {
	AssetID         string                   `json:"asset_id"`
	Kind            string                   `json:"kind"`
	Node            string                   `json:"node"`
	VMID            string                   `json:"vmid,omitempty"`
	CollectorID     string                   `json:"collector_id,omitempty"`
	Version         string                   `json:"version,omitempty"`
	Config          map[string]any           `json:"config,omitempty"`
	Snapshots       []proxmox.Snapshot       `json:"snapshots,omitempty"`
	Tasks           []proxmox.Task           `json:"tasks,omitempty"`
	HA              ProxmoxAssetHAView       `json:"ha"`
	FirewallRules   []proxmox.FirewallRule   `json:"firewall_rules,omitempty"`
	BackupSchedules []proxmox.BackupSchedule `json:"backup_schedules,omitempty"`
	CephStatus      *proxmox.CephStatus      `json:"ceph_status,omitempty"`
	CephOSDs        []proxmox.CephOSD        `json:"ceph_osds,omitempty"`
	ZFSPools        []proxmox.ZFSPool        `json:"zfs_pools,omitempty"`
	StorageContent  []proxmox.StorageContent `json:"storage_content,omitempty"`
	Warnings        []string                 `json:"warnings,omitempty"`
	FetchedAt       string                   `json:"fetched_at"`
}

type ProxmoxAssetHAView struct {
	Match     *proxmox.HAResource  `json:"match,omitempty"`
	Resources []proxmox.HAResource `json:"resources,omitempty"`
}

type ProxmoxStorageInsightsResponse struct {
	GeneratedAt string                        `json:"generated_at"`
	Window      string                        `json:"window"`
	AssetID     string                        `json:"asset_id"`
	Node        string                        `json:"node"`
	Kind        string                        `json:"kind"`
	Summary     ProxmoxStorageInsightsSummary `json:"summary"`
	Pools       []ProxmoxStorageInsightPool   `json:"pools,omitempty"`
	Events      []ProxmoxStorageInsightEvent  `json:"events,omitempty"`
	Warnings    []string                      `json:"warnings,omitempty"`
}

type ProxmoxStorageInsightsSummary struct {
	DegradedPools      int `json:"degraded_pools"`
	HotPools           int `json:"hot_pools"`
	PredictedFullLT30D int `json:"predicted_full_lt_30d"`
	ScrubOverdue       int `json:"scrub_overdue"`
	StaleTelemetry     int `json:"stale_telemetry"`
}

type ProxmoxStorageInsightPool struct {
	Name               string                           `json:"name"`
	Health             string                           `json:"health"`
	SizeBytes          *int64                           `json:"size_bytes,omitempty"`
	UsedBytes          *int64                           `json:"used_bytes,omitempty"`
	FreeBytes          *int64                           `json:"free_bytes,omitempty"`
	UsedPercent        *float64                         `json:"used_percent,omitempty"`
	FragPercent        *int                             `json:"frag_percent,omitempty"`
	DedupRatio         *float64                         `json:"dedup_ratio,omitempty"`
	GrowthBytes7D      *int64                           `json:"growth_bytes_7d,omitempty"`
	Forecast           ProxmoxStorageForecast           `json:"forecast"`
	Scrub              ProxmoxStorageScrub              `json:"scrub"`
	IO                 ProxmoxStorageIO                 `json:"io"`
	Errors             ProxmoxStorageErrors             `json:"errors"`
	Snapshots          ProxmoxStorageSnapshots          `json:"snapshots"`
	DependentWorkloads ProxmoxStorageDependentWorkloads `json:"dependent_workloads"`
	RiskScore          int                              `json:"risk_score"`
	RiskState          string                           `json:"risk_state"`
	Reasons            []string                         `json:"reasons,omitempty"`
	TelemetryStale     bool                             `json:"telemetry_stale"`
}

type ProxmoxStorageForecast struct {
	DaysTo80   *float64 `json:"days_to_80,omitempty"`
	DaysToFull *float64 `json:"days_to_full,omitempty"`
	Confidence string   `json:"confidence"`
}

type ProxmoxStorageScrub struct {
	LastCompletedAt string `json:"last_completed_at,omitempty"`
	Overdue         bool   `json:"overdue"`
}

type ProxmoxStorageIO struct {
	ReadLatencyMSP95  *float64 `json:"read_latency_ms_p95,omitempty"`
	WriteLatencyMSP95 *float64 `json:"write_latency_ms_p95,omitempty"`
}

type ProxmoxStorageErrors struct {
	Read     int `json:"read"`
	Write    int `json:"write"`
	Checksum int `json:"checksum"`
}

type ProxmoxStorageSnapshots struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

type ProxmoxStorageDependentWorkloads struct {
	VMCount int   `json:"vm_count"`
	CTCount int   `json:"ct_count"`
	VMIDs   []int `json:"vm_ids,omitempty"`
	CTIDs   []int `json:"ct_ids,omitempty"`
}

type ProxmoxStorageInsightEvent struct {
	Timestamp  string `json:"timestamp"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Pool       string `json:"pool,omitempty"`
	Node       string `json:"node,omitempty"`
	UPID       string `json:"upid,omitempty"`
	TaskType   string `json:"task_type,omitempty"`
	TaskStatus string `json:"task_status,omitempty"`
	ExitStatus string `json:"exit_status,omitempty"`
}
