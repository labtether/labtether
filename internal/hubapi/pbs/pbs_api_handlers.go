package pbs

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type PBSAssetDetailsResponse struct {
	AssetID     string                `json:"asset_id"`
	Kind        string                `json:"kind"`
	CollectorID string                `json:"collector_id,omitempty"`
	Node        string                `json:"node,omitempty"`
	Version     string                `json:"version,omitempty"`
	Store       string                `json:"store,omitempty"`
	Datastore   *PBSDatastoreSummary  `json:"datastore,omitempty"`
	Datastores  []PBSDatastoreSummary `json:"datastores,omitempty"`
	Tasks       []pbsconnector.Task   `json:"tasks,omitempty"`
	Warnings    []string              `json:"warnings,omitempty"`
	FetchedAt   string                `json:"fetched_at"`
}

type PBSDatastoreSummary struct {
	Store           string                          `json:"store"`
	Status          string                          `json:"status"`
	MountStatus     string                          `json:"mount_status,omitempty"`
	Maintenance     string                          `json:"maintenance_mode,omitempty"`
	Comment         string                          `json:"comment,omitempty"`
	TotalBytes      int64                           `json:"total_bytes,omitempty"`
	UsedBytes       int64                           `json:"used_bytes,omitempty"`
	AvailBytes      int64                           `json:"avail_bytes,omitempty"`
	UsagePercent    float64                         `json:"usage_percent,omitempty"`
	GroupCount      int                             `json:"group_count"`
	SnapshotCount   int                             `json:"snapshot_count"`
	LastBackupAt    string                          `json:"last_backup_at,omitempty"`
	DaysSinceBackup float64                         `json:"days_since_backup,omitempty"`
	GCStatus        *pbsconnector.DatastoreGCStatus `json:"gc_status,omitempty"`
}

type PBSBackupGroupEntry struct {
	BackupType  string `json:"backup_type"`
	BackupID    string `json:"backup_id"`
	Owner       string `json:"owner,omitempty"`
	Comment     string `json:"comment,omitempty"`
	BackupCount int64  `json:"backup_count"`
	LastBackup  int64  `json:"last_backup,omitempty"`
}

type pbsDatastoreGroups struct {
	Store  string                `json:"store"`
	Groups []PBSBackupGroupEntry `json:"groups"`
}

type PBSGroupsResponse struct {
	Datastores []pbsDatastoreGroups `json:"datastores"`
	Warnings   []string             `json:"warnings,omitempty"`
	FetchedAt  string               `json:"fetched_at"`
}

type PBSSnapshotEntry struct {
	BackupType   string                             `json:"backup_type"`
	BackupID     string                             `json:"backup_id"`
	BackupTime   int64                              `json:"backup_time"`
	Size         int64                              `json:"size,omitempty"`
	Protected    bool                               `json:"protected,omitempty"`
	Owner        string                             `json:"owner,omitempty"`
	Comment      string                             `json:"comment,omitempty"`
	Verification *pbsconnector.SnapshotVerification `json:"verification,omitempty"`
	Files        []string                           `json:"files,omitempty"`
}

type PBSSnapshotsResponse struct {
	Store     string             `json:"store"`
	Snapshots []PBSSnapshotEntry `json:"snapshots"`
	FetchedAt string             `json:"fetched_at"`
	Error     string             `json:"error,omitempty"`
}

type pbsDatastoreVerification struct {
	Store           string `json:"store"`
	VerifiedCount   int    `json:"verified_count"`
	UnverifiedCount int    `json:"unverified_count"`
	FailedCount     int    `json:"failed_count"`
	LastVerifyTime  int64  `json:"last_verify_time,omitempty"`
	Status          string `json:"status"`
}

type PBSVerificationResponse struct {
	Datastores []pbsDatastoreVerification `json:"datastores"`
	Warnings   []string                   `json:"warnings,omitempty"`
	FetchedAt  string                     `json:"fetched_at"`
}

