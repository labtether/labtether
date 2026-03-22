package collectors

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/metricschema"
)

func (d *Deps) executeProxmoxCollector(ctx context.Context, collector hubcollector.Collector) {
	// Per-collection timeout to prevent stalled API calls from blocking the
	// collector loop indefinitely (mirrors ExecuteTrueNASCollector pattern).
	collectorCtx, collectorCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer collectorCancel()
	lifecycle := NewCollectorLifecycle(d, collector, "proxmox", hubcollector.CollectorTypeProxmox)

	// Use the shared cached client so the collector reuses the same HTTP
	// connection pool as user-facing API handlers (Phase 1/2 pooling).
	client, err := d.LoadProxmoxRuntime(collector.ID)
	if err != nil {
		lifecycle.Failf("proxmox runtime: %v", err)
		return
	}

	resources, err := client.GetClusterResources(collectorCtx)
	if err != nil {
		lifecycle.Failf("proxmox query failed: %v", err)
		return
	}
	collectedAt := time.Now().UTC()
	latestBackups := collectLatestProxmoxBackups(collectorCtx, client, resources)
	guestIdentityMetadata := collectProxmoxGuestIdentityMetadata(collectorCtx, client, resources)

	eligibleResources := 0
	discovered := 0
	upsertFailures := 0
	snapshotAssets := make([]connectorsdk.Asset, 0, len(resources)+1)
	for _, resource := range resources {
		req, include := ProxmoxResourceHeartbeat(
			resource,
			collector.ID,
			latestBackups,
			proxmoxGuestIdentityForResource(resource, guestIdentityMetadata),
			collectedAt,
		)
		if !include {
			continue
		}
		eligibleResources++
		if _, err := d.ProcessHeartbeatRequest(req); err != nil {
			log.Printf("hub collector proxmox: failed to upsert %s: %v", req.AssetID, err)
			upsertFailures++
			continue
		}
		snapshotAssets = append(snapshotAssets, ConnectorSnapshotAssetFromHeartbeat(req, ""))
		discovered++
	}

	if clusterAsset, ok := d.KeepConnectorClusterAssetAlive(collector, "proxmox", discovered, "hub collector proxmox"); ok {
		snapshotAssets = append(snapshotAssets, clusterAsset)
	}

	d.PersistCanonicalConnectorSnapshot("proxmox", collector.ID, "Proxmox", "", nil, snapshotAssets)

	if eligibleResources == 0 {
		lifecycle.Partial("no node/vm/container resources discovered")
		return
	}
	if discovered == 0 {
		lifecycle.Failf("failed to persist discovered Proxmox resources: visible=%d upsert_failures=%d", eligibleResources, upsertFailures)
		return
	}

	taskLogs := 0
	if ingested, taskErr := d.ingestProxmoxTaskLogs(collectorCtx, client, collector.AssetID); taskErr != nil {
		log.Printf("hub collector proxmox: failed to ingest task logs: %v", taskErr)
	} else {
		taskLogs = ingested
	}
	d.AppendConnectorLogEvent(
		collector.AssetID,
		"proxmox",
		"info",
		fmt.Sprintf("collector run complete: discovered=%d task_events=%d", discovered, taskLogs),
		lifecycle.logFields,
		collectedAt,
	)

	if upsertFailures > 0 {
		lifecycle.Partialf("partial Proxmox inventory persisted: resources=%d upsert_failures=%d", discovered, upsertFailures)
		return
	}

	d.UpdateCollectorStatus(collector.ID, "ok", "")
}

