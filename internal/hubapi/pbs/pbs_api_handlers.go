package pbs

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/assets"
	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/servicehttp"
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
	case "details", "groups", "verification", "certificates":
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
		response, loadErr := d.LoadPBSAssetDetails(ctx, asset, runtime)
		if loadErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to load pbs details: "+loadErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, response)

	case "groups":
		if len(parts) >= 3 && strings.TrimSpace(parts[2]) == "forget" {
			d.HandlePBSGroupForget(ctx, w, r, collectorID)
			return
		}
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.HandlePBSAssetGroups(ctx, w, asset, runtime)

	case "snapshots":
		if len(parts) >= 3 {
			sub := strings.TrimSpace(parts[2])
			switch sub {
			case "verify":
				d.HandlePBSSnapshotVerify(ctx, w, r, collectorID)
				return
			case "forget":
				d.HandlePBSSnapshotForget(ctx, w, r, collectorID)
				return
			}
		}
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.HandlePBSAssetSnapshots(ctx, w, r, asset, runtime)

	case "verification":
		d.HandlePBSAssetVerification(ctx, w, asset, runtime)

	case "verify-jobs":
		d.HandlePBSVerifyJobs(ctx, w, r, collectorID, parts[1:])

	case "prune-jobs":
		d.HandlePBSPruneJobs(ctx, w, r, collectorID, parts[1:])

	case "sync-jobs":
		d.HandlePBSSyncJobs(ctx, w, r, collectorID, parts[1:])

	case "remotes":
		d.HandlePBSRemotes(ctx, w, r, collectorID)

	case "traffic-control":
		d.HandlePBSTrafficControl(ctx, w, r, collectorID, parts[1:])

	case "certificates":
		d.HandlePBSCertificates(ctx, w, r, collectorID)

	case "datastores":
		// /pbs/assets/{assetID}/datastores/{ds}/{sub}
		if len(parts) < 4 {
			servicehttp.WriteError(w, http.StatusNotFound, "expected datastores/{store}/{action}")
			return
		}
		ds := strings.TrimSpace(parts[2])
		sub := strings.TrimSpace(parts[3])
		switch sub {
		case "gc":
			d.HandlePBSDatastoreGC(ctx, w, r, collectorID, ds)
		case "maintenance":
			d.HandlePBSDatastoreMaintenance(ctx, w, r, collectorID, ds)
		default:
			servicehttp.WriteError(w, http.StatusNotFound, "unknown datastore action")
		}
	}
}

// handlePBSTaskRoutes dispatches /pbs/tasks/{node}/{upid}/{action}.
func (d *Deps) HandlePBSTaskRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/pbs/tasks/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing task path")
		return
	}
	if strings.HasSuffix(path, "/status") {
		d.HandlePBSTaskStatus(w, r)
		return
	}
	if strings.HasSuffix(path, "/log") {
		d.HandlePBSTaskLog(w, r)
		return
	}
	if strings.HasSuffix(path, "/stop") {
		d.HandlePBSTaskStop(w, r)
		return
	}
	servicehttp.WriteError(w, http.StatusNotFound, "unknown pbs task action")
}

// handlePBSTaskStatus handles GET /pbs/tasks/{node}/{upid}/status.
func (d *Deps) HandlePBSTaskStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	node, upid, ok := ParsePBSTaskPath(r.URL.Path, "status")
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /pbs/tasks/{node}/{upid}/status")
		return
	}
	if node == "" || upid == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node and upid are required")
		return
	}

	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	status, err := runtime.Client.GetTaskStatus(ctx, node, upid)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch pbs task status: "+err.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"task": status})
}

// handlePBSTaskLog handles GET /pbs/tasks/{node}/{upid}/log.
func (d *Deps) HandlePBSTaskLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	node, upid, ok := ParsePBSTaskPath(r.URL.Path, "log")
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /pbs/tasks/{node}/{upid}/log")
		return
	}
	if node == "" || upid == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node and upid are required")
		return
	}

	limit := 200
	if parsed, ok := shared.ParsePositiveInt(r.URL.Query().Get("limit")); ok {
		limit = parsed
	}
	if limit > 2000 {
		limit = 2000
	}
	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	lines, err := runtime.Client.GetTaskLog(ctx, node, upid, limit)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch pbs task log: "+err.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