// handlePBSAssets dispatches /pbs/assets/{assetID}/{action}.
func (d *Deps) HandlePBSAssets(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/pbs/assets/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "pbs asset path not found")
		return
	}
	parts := strings.Split(path, "/")
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "pbs asset path not found")
		return
	}
	if !apiv2.RequireAssetAccess(w, r, assetID) {
		return
	}
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown pbs asset action")
		return
	}
	action := strings.TrimSpace(parts[1])

	// Validate known actions before resolving the asset runtime.
	switch action {
	case "capabilities":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		asset, err := d.ResolvePBSAsset(assetID)
		if err != nil {
			WritePBSResolveError(w, err)
			return
		}
		d.HandlePBSCapabilities(w, asset)
		return
	case "details", "groups", "snapshots", "verification",
		"verify-jobs", "prune-jobs", "sync-jobs",
		"remotes", "traffic-control", "certificates",
		"datastores":
		// valid — handled below
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown pbs asset action")
		return
	}

	// Read-only legacy actions require GET.
	switch action {
	case "details", "verification", "certificates":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	asset, runtime, err := d.ResolvePBSAssetRuntime(assetID)
	if err != nil {
		WritePBSResolveError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	collectorID := runtime.CollectorID

	switch action {
	case "details":
		if !d.requirePBSAggregateAccess(w, r, asset, collectorID) {
			return
		}
		response, loadErr := d.LoadPBSAssetDetails(ctx, asset, runtime)
		if loadErr != nil {
			// A details refresh is a direct liveness probe of the configured PBS
			// endpoint. Reconcile a failed probe immediately so a user does not
			// keep seeing Online while the actionable upstream error is visible.
			// Do not improve an already-offline state to merely unresponsive.
			if _, heartbeatErr := d.upsertPBSAssetRefreshStatus(asset, pbsFailedRefreshStatus(asset.Status)); heartbeatErr != nil {
				securityruntime.Logf("pbs: details failed and asset liveness reconciliation failed: %v", heartbeatErr)
			}
			writePBSError(w, http.StatusBadGateway, "failed to load pbs details", loadErr)
			return
		}
		// A completed details request has just exercised the configured PBS
		// credentials and a critical upstream read. Treat that as fresh liveness
		// evidence so a user-initiated refresh can recover an asset that was
		// previously marked offline. Failed reads reconcile to unresponsive or
		// preserve an already-offline state in the branch above.
		if _, heartbeatErr := d.upsertPBSAssetRefreshStatus(asset, "online"); heartbeatErr != nil {
			securityruntime.Logf("pbs: details succeeded but asset liveness reconciliation failed: %v", heartbeatErr)
			response.Warnings = DedupeNonEmptyWarnings(append(response.Warnings, "asset status refresh unavailable"))
		}
		servicehttp.WriteJSON(w, http.StatusOK, response)

	case "groups":
		if len(parts) >= 3 && strings.TrimSpace(parts[2]) == "forget" {
			if !d.requirePBSStoreAccess(w, r, asset, collectorID, r.URL.Query().Get("store")) {
				return
			}
			d.HandlePBSGroupForget(ctx, w, r, collectorID)
			return
		}
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.requirePBSAggregateAccess(w, r, asset, collectorID) {
			return
		}
		d.HandlePBSAssetGroups(ctx, w, asset, runtime)

	case "snapshots":
		if len(parts) >= 3 {
			sub := strings.TrimSpace(parts[2])
			switch sub {
			case "verify":
				if !d.requirePBSStoreAccess(w, r, asset, collectorID, r.URL.Query().Get("store")) {
					return
				}
				d.HandlePBSSnapshotVerify(ctx, w, r, collectorID)
				return
			case "forget":
				if !d.requirePBSStoreAccess(w, r, asset, collectorID, r.URL.Query().Get("store")) {
					return
				}
				d.HandlePBSSnapshotForget(ctx, w, r, collectorID)
				return
			}
		}
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		requestedStore := strings.TrimSpace(r.URL.Query().Get("store"))
		if requestedStore == "" {
			requestedStore = PBSStoreFromAsset(asset)
		}
		if !d.requirePBSStoreAccess(w, r, asset, collectorID, requestedStore) {
			return
		}
		d.HandlePBSAssetSnapshots(ctx, w, r, asset, runtime)

	case "verification":
		if !d.requirePBSAggregateAccess(w, r, asset, collectorID) {
			return
		}
		d.HandlePBSAssetVerification(ctx, w, asset, runtime)

	case "verify-jobs":
		if !d.requirePBSCollectorAccess(w, r, collectorID) {
			return
		}
		d.HandlePBSVerifyJobs(ctx, w, r, collectorID, parts[1:])

	case "prune-jobs":
		if !d.requirePBSCollectorAccess(w, r, collectorID) {
			return
		}
		d.HandlePBSPruneJobs(ctx, w, r, collectorID, parts[1:])

	case "sync-jobs":
		if !d.requirePBSCollectorAccess(w, r, collectorID) {
			return
		}
		d.HandlePBSSyncJobs(ctx, w, r, collectorID, parts[1:])

	case "remotes":
		if !d.requirePBSCollectorAccess(w, r, collectorID) {
			return
		}
		d.HandlePBSRemotes(ctx, w, r, collectorID)

	case "traffic-control":
		if !d.requirePBSCollectorAccess(w, r, collectorID) {
			return
		}
		d.HandlePBSTrafficControl(ctx, w, r, collectorID, parts[1:])

	case "certificates":
		if !d.requirePBSCollectorAccess(w, r, collectorID) {
			return
		}
		d.HandlePBSCertificates(ctx, w, r, collectorID)

	case "datastores":
		// /pbs/assets/{assetID}/datastores/{ds}/{sub}
		if len(parts) < 4 {
			servicehttp.WriteError(w, http.StatusNotFound, "expected datastores/{store}/{action}")
			return
		}
		ds := strings.TrimSpace(parts[2])
		sub := strings.TrimSpace(parts[3])
		if !d.requirePBSStoreAccess(w, r, asset, collectorID, ds) {
			return
		}
		switch sub {
		case "gc":
			d.HandlePBSDatastoreGC(ctx, w, r, collectorID, ds)
		case "verify":
			d.HandlePBSDatastoreVerify(ctx, w, r, collectorID, ds)
		case "maintenance":
			d.HandlePBSDatastoreMaintenance(ctx, w, r, collectorID, ds)
		case "maintenance-enable":
			d.HandlePBSDatastoreMaintenanceMode(ctx, w, r, collectorID, ds, "read-only")
		case "maintenance-disable":
			d.HandlePBSDatastoreMaintenanceMode(ctx, w, r, collectorID, ds, "")
		default:
			servicehttp.WriteError(w, http.StatusNotFound, "unknown datastore action")
		}
	}
}

func (d *Deps) upsertPBSAssetRefreshStatus(asset assets.Asset, status string) (assets.Asset, error) {
	return d.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  asset.ID,
		Type:     asset.Type,
		Name:     asset.Name,
		Source:   asset.Source,
		GroupID:  asset.GroupID,
		Status:   status,
		Platform: asset.Platform,
		Metadata: clonePBSAssetMetadata(asset.Metadata),
	})
}

