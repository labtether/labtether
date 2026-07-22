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
	"github.com/labtether/labtether/internal/telemetry"
)

func (d *Deps) executePBSCollector(ctx context.Context, collector hubcollector.Collector) {
	if collectorContextCanceled(ctx) {
		return
	}
	if d.CredentialStore == nil || d.SecretsManager == nil {
		d.UpdateCollectorStatus(collector.ID, "error", "credential store unavailable")
		return
	}

	lifecycle := NewCollectorLifecycle(d, collector, "pbs", hubcollector.CollectorTypePBS)

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
	if collectorContextCanceled(ctx) {
		return
	}
	if err != nil || !ok {
		lifecycle.Fail("credential not found")
		return
	}
	tokenSecret, err := d.SecretsManager.DecryptString(cred.SecretCiphertext, cred.ID)
	if collectorContextCanceled(ctx) {
		return
	}
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
	d.executePBSCollectorWithClient(collectorCtx, collector, lifecycle, baseURL, client)
}

type pbsCollectorClient interface {
	ListDatastores(context.Context) ([]pbs.Datastore, error)
	ListDatastoreUsage(context.Context) ([]pbs.DatastoreUsage, error)
	GetVersion(context.Context) (pbs.Version, error)
	GetDatastoreStatus(context.Context, string, bool) (pbs.DatastoreStatus, error)
	ListDatastoreGroups(context.Context, string) ([]pbs.BackupGroup, error)
	ListDatastoreSnapshots(context.Context, string) ([]pbs.BackupSnapshot, error)
	ListNodeTasks(context.Context, string, int) ([]pbs.Task, error)
}