func ProxmoxResourceHeartbeat(
	resource proxmox.Resource,
	collectorID string,
	latestBackups map[string]time.Time,
	guestIdentity map[string]string,
	collectedAt time.Time,
) (assets.HeartbeatRequest, bool) {
	resourceType := strings.ToLower(strings.TrimSpace(resource.Type))

	nodeName := strings.TrimSpace(resource.Node)
	if nodeName == "" {
		nodeName = strings.TrimSpace(resource.Name)
	}
	vmid := proxmoxVMIDString(resource.VMID)

	assetID := ""
	assetType := ""
	name := strings.TrimSpace(resource.Name)

	switch resourceType {
	case "node":
		if nodeName == "" {
			return assets.HeartbeatRequest{}, false
		}
		assetID = "proxmox-node-" + NormalizeAssetKey(nodeName)
		assetType = "hypervisor-node"
		if name == "" {
			name = nodeName
		}
	case "qemu":
		if nodeName == "" || vmid == "" {
			return assets.HeartbeatRequest{}, false
		}
		assetID = "proxmox-vm-" + vmid
		assetType = "vm"
		if name == "" {
			name = "vm-" + vmid
		}
	case "lxc":
		if nodeName == "" || vmid == "" {
			return assets.HeartbeatRequest{}, false
		}
		assetID = "proxmox-ct-" + vmid
		assetType = "container"
		if name == "" {
			name = "ct-" + vmid
		}
	case "storage":
		storageID := strings.TrimSpace(resource.ID)
		if storageID == "" {
			storageID = strings.TrimSpace(resource.Name)
		}
		if storageID == "" {
			return assets.HeartbeatRequest{}, false
		}
		assetID = "proxmox-storage-" + NormalizeAssetKey(storageID)
		assetType = "storage-pool"
		if name == "" {
			name = storageID
		}
	default:
		return assets.HeartbeatRequest{}, false
	}

	metadata := map[string]string{
		"proxmox_type": resourceType,
		"proxmox_id":   strings.TrimSpace(resource.ID),
		"collector_id": strings.TrimSpace(collectorID),
		"status":       strings.TrimSpace(resource.Status),
		"node":         nodeName,
		"hastate":      strings.TrimSpace(resource.HAState),
		"uptime_sec":   formatMetricValue(resource.Uptime),
	}
	if vmid != "" {
		metadata["vmid"] = vmid
	}
	if resourceType == "storage" {
		metadata["storage_id"] = strings.TrimSpace(resource.ID)
	}
	if (resourceType == "qemu" || resourceType == "lxc") && vmid != "" {
		if backupAt, ok := latestBackups[vmid]; ok {
			metadata["last_backup_at"] = backupAt.UTC().Format(time.RFC3339)
			metadata["days_since_backup"] = formatMetricValue(collectedAt.Sub(backupAt).Hours() / 24)
		} else {
			metadata["backup_state"] = "missing"
		}
	}
	if strings.TrimSpace(resource.Content) != "" {
		metadata["content"] = strings.TrimSpace(resource.Content)
	}
	if strings.TrimSpace(resource.PlugInType) != "" {
		metadata["plugintype"] = strings.TrimSpace(resource.PlugInType)
	}
	if template := strings.TrimSpace(fmt.Sprintf("%v", resource.Template)); template != "" && template != "<nil>" {
		metadata["template"] = template
	}
	for key, value := range guestIdentity {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		metadata[key] = trimmed
	}

	metadata[metricschema.HeartbeatKeyCPUUsedPercent] = formatMetricValue(derivePercentFromRatio(resource.CPU))
	if resource.MaxCPU > 0 {
		metadata["cpu_cores_physical"] = formatMetricValue(resource.MaxCPU)
	}
	if memPercent, ok := derivePercent(resource.Mem, resource.MaxMem); ok {
		metadata[metricschema.HeartbeatKeyMemoryUsedPercent] = formatMetricValue(memPercent)
	}
	if resource.MaxMem > 0 {
		metadata["memory_total_bytes"] = formatMetricValue(resource.MaxMem)
	}
	if diskPercent, ok := derivePercent(resource.Disk, resource.MaxDisk); ok {
		metadata[metricschema.HeartbeatKeyDiskUsedPercent] = formatMetricValue(diskPercent)
	}
	// NOTE: Proxmox /cluster/resources reports netin/netout as cumulative total
	// bytes since boot, NOT per-second throughput rates. Storing these as
	// network_rx/tx_bytes_per_sec would produce absurdly large values (e.g.
	// 11.6 TB/s). We intentionally omit network throughput here — accurate
	// rates would require delta tracking between collection cycles.
	_, metadata = WithCanonicalResourceMetadata("proxmox", assetType, metadata)

	return assets.HeartbeatRequest{
		AssetID:  assetID,
		Type:     assetType,
		Name:     name,
		Source:   "proxmox",
		Status:   normalizeProxmoxStatus(resource.Status),
		Platform: "",
		Metadata: metadata,
	}, true
}