// handlePBSTaskStop handles POST /pbs/tasks/{node}/{upid}/stop.
func (d *Deps) HandlePBSTaskStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.RequireAdminAuth(w, r) {
		return
	}
	node, upid, ok := ParsePBSTaskPath(r.URL.Path, "stop")
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /pbs/tasks/{node}/{upid}/stop")
		return
	}
	if node == "" || upid == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node and upid are required")
		return
	}

	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := runtime.Client.StopTask(ctx, node, upid); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to stop pbs task: "+err.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
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
		warnings = append(warnings, "version unavailable: "+err.Error())
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

		tasks, taskErr := runtime.Client.ListNodeTasks(ctx, node, 60)
		if taskErr != nil {
			warnings = append(warnings, "task listing unavailable: "+taskErr.Error())
		} else {
			response.Tasks = FilterAndSortPBSTasks(tasks, store, 40)
		}
		response.Warnings = DedupeNonEmptyWarnings(warnings)
		return response, nil
	}

	response.Kind = "server"
	usageByStore := map[string]pbsconnector.DatastoreUsage{}
	usage, usageErr := runtime.Client.ListDatastoreUsage(ctx)
	if usageErr != nil {
		warnings = append(warnings, "datastore usage unavailable: "+usageErr.Error())
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
		warnings = append(warnings, "task listing unavailable: "+taskErr.Error())
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

func ParsePBSTaskPath(path, action string) (node string, upid string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/pbs/tasks/")
	if trimmed == path || trimmed == "" {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) < 3 || strings.TrimSpace(parts[2]) != action {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func FilterAndSortPBSTasks(tasks []pbsconnector.Task, store string, limit int) []pbsconnector.Task {
	filtered := make([]pbsconnector.Task, 0, len(tasks))
	trimmedStore := strings.ToLower(strings.TrimSpace(store))

	for _, task := range tasks {
		if trimmedStore != "" {
			workerID := strings.ToLower(strings.TrimSpace(task.WorkerID))
			upid := strings.ToLower(strings.TrimSpace(task.UPID))
			if !strings.Contains(workerID, trimmedStore) && !strings.Contains(upid, ":"+trimmedStore+":") {
				continue
			}
		}
		filtered = append(filtered, task)
	}

	sort.Slice(filtered, func(i, j int) bool {
		left := filtered[i].StartTime
		right := filtered[j].StartTime
		if left == right {
			return strings.ToLower(strings.TrimSpace(filtered[i].UPID)) > strings.ToLower(strings.TrimSpace(filtered[j].UPID))
		}
		return left > right
	})

	if limit > 0 && len(filtered) > limit {
		return filtered[:limit]
	}
	return filtered
}

func (d *Deps) ResolvePBSStoreList(ctx context.Context, asset assets.Asset, runtime *PBSRuntime) ([]string, []string) {
	store := PBSStoreFromAsset(asset)
	if store != "" {
		return []string{store}, nil
	}
	datastores, err := runtime.Client.ListDatastores(ctx)
	if err != nil {
		return nil, []string{"datastore listing unavailable: " + err.Error()}
	}
	names := make([]string, 0, len(datastores))
	for _, ds := range datastores {
		name := strings.TrimSpace(ds.Store)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func (d *Deps) HandlePBSAssetGroups(ctx context.Context, w http.ResponseWriter, asset assets.Asset, runtime *PBSRuntime) {
	warnings := make([]string, 0, 4)
	storeNames, storeWarnings := d.ResolvePBSStoreList(ctx, asset, runtime)
	warnings = append(warnings, storeWarnings...)

	result := make([]pbsDatastoreGroups, 0, len(storeNames))
	for _, storeName := range storeNames {
		groups, err := runtime.Client.ListDatastoreGroups(ctx, storeName)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s groups unavailable: %v", storeName, err))
			continue
		}
		entries := make([]PBSBackupGroupEntry, 0, len(groups))
		for _, group := range groups {
			entries = append(entries, PBSBackupGroupEntry{
				BackupType:  strings.TrimSpace(group.BackupType),
				BackupID:    strings.TrimSpace(group.BackupID),
				Owner:       strings.TrimSpace(group.Owner),
				Comment:     strings.TrimSpace(group.Comment),
				BackupCount: group.BackupCount,
				LastBackup:  group.LastBackup,
			})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].LastBackup < entries[j].LastBackup
		})
		result = append(result, pbsDatastoreGroups{Store: storeName, Groups: entries})
	}

	servicehttp.WriteJSON(w, http.StatusOK, PBSGroupsResponse{
		Datastores: result,
		Warnings:   DedupeNonEmptyWarnings(warnings),
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
	})
}

func (d *Deps) HandlePBSAssetSnapshots(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *PBSRuntime) {
	storeName := strings.TrimSpace(r.URL.Query().Get("store"))
	if storeName == "" {
		storeName = PBSStoreFromAsset(asset)
	}
	if storeName == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "store query parameter is required for server-kind assets")
		return
	}
	filterType := strings.TrimSpace(r.URL.Query().Get("type"))
	filterID := strings.TrimSpace(r.URL.Query().Get("id"))

	snapshots, err := runtime.Client.ListDatastoreSnapshots(ctx, storeName)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to list snapshots: "+err.Error())
		return
	}

	entries := make([]PBSSnapshotEntry, 0, len(snapshots))
	for _, snap := range snapshots {
		bt := strings.TrimSpace(snap.BackupType)
		bi := strings.TrimSpace(snap.BackupID)
		if filterType != "" && !strings.EqualFold(bt, filterType) {
			continue
		}
		if filterID != "" && !strings.EqualFold(bi, filterID) {
			continue
		}
		entries = append(entries, PBSSnapshotEntry{
			BackupType:   bt,
			BackupID:     bi,
			BackupTime:   snap.BackupTime,
			Size:         snap.Size,
			Protected:    snap.Protected,
			Owner:        strings.TrimSpace(snap.Owner),
			Comment:      strings.TrimSpace(snap.Comment),
			Verification: snap.Verification,
			Files:        snap.Files,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].BackupTime > entries[j].BackupTime
	})

	servicehttp.WriteJSON(w, http.StatusOK, PBSSnapshotsResponse{
		Store:     storeName,
		Snapshots: entries,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (d *Deps) HandlePBSAssetVerification(ctx context.Context, w http.ResponseWriter, asset assets.Asset, runtime *PBSRuntime) {
	warnings := make([]string, 0, 4)
	storeNames, storeWarnings := d.ResolvePBSStoreList(ctx, asset, runtime)
	warnings = append(warnings, storeWarnings...)

	result := make([]pbsDatastoreVerification, 0, len(storeNames))
	for _, storeName := range storeNames {
		snapshots, err := runtime.Client.ListDatastoreSnapshots(ctx, storeName)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s snapshots unavailable: %v", storeName, err))
			continue
		}
		entry := pbsDatastoreVerification{Store: storeName}
		for _, snap := range snapshots {
			if snap.Verification == nil {
				entry.UnverifiedCount++
				continue
			}
			state := strings.ToLower(strings.TrimSpace(snap.Verification.State))
			switch {
			case state == "ok":
				entry.VerifiedCount++
			case state == "failed" || strings.Contains(state, "error"):
				entry.FailedCount++
			default:
				entry.UnverifiedCount++
			}
		}

		node := PBSNodeFromAsset(asset)
		tasks, taskErr := runtime.Client.ListNodeTasks(ctx, node, 50)
		if taskErr == nil {
			for _, task := range tasks {
				if strings.EqualFold(strings.TrimSpace(task.WorkerType), "verificationjob") ||
					strings.EqualFold(strings.TrimSpace(task.WorkerType), "verify") {
					workerID := strings.ToLower(strings.TrimSpace(task.WorkerID))
					if strings.Contains(workerID, strings.ToLower(storeName)) || workerID == "" {
						if task.StartTime > entry.LastVerifyTime {
							entry.LastVerifyTime = task.StartTime
						}
						break
					}
				}
			}
		}

		if entry.FailedCount > 0 {
			entry.Status = "bad"
		} else if entry.UnverifiedCount > 0 {
			entry.Status = "warn"
		} else {
			entry.Status = "ok"
		}
		result = append(result, entry)
	}

	servicehttp.WriteJSON(w, http.StatusOK, PBSVerificationResponse{
		Datastores: result,
		Warnings:   DedupeNonEmptyWarnings(warnings),
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
	})
}

func latestPBSSnapshotEpoch(snapshots []pbsconnector.BackupSnapshot) int64 {
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
