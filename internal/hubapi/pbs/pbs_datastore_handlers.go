package pbs

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/assets"
	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (d *Deps) ResolvePBSStoreList(ctx context.Context, asset assets.Asset, runtime *PBSRuntime) ([]string, []string) {
	store := PBSStoreFromAsset(asset)
	if store != "" {
		return []string{store}, nil
	}
	datastores, err := runtime.Client.ListDatastores(ctx)
	if err != nil {
		return nil, []string{pbsWarning("datastore listing unavailable", err)}
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
		writePBSError(w, http.StatusBadGateway, "failed to list snapshots", err)
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