func (d *Deps) executePBSCollectorWithClient(
	collectorCtx context.Context,
	collector hubcollector.Collector,
	lifecycle CollectorLifecycle,
	baseURL string,
	client pbsCollectorClient,
) {
	if collectorContextCanceled(collectorCtx) {
		return
	}
	logFields := lifecycle.logFields

	datastores, err := client.ListDatastores(collectorCtx)
	if err != nil {
		if collectorContextCanceled(collectorCtx) {
			return
		}
		lifecycle.Failf("pbs list datastores failed: %v", err)
		return
	}

	usageByStore := make(map[string]pbs.DatastoreUsage, len(datastores))
	if usage, usageErr := client.ListDatastoreUsage(collectorCtx); usageErr != nil {
		if collectorContextCanceled(collectorCtx) {
			return
		}
		log.Printf("hub collector pbs: datastore usage call failed: %v", usageErr)
	} else {
		for _, entry := range usage {
			store := strings.TrimSpace(entry.Store)
			if store != "" {
				usageByStore[store] = entry
			}
		}
	}
	if collectorContextCanceled(collectorCtx) {
		return
	}

	// Fetch server version for root asset enrichment (best-effort).
	var pbsVersion string
	if ver, verErr := client.GetVersion(collectorCtx); verErr == nil {
		pbsVersion = strings.TrimSpace(ver.Version)
		if release := strings.TrimSpace(ver.Release); release != "" && pbsVersion == "" {
			pbsVersion = release
		}
	}
	if collectorContextCanceled(collectorCtx) {
		return
	}

	visibleDatastores := 0
	discovered := 0
	upsertFailures := 0
	// Aggregate storage totals across all datastores for root asset telemetry.
	var aggregateTotalBytes, aggregateUsedBytes int64
	var totalSnapshotCount, totalGroupCount int
	var latestBackupEpoch int64
	metricCollectedAt := time.Now().UTC()
	metricSamples := make([]telemetry.MetricSample, 0, min(len(datastores), telemetry.MaxBridgePBSDataStoreSeries)*6)
	metricDatastores := 0
	snapshotAssets := make([]connectorsdk.Asset, 0, len(datastores)+1)
	endpointHost, endpointIP := CollectorEndpointIdentity(baseURL)
	for _, datastore := range datastores {
		if collectorContextCanceled(collectorCtx) {
			return
		}
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
		if collectorContextCanceled(collectorCtx) {
			return
		}
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
		gcPendingKnown := status.GCStatus != nil
		gcPendingBytes := int64(0)
		if status.GCStatus != nil {
			gcPendingBytes = status.GCStatus.PendingBytes
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
			if collectorContextCanceled(collectorCtx) {
				return
			}
			log.Printf("hub collector pbs: failed to load groups for %s: %v", store, groupsErr)
		} else {
			metadata["group_count"] = strconv.Itoa(len(groups))
		}
		if collectorContextCanceled(collectorCtx) {
			return
		}

		backupCountKnown := false
		backupCount := 0
		latestDatastoreBackupEpoch := int64(0)
		if snapshots, snapshotsErr := client.ListDatastoreSnapshots(collectorCtx, store); snapshotsErr != nil {
			if collectorContextCanceled(collectorCtx) {
				return
			}
			log.Printf("hub collector pbs: failed to load snapshots for %s: %v", store, snapshotsErr)
		} else {
			backupCountKnown = true
			backupCount = len(snapshots)
			metadata["snapshot_count"] = strconv.Itoa(len(snapshots))
			if latestBackup := latestPBSSnapshotEpoch(snapshots); latestBackup > 0 {
				latestDatastoreBackupEpoch = latestBackup
				backupAt := time.Unix(latestBackup, 0).UTC()
				metadata["last_backup_at"] = backupAt.Format(time.RFC3339)
				metadata["days_since_backup"] = formatMetricValue(time.Since(backupAt).Hours() / 24)
			}
		}
		if collectorContextCanceled(collectorCtx) {
			return
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
		req = ScopedCollectorHeartbeatRequest(collector.ID, req)
		if collectorContextCanceled(collectorCtx) {
			return
		}
		if _, err := d.ProcessScopedCollectorHeartbeat(collector.ID, req); err != nil {
			log.Printf("hub collector pbs: failed to upsert %s: %v", req.AssetID, err)
			upsertFailures++
			continue
		}
		if collectorContextCanceled(collectorCtx) {
			return
		}
		if metricDatastores < telemetry.MaxBridgePBSDataStoreSeries {
			metricSamples = append(metricSamples, PBSDatastoreMetricSamples(PBSDatastoreMetricSnapshot{
				AssetID: req.AssetID, Datastore: store, CollectedAt: metricCollectedAt,
				Total: total, Used: used, Available: avail,
				BackupCountKnown: backupCountKnown, BackupCount: backupCount,
				LatestBackupEpoch: latestDatastoreBackupEpoch,
				GCPendingKnown:    gcPendingKnown, GCPendingBytes: gcPendingBytes,
			})...)
			metricDatastores++
		}
		snapshotAssets = append(snapshotAssets, ConnectorSnapshotAssetFromHeartbeat(req, ""))
		discovered++
	}

	if collectorContextCanceled(collectorCtx) {
		return
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
	if collectorContextCanceled(collectorCtx) {
		return
	}

	var snapshotConnector connectorsdk.Connector
	if d.ConnectorRegistry != nil {
		if connector, ok := d.ConnectorRegistry.Get("pbs"); ok {
			snapshotConnector = connector
		}
	}
	d.PersistCanonicalConnectorSnapshot("pbs", collector.ID, "PBS", "", snapshotConnector, snapshotAssets)
	if collectorContextCanceled(collectorCtx) {
		return
	}

	metricAppendFailed := false
	if len(metricSamples) > 0 && d.TelemetryStore != nil {
		if err := d.TelemetryStore.AppendSamples(collectorCtx, metricSamples); err != nil {
			if collectorContextCanceled(collectorCtx) {
				return
			}
			metricAppendFailed = true
			log.Printf("hub collector pbs: failed to persist datastore telemetry: %v", err)
		}
	}
	if collectorContextCanceled(collectorCtx) {
		return
	}

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
		if collectorContextCanceled(collectorCtx) {
			return
		}
		log.Printf("hub collector pbs: failed to list node tasks: %v", taskErr)
	}
	if collectorContextCanceled(collectorCtx) {
		return
	}

	for _, task := range collectedTasks {
		if collectorContextCanceled(collectorCtx) {
			return
		}
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
	if metricAppendFailed {
		if collectorContextCanceled(collectorCtx) {
			return
		}
		lifecycle.Partialf("PBS inventory and recent tasks persisted but datastore telemetry persistence failed: recent_tasks=%d", taskCount)
		return
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
	if collectorContextCanceled(collectorCtx) {
		return
	}
	d.UpdateCollectorStatus(collector.ID, "ok", "")
}

// PBSDatastoreMetricSnapshot is the exact data already loaded during one PBS
// collector pass. It avoids an export-only second poll while preserving which
// optional API calls actually succeeded.
type PBSDatastoreMetricSnapshot struct {
	AssetID           string
	Datastore         string
	CollectedAt       time.Time
	Total             int64
	Used              int64
	Available         int64
	BackupCountKnown  bool
	BackupCount       int
	LatestBackupEpoch int64
	GCPendingKnown    bool
	GCPendingBytes    int64
}

func PBSDatastoreMetricSamples(snapshot PBSDatastoreMetricSnapshot) []telemetry.MetricSample {
	if strings.TrimSpace(snapshot.AssetID) == "" || strings.TrimSpace(snapshot.Datastore) == "" || snapshot.CollectedAt.IsZero() {
		return nil
	}
	labels := map[string]string{"datastore": strings.TrimSpace(snapshot.Datastore)}
	out := make([]telemetry.MetricSample, 0, 6)
	appendSample := func(metric, unit string, value float64) {
		sample := telemetry.MetricSample{
			AssetID: snapshot.AssetID, Metric: metric, Unit: unit, Value: value,
			CollectedAt: snapshot.CollectedAt.UTC(), Labels: labels,
		}
		if _, err := telemetry.MetricSampleEnvelopeBytes(sample); err == nil {
			out = append(out, sample)
		}
	}
	if snapshot.Total > 0 && snapshot.Used >= 0 && snapshot.Used <= snapshot.Total &&
		snapshot.Available >= 0 && snapshot.Available <= snapshot.Total && snapshot.Used <= snapshot.Total-snapshot.Available {
		appendSample(telemetry.MetricStorageTotalBytes, "bytes", float64(snapshot.Total))
		appendSample(telemetry.MetricStorageUsedBytes, "bytes", float64(snapshot.Used))
		appendSample(telemetry.MetricStorageAvailableBytes, "bytes", float64(snapshot.Available))
	}
	if snapshot.BackupCountKnown && snapshot.BackupCount >= 0 {
		appendSample(telemetry.MetricBackupCount, "count", float64(snapshot.BackupCount))
	}
	if snapshot.LatestBackupEpoch > 0 {
		age := snapshot.CollectedAt.Sub(time.Unix(snapshot.LatestBackupEpoch, 0)).Seconds()
		if age >= 0 {
			appendSample(telemetry.MetricBackupAgeSeconds, "seconds", age)
		}
	}
	if snapshot.GCPendingKnown && snapshot.GCPendingBytes >= 0 {
		appendSample(telemetry.MetricGCPendingBytes, "bytes", float64(snapshot.GCPendingBytes))
	}
	return out
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