func pbsFailedRefreshStatus(currentStatus string) string {
	switch strings.ToLower(strings.TrimSpace(currentStatus)) {
	case "offline", "down", "critical", "error", "unhealthy", "failed", "stopped", "exited", "dead", "unknown", "unavailable":
		return "offline"
	default:
		return "unresponsive"
	}
}

func clonePBSAssetMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func (d *Deps) LoadPBSAssetDetails(ctx context.Context, asset assets.Asset, runtime *PBSRuntime) (PBSAssetDetailsResponse, error) {
	node := PBSNodeFromAsset(asset)
	response := PBSAssetDetailsResponse{
		AssetID:     strings.TrimSpace(asset.ID),
		CollectorID: strings.TrimSpace(runtime.CollectorID),
		Node:        node,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	warnings := make([]string, 0, 8)

	version, err := runtime.Client.GetVersion(ctx)
	if err != nil {
		warnings = append(warnings, pbsWarning("version unavailable", err))
	} else {
		response.Version = strings.TrimSpace(version.Release)
		if response.Version == "" {
			response.Version = strings.TrimSpace(version.Version)
		}
	}

	store := PBSStoreFromAsset(asset)
	if store != "" || strings.EqualFold(strings.TrimSpace(asset.Type), "storage-pool") {
		if store == "" {
			return PBSAssetDetailsResponse{}, fmt.Errorf("pbs datastore asset missing store metadata")
		}
		response.Kind = "datastore"
		response.Store = store

		summary, summaryWarnings, err := LoadPBSDatastoreSummary(ctx, runtime.Client, store, pbsconnector.DatastoreUsage{})
		if err != nil {
			return PBSAssetDetailsResponse{}, err
		}
		response.Datastore = &summary
		warnings = append(warnings, summaryWarnings...)

		// Node-wide tasks have no trustworthy datastore scope. A fuzzy worker ID
		// match can include a sibling store, so omit them for restricted API keys.
		if !shared.HasAssetRestriction(ctx) {
			tasks, taskErr := runtime.Client.ListNodeTasks(ctx, node, 60)
			if taskErr != nil {
				warnings = append(warnings, pbsWarning("task listing unavailable", taskErr))
			} else {
				response.Tasks = FilterAndSortPBSTasks(tasks, store, 40)
			}
		}
		response.Warnings = DedupeNonEmptyWarnings(warnings)
		return response, nil
	}

	response.Kind = "server"
	usageByStore := map[string]pbsconnector.DatastoreUsage{}
	usage, usageErr := runtime.Client.ListDatastoreUsage(ctx)
	if usageErr != nil {
		warnings = append(warnings, pbsWarning("datastore usage unavailable", usageErr))
	} else {
		for _, entry := range usage {
			storeName := strings.TrimSpace(entry.Store)
			if storeName != "" {
				usageByStore[storeName] = entry
			}
		}
	}

	datastores, err := runtime.Client.ListDatastores(ctx)
	if err != nil {
		return PBSAssetDetailsResponse{}, err
	}

	type datastoreSummaryResult struct {
		summary  PBSDatastoreSummary
		warnings []string
		err      error
	}
	results := make([]datastoreSummaryResult, len(datastores))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for idx, datastore := range datastores {
		storeName := strings.TrimSpace(datastore.Store)
		if storeName == "" {
			continue
		}
		wg.Add(1)
		go func(index int, entry pbsconnector.Datastore) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			storeName := strings.TrimSpace(entry.Store)
			summary, summaryWarnings, summaryErr := LoadPBSDatastoreSummary(ctx, runtime.Client, storeName, usageByStore[storeName])
			if summaryErr == nil {
				if summary.Comment == "" {
					summary.Comment = strings.TrimSpace(entry.Comment)
				}
				if summary.MountStatus == "" {
					summary.MountStatus = strings.TrimSpace(entry.MountStatus)
				}
				if summary.Maintenance == "" {
					summary.Maintenance = strings.TrimSpace(entry.Maintenance)
				}
			}
			results[index] = datastoreSummaryResult{
				summary:  summary,
				warnings: summaryWarnings,
				err:      summaryErr,
			}
		}(idx, datastore)
	}
	wg.Wait()

	summaries := make([]PBSDatastoreSummary, 0, len(datastores))
	for idx, datastore := range datastores {
		storeName := strings.TrimSpace(datastore.Store)
		if storeName == "" {
			continue
		}
		result := results[idx]
		if result.err != nil {
			warnings = append(warnings, fmt.Sprintf("datastore %s unavailable: %v", storeName, result.err))
			continue
		}
		summaries = append(summaries, result.summary)
		warnings = append(warnings, result.warnings...)
	}
	sort.Slice(summaries, func(i, j int) bool {
		return strings.ToLower(strings.TrimSpace(summaries[i].Store)) < strings.ToLower(strings.TrimSpace(summaries[j].Store))
	})
	response.Datastores = summaries

	tasks, taskErr := runtime.Client.ListNodeTasks(ctx, node, 80)
	if taskErr != nil {
		warnings = append(warnings, pbsWarning("task listing unavailable", taskErr))
	} else {
		response.Tasks = FilterAndSortPBSTasks(tasks, "", 50)
	}
	response.Warnings = DedupeNonEmptyWarnings(warnings)
	return response, nil
}

