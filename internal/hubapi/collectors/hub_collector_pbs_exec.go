package collectors

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/metricschema"
)

func (d *Deps) executePBSCollector(ctx context.Context, collector hubcollector.Collector) {
	if d.CredentialStore == nil || d.SecretsManager == nil {
		d.UpdateCollectorStatus(collector.ID, "error", "credential store unavailable")
		return
	}

	lifecycle := NewCollectorLifecycle(d, collector, "pbs", hubcollector.CollectorTypePBS)
	logFields := lifecycle.logFields

	baseURL := CollectorConfigString(collector.Config, "base_url")
	credentialID := CollectorConfigString(collector.Config, "credential_id")
	tokenID := CollectorConfigString(collector.Config, "token_id")
	if baseURL == "" {
		lifecycle.Fail("missing base_url in config")
		return
	}
	if credentialID == "" {
		lifecycle.Fail("missing credential_id in config")
		return
	}

	cred, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
	if err != nil || !ok {
		lifecycle.Fail("credential not found")
		return
	}
	tokenSecret, err := d.SecretsManager.DecryptString(cred.SecretCiphertext, cred.ID)
	if err != nil {
		lifecycle.Fail("failed to decrypt credential")
		return
	}

	if tokenID == "" {
		tokenID = strings.TrimSpace(cred.Username)
	}
	if tokenID == "" {
		tokenID = strings.TrimSpace(cred.Metadata["token_id"])
	}
	if tokenID == "" {
		lifecycle.Fail("missing token_id in config")
		return
	}

	skipVerify, hasSkip := collectorConfigBool(collector.Config, "skip_verify")
	if !hasSkip {
		skipVerify = false
	}

	client, err := pbs.NewClient(pbs.Config{
		BaseURL:     baseURL,
		TokenID:     tokenID,
		TokenSecret: strings.TrimSpace(tokenSecret),
		SkipVerify:  skipVerify,
		CAPEM:       CollectorConfigString(collector.Config, "ca_pem"),
		Timeout:     collectorConfigDuration(collector.Config, "timeout", 15*time.Second),
	})
	if err != nil {
		lifecycle.Failf("pbs client init failed: %v", err)
		return
	}

	collectorCtx, collectorCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer collectorCancel()

	datastores, err := client.ListDatastores(collectorCtx)
	if err != nil {
		lifecycle.Failf("pbs list datastores failed: %v", err)
		return
	}

	usageByStore := make(map[string]pbs.DatastoreUsage, len(datastores))
	if usage, usageErr := client.ListDatastoreUsage(collectorCtx); usageErr != nil {
		log.Printf("hub collector pbs: datastore usage call failed: %v", usageErr)
	} else {
		for _, entry := range usage {
			store := strings.TrimSpace(entry.Store)
			if store != "" {
				usageByStore[store] = entry
			}
		}
	}

	// Fetch server version for root asset enrichment (best-effort).
	var pbsVersion string
	if ver, verErr := client.GetVersion(collectorCtx); verErr == nil {
		pbsVersion = strings.TrimSpace(ver.Version)
		if release := strings.TrimSpace(ver.Release); release != "" && pbsVersion == "" {
			pbsVersion = release
		}
	}

	visibleDatastores := 0
	discovered := 0
	upsertFailures := 0
	// Aggregate storage totals across all datastores for root asset telemetry.
	var aggregateTotalBytes, aggregateUsedBytes int64
	var totalSnapshotCount, totalGroupCount int
	var latestBackupEpoch int64
	snapshotAssets := make([]connectorsdk.Asset, 0, len(datastores)+1)
	endpointHost, endpointIP := CollectorEndpointIdentity(baseURL)
	for _, datastore := range datastores {
		store := strings.TrimSpace(datastore.Store)
		if store == "" {
			continue
		}
		visibleDatastores++

		metadata := map[string]string{
			"collector_id":       strings.TrimSpace(collector.ID),
			"collector_base_url": strings.TrimSpace(baseURL),
			"store":              store,
			"mount_status":       strings.TrimSpace(datastore.MountStatus),
			"comment":            strings.TrimSpace(datastore.Comment),
		}
		if endpointHost != "" {
			metadata["collector_endpoint_host"] = endpointHost
		}
		if endpointIP != "" {
			metadata["collector_endpoint_ip"] = endpointIP
		}
		if maintenance := strings.TrimSpace(datastore.Maintenance); maintenance != "" {
			metadata["maintenance_mode"] = maintenance
		}

		status, statusErr := client.GetDatastoreStatus(collectorCtx, store, true)
		if statusErr != nil {
			log.Printf("hub collector pbs: failed to load status for %s: %v", store, statusErr)
		}

		total := firstNonZeroInt64(status.Total, usageByStore[store].Total)
		used := firstNonZeroInt64(status.Used, usageByStore[store].Used)
		avail := firstNonZeroInt64(status.Avail, usageByStore[store].Avail)
		if total > 0 {
			metadata["total_bytes"] = strconv.FormatInt(total, 10)
		}
		if used > 0 {
			metadata["used_bytes"] = strconv.FormatInt(used, 10)
		}
		if avail > 0 {
			metadata["avail_bytes"] = strconv.FormatInt(avail, 10)
		}
		if total > 0 && used >= 0 {
			usagePercent := (float64(used) / float64(total)) * 100
			if usagePercent < 0 {
				usagePercent = 0
			}
			if usagePercent > 100 {
				usagePercent = 100
			}
			metadata["usage_percent"] = formatMetricValue(usagePercent)
			metadata[metricschema.HeartbeatKeyDiskUsedPercent] = formatMetricValue(usagePercent)
		}
		if mountStatus := strings.TrimSpace(status.MountStatus); mountStatus != "" {
			metadata["mount_status"] = mountStatus
		}
		if status.GCStatus != nil {
			if upid := strings.TrimSpace(status.GCStatus.UPID); upid != "" {
				metadata["gc_last_upid"] = upid
			}
			if status.GCStatus.PendingBytes > 0 {
				metadata["gc_pending_bytes"] = strconv.FormatInt(status.GCStatus.PendingBytes, 10)
			}
			if status.GCStatus.RemovedBytes > 0 {
				metadata["gc_removed_bytes"] = strconv.FormatInt(status.GCStatus.RemovedBytes, 10)
			}
		}

		if groups, groupsErr := client.ListDatastoreGroups(collectorCtx, store); groupsErr != nil {
			log.Printf("hub collector pbs: failed to load groups for %s: %v", store, groupsErr)
		} else {
			metadata["group_count"] = strconv.Itoa(len(groups))
		}

		if snapshots, snapshotsErr := client.ListDatastoreSnapshots(collectorCtx, store); snapshotsErr != nil {
			log.Printf("hub collector pbs: failed to load snapshots for %s: %v", store, snapshotsErr)
		} else {
			metadata["snapshot_count"] = strconv.Itoa(len(snapshots))
			if latestBackup := latestPBSSnapshotEpoch(snapshots); latestBackup > 0 {
				backupAt := time.Unix(latestBackup, 0).UTC()
				metadata["last_backup_at"] = backupAt.Format(time.RFC3339)
				metadata["days_since_backup"] = formatMetricValue(time.Since(backupAt).Hours() / 24)
			}
		}
		// Accumulate aggregate storage totals for root asset.
		aggregateTotalBytes += total
		aggregateUsedBytes += used
		if groupCount, parseErr := strconv.Atoi(metadata["group_count"]); parseErr == nil {
			totalGroupCount += groupCount
		}
		if snapCount, parseErr := strconv.Atoi(metadata["snapshot_count"]); parseErr == nil {
			totalSnapshotCount += snapCount
		}
		if lba := metadata["last_backup_at"]; lba != "" {
			if parsed, parseErr := time.Parse(time.RFC3339, lba); parseErr == nil {
				if epoch := parsed.Unix(); epoch > latestBackupEpoch {
					latestBackupEpoch = epoch
				}
			}
		}

		_, metadata = WithCanonicalResourceMetadata("pbs", "storage-pool", metadata)

		statusValue := normalizePBSDatastoreStatus(metadata["mount_status"], metadata["maintenance_mode"])
		req := assets.HeartbeatRequest{
			AssetID:  "pbs-datastore-" + NormalizeAssetKey(store),
			Type:     "storage-pool",
			Name:     store,
			Source:   "pbs",
			Status:   statusValue,
			Platform: "",
			Metadata: metadata,
		}
		if _, err := d.ProcessHeartbeatRequest(req); err != nil {
			log.Printf("hub collector pbs: failed to upsert %s: %v", req.AssetID, err)
			upsertFailures++
			continue
		}
		snapshotAssets = append(snapshotAssets, ConnectorSnapshotAssetFromHeartbeat(req, ""))
		discovered++
	}

	if collector.AssetID != "" {
		rootName := CollectorConfigString(collector.Config, "display_name")
		if rootName == "" {
			rootName = endpointHost
		}
		if rootName == "" {
			rootName = "pbs"
		}
		clusterMeta := map[string]string{
			"connector_type":  "pbs",
			"discovered":      strconv.Itoa(discovered),
			"datastore_count": strconv.Itoa(discovered),
		}
		if pbsVersion != "" {
			clusterMeta["version"] = pbsVersion
		}
		if endpointHost != "" {
			clusterMeta["collector_endpoint_host"] = endpointHost
		}
		if endpointIP != "" {
			clusterMeta["collector_endpoint_ip"] = endpointIP
		}
		// Aggregate disk usage across all datastores.
		if aggregateTotalBytes > 0 {
			clusterMeta["total_bytes"] = strconv.FormatInt(aggregateTotalBytes, 10)
			clusterMeta["used_bytes"] = strconv.FormatInt(aggregateUsedBytes, 10)
			usagePercent := (float64(aggregateUsedBytes) / float64(aggregateTotalBytes)) * 100
			if usagePercent < 0 {
				usagePercent = 0
			}
			if usagePercent > 100 {
				usagePercent = 100
			}
			clusterMeta[metricschema.HeartbeatKeyDiskUsedPercent] = formatMetricValue(usagePercent)
		}
		if totalGroupCount > 0 {
			clusterMeta["group_count"] = strconv.Itoa(totalGroupCount)
		}
		if totalSnapshotCount > 0 {
			clusterMeta["snapshot_count"] = strconv.Itoa(totalSnapshotCount)
		}
		if latestBackupEpoch > 0 {
			clusterMeta["last_backup_at"] = time.Unix(latestBackupEpoch, 0).UTC().Format(time.RFC3339)
		}
		if rootAsset, ok := d.RefreshCollectorParentAsset(CollectorParentAssetRefreshOptions{
			Collector:      collector,
			Source:         "pbs",
			AssetType:      "storage-controller",
			Name:           rootName,
			Status:         "online",
			Metadata:       clusterMeta,
			LogPrefix:      "hub collector pbs",
			FailureSubject: "collector root asset",
		}); ok {
			snapshotAssets = append(snapshotAssets, rootAsset)
		}
	}

	var snapshotConnector connectorsdk.Connector
	if d.ConnectorRegistry != nil {
		if connector, ok := d.ConnectorRegistry.Get("pbs"); ok {
			snapshotConnector = connector
		}
	}
	d.PersistCanonicalConnectorSnapshot("pbs", collector.ID, "PBS", "", snapshotConnector, snapshotAssets)

	if visibleDatastores == 0 {
		lifecycle.Partial("no datastores discovered from PBS")
		return
	}
	if discovered == 0 {
		lifecycle.Failf("failed to persist discovered PBS datastores: visible=%d upsert_failures=%d", visibleDatastores, upsertFailures)
		return
	}
	if upsertFailures > 0 {
		lifecycle.Partialf("partial PBS inventory persisted: datastores=%d upsert_failures=%d", discovered, upsertFailures)
		return
	}

	taskCount := 0
	collectedTasks := make([]pbs.Task, 0, 30)
	node := CollectorConfigString(collector.Config, "node")
	if tasks, taskErr := client.ListNodeTasks(collectorCtx, node, 30); taskErr == nil {
		taskCount = len(tasks)
		collectedTasks = tasks
	} else {
		log.Printf("hub collector pbs: failed to list node tasks: %v", taskErr)
	}

	for _, task := range collectedTasks {
		upid := strings.TrimSpace(task.UPID)
		if upid == "" {
			continue
		}
		taskFields := cloneStringMap(logFields)
		taskFields["upid"] = upid
		taskFields["node"] = strings.TrimSpace(task.Node)
		taskFields["task_type"] = strings.TrimSpace(task.WorkerType)
		taskFields["task_target"] = strings.TrimSpace(task.WorkerID)
		taskFields["task_status"] = strings.TrimSpace(task.Status)
		if user := strings.TrimSpace(task.User); user != "" {
			taskFields["user"] = user
		}
		eventAt := time.Now().UTC()
		if task.EndTime > 0 {
			eventAt = time.Unix(task.EndTime, 0).UTC()
		} else if task.StartTime > 0 {
			eventAt = time.Unix(task.StartTime, 0).UTC()
		}
		d.AppendConnectorLogEventWithID(
			StableConnectorLogID("log_pbs_task", upid),
			collector.AssetID,
			"pbs",
			normalizeCollectorLogLevel(task.Status),
			fmt.Sprintf("pbs %s %s: %s",
				strings.TrimSpace(task.WorkerType),
				FirstNonEmptyString(strings.TrimSpace(task.WorkerID), upid),
				FirstNonEmptyString(strings.TrimSpace(task.Status), "unknown"),
			),
			taskFields,
			eventAt,
		)
	}

	summaryMessage := fmt.Sprintf("collector run complete: datastores=%d recent_tasks=%d", discovered, taskCount)
	d.AppendConnectorLogEventWithID(
		StableConnectorLogID("log_pbs_collector_summary", collector.ID+"|"+summaryMessage),
		collector.AssetID,
		"pbs",
		"info",
		summaryMessage,
		logFields,
		time.Now().UTC(),
	)
	d.UpdateCollectorStatus(collector.ID, "ok", "")
}

func latestPBSSnapshotEpoch(snapshots []pbs.BackupSnapshot) int64 {
	var latest int64
	for _, snapshot := range snapshots {
		if snapshot.BackupTime > latest {
			latest = snapshot.BackupTime
		}
	}
	return latest
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func normalizePBSDatastoreStatus(mountStatus, maintenance string) string {
	normalizedMount := strings.ToLower(strings.TrimSpace(mountStatus))
	normalizedMaintenance := strings.ToLower(strings.TrimSpace(maintenance))

	if normalizedMaintenance == "offline" || normalizedMaintenance == "delete" || normalizedMaintenance == "unmount" {
		return "offline"
	}
	if normalizedMount == "notmounted" {
		return "offline"
	}
	if normalizedMaintenance == "read-only" {
		return "stale"
	}
	return "online"
}

func FirstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