func LoadPBSDatastoreSummary(ctx context.Context, client *pbsconnector.Client, store string, usage pbsconnector.DatastoreUsage) (PBSDatastoreSummary, []string, error) {
	summary := PBSDatastoreSummary{
		Store: strings.TrimSpace(store),
	}
	warnings := make([]string, 0, 4)

	status, err := client.GetDatastoreStatus(ctx, summary.Store, true)
	if err != nil {
		return PBSDatastoreSummary{}, nil, err
	}
	summary.MountStatus = strings.TrimSpace(status.MountStatus)
	summary.Status = normalizePBSDatastoreStatus(summary.MountStatus, "")
	summary.GCStatus = status.GCStatus

	summary.TotalBytes = firstNonZeroInt64(status.Total, usage.Total)
	summary.UsedBytes = firstNonZeroInt64(status.Used, usage.Used)
	summary.AvailBytes = firstNonZeroInt64(status.Avail, usage.Avail)
	if summary.TotalBytes > 0 && summary.UsedBytes >= 0 {
		summary.UsagePercent = (float64(summary.UsedBytes) / float64(summary.TotalBytes)) * 100
		if summary.UsagePercent > 100 {
			summary.UsagePercent = 100
		}
	}

	groups, groupsErr := client.ListDatastoreGroups(ctx, summary.Store)
	if groupsErr != nil {
		warnings = append(warnings, fmt.Sprintf("%s groups unavailable: %v", summary.Store, groupsErr))
	} else {
		summary.GroupCount = len(groups)
	}

	snapshots, snapshotsErr := client.ListDatastoreSnapshots(ctx, summary.Store)
	if snapshotsErr != nil {
		warnings = append(warnings, fmt.Sprintf("%s snapshots unavailable: %v", summary.Store, snapshotsErr))
	} else {
		summary.SnapshotCount = len(snapshots)
		if latest := latestPBSSnapshotEpoch(snapshots); latest > 0 {
			backupAt := time.Unix(latest, 0).UTC()
			summary.LastBackupAt = backupAt.Format(time.RFC3339)
			days := time.Since(backupAt).Hours() / 24
			if days < 0 {
				days = 0
			}
			summary.DaysSinceBackup = days
		}
	}

	return summary, warnings, nil
}
