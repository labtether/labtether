package proxmox

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	telemetrypkg "github.com/labtether/labtether/internal/telemetry"
)

type ProxmoxStoragePoolState struct {
	PoolName   string
	Asset      assets.Asset
	HasAsset   bool
	ZFSPool    proxmox.ZFSPool
	HasZFSPool bool
	Status     map[string]any
	Content    []proxmox.StorageContent
	DiskSeries []telemetrypkg.Point
}

func (d *Deps) LoadProxmoxStorageInsights(
	ctx context.Context,
	assetID string,
	target ProxmoxSessionTarget,
	runtime *ProxmoxRuntime,
	window time.Duration,
) (ProxmoxStorageInsightsResponse, error) {
	now := time.Now().UTC()
	resp := ProxmoxStorageInsightsResponse{
		GeneratedAt: now.Format(time.RFC3339),
		Window:      FormatStorageInsightsWindow(window),
		AssetID:     strings.TrimSpace(assetID),
		Node:        strings.TrimSpace(target.Node),
		Kind:        strings.TrimSpace(target.Kind),
		Pools:       make([]ProxmoxStorageInsightPool, 0),
		Events:      make([]ProxmoxStorageInsightEvent, 0),
		Warnings:    make([]string, 0),
	}
	if strings.TrimSpace(target.Node) == "" {
		return ProxmoxStorageInsightsResponse{}, ErrProxmoxMissingNode
	}

	addWarning := func(msg string) {
		resp.Warnings = append(resp.Warnings, msg)
	}

	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return ProxmoxStorageInsightsResponse{}, err
	}

	poolStates := BuildProxmoxStoragePoolStates(assetList, target, assetID)
	stateByName := make(map[string]int, len(poolStates))
	for i := range poolStates {
		stateByName[NormalizePoolKey(poolStates[i].PoolName)] = i
	}

	zfsPools, zfsErr := runtime.client.GetNodeZFSPools(ctx, target.Node)
	if zfsErr != nil {
		addWarning("zfs pools unavailable: " + zfsErr.Error())
	}
	for _, zfsPool := range zfsPools {
		name := strings.TrimSpace(zfsPool.Name)
		if name == "" {
			continue
		}
		key := NormalizePoolKey(name)
		if idx, ok := stateByName[key]; ok {
			poolStates[idx].ZFSPool = zfsPool
			poolStates[idx].HasZFSPool = true
			continue
		}
		poolStates = append(poolStates, ProxmoxStoragePoolState{
			PoolName:   name,
			ZFSPool:    zfsPool,
			HasZFSPool: true,
		})
		stateByName[key] = len(poolStates) - 1
	}

	windowStart := now.Add(-window)
	step := StorageInsightsStep(window)
	for i := range poolStates {
		state := &poolStates[i]

		if !state.HasZFSPool {
			status, statusErr := runtime.client.GetStorageStatus(ctx, target.Node, state.PoolName)
			if statusErr != nil {
				addWarning("storage status unavailable for " + state.PoolName + ": " + statusErr.Error())
			} else {
				state.Status = status
			}
		}

		content, contentErr := runtime.client.GetStorageContent(ctx, target.Node, state.PoolName)
		if contentErr != nil {
			addWarning("storage content unavailable for " + state.PoolName + ": " + contentErr.Error())
		} else {
			state.Content = content
		}

		if state.HasAsset {
			series, seriesErr := d.TelemetryStore.Series(state.Asset.ID, windowStart, now, step)
			if seriesErr != nil {
				addWarning("telemetry series unavailable for " + state.PoolName + ": " + seriesErr.Error())
			} else {
				state.DiskSeries = SelectDiskSeriesPoints(series)
			}
		}

		pool := BuildProxmoxStorageInsightPool(*state, now)
		resp.Pools = append(resp.Pools, pool)

		if !ProxmoxStorageHealthOK(pool.Health) {
			resp.Summary.DegradedPools++
		}
		if pool.UsedPercent != nil && *pool.UsedPercent >= 80 {
			resp.Summary.HotPools++
		}
		if pool.Forecast.DaysToFull != nil && *pool.Forecast.DaysToFull <= 30 {
			resp.Summary.PredictedFullLT30D++
		}
		if pool.Scrub.Overdue {
			resp.Summary.ScrubOverdue++
		}
		if pool.TelemetryStale {
			resp.Summary.StaleTelemetry++
		}
	}

	tasks, tasksErr := runtime.client.ListClusterTasks(ctx, target.Node, "", 240)
	if tasksErr != nil {
		addWarning("storage task timeline unavailable: " + tasksErr.Error())
	} else {
		resp.Events = BuildProxmoxStorageInsightEvents(tasks, poolStates, now, 24*time.Hour)
	}

	sort.Slice(resp.Pools, func(i, j int) bool {
		if resp.Pools[i].RiskScore != resp.Pools[j].RiskScore {
			return resp.Pools[i].RiskScore > resp.Pools[j].RiskScore
		}
		leftUsed := -1.0
		rightUsed := -1.0
		if resp.Pools[i].UsedPercent != nil {
			leftUsed = *resp.Pools[i].UsedPercent
		}
		if resp.Pools[j].UsedPercent != nil {
			rightUsed = *resp.Pools[j].UsedPercent
		}
		if leftUsed != rightUsed {
			return leftUsed > rightUsed
		}
		return resp.Pools[i].Name < resp.Pools[j].Name
	})

	if len(resp.Pools) == 0 {
		resp.Pools = nil
	}
	if len(resp.Events) == 0 {
		resp.Events = nil
	}
	if len(resp.Warnings) == 0 {
		resp.Warnings = nil
	}
	return resp, nil
}

func BuildProxmoxStoragePoolStates(assetList []assets.Asset, target ProxmoxSessionTarget, requestedAssetID string) []ProxmoxStoragePoolState {
	stateByKey := make(map[string]ProxmoxStoragePoolState)

	for _, asset := range assetList {
		if !strings.EqualFold(strings.TrimSpace(asset.Source), "proxmox") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(asset.Type), "storage-pool") {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(target.Kind), "storage") {
			if strings.TrimSpace(requestedAssetID) != "" && asset.ID != strings.TrimSpace(requestedAssetID) {
				continue
			}
		} else if !ProxmoxStorageAssetBelongsToNode(asset, target.Node) {
			continue
		}

		poolName := ProxmoxStoragePoolNameFromAsset(asset)
		if poolName == "" {
			continue
		}
		key := NormalizePoolKey(poolName)
		state := stateByKey[key]
		state.PoolName = poolName
		state.Asset = asset
		state.HasAsset = true
		stateByKey[key] = state
	}

	if strings.EqualFold(strings.TrimSpace(target.Kind), "storage") && strings.TrimSpace(target.StorageName) != "" {
		key := NormalizePoolKey(target.StorageName)
		state := stateByKey[key]
		if strings.TrimSpace(state.PoolName) == "" {
			state.PoolName = strings.TrimSpace(target.StorageName)
		}
		stateByKey[key] = state
	}

	out := make([]ProxmoxStoragePoolState, 0, len(stateByKey))
	for _, state := range stateByKey {
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PoolName < out[j].PoolName })
	return out
}

func ProxmoxStorageAssetBelongsToNode(asset assets.Asset, node string) bool {
	node = strings.TrimSpace(strings.ToLower(node))
	if node == "" {
		return false
	}

	assetNode := strings.TrimSpace(strings.ToLower(asset.Metadata["node"]))
	if assetNode != "" && assetNode == node {
		return true
	}

	storageID := strings.TrimSpace(strings.ToLower(asset.Metadata["storage_id"]))
	if storageID == "" {
		return false
	}
	parts := strings.Split(storageID, "/")
	if len(parts) >= 3 && parts[0] == "storage" {
		return strings.TrimSpace(parts[1]) == node
	}
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[0]) == node
	}
	return strings.HasPrefix(storageID, node+"/")
}

func ProxmoxStoragePoolNameFromAsset(asset assets.Asset) string {
	storageID := strings.TrimSpace(asset.Metadata["storage_id"])
	if storageID != "" {
		parts := strings.Split(storageID, "/")
		candidate := strings.TrimSpace(parts[len(parts)-1])
		if candidate != "" {
			return candidate
		}
	}

	name := strings.TrimSpace(asset.Name)
	if name == "" {
		return ""
	}
	parts := strings.Split(name, "/")
	candidate := strings.TrimSpace(parts[len(parts)-1])
	if candidate != "" {
		return candidate
	}
	return name
}

func NormalizePoolKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func SelectDiskSeriesPoints(series []telemetrypkg.Series) []telemetrypkg.Point {
	for _, entry := range series {
		if entry.Metric == telemetrypkg.MetricDiskUsedPercent {
			return entry.Points
		}
	}
	return nil
}

func BuildProxmoxStorageInsightPool(state ProxmoxStoragePoolState, now time.Time) ProxmoxStorageInsightPool {
	pool := ProxmoxStorageInsightPool{
		Name: state.PoolName,
		Forecast: ProxmoxStorageForecast{
			Confidence: "low",
		},
		Scrub: ProxmoxStorageScrub{
			Overdue: false,
		},
		IO: ProxmoxStorageIO{
			ReadLatencyMSP95:  nil,
			WriteLatencyMSP95: nil,
		},
		Errors: ProxmoxStorageErrors{
			Read:     0,
			Write:    0,
			Checksum: 0,
		},
		Snapshots:          ProxmoxStorageSnapshots{},
		DependentWorkloads: ProxmoxStorageDependentWorkloads{},
		Reasons:            make([]string, 0, 4),
	}

	health := strings.ToUpper(strings.TrimSpace(state.Asset.Metadata["status"]))
	if state.HasZFSPool {
		health = strings.ToUpper(strings.TrimSpace(state.ZFSPool.Health))
	}
	if health == "" {
		health = "UNKNOWN"
	}
	pool.Health = health

	var sizeBytes *int64
	var usedBytes *int64
	var freeBytes *int64
	var usedPercent *float64

	if state.HasZFSPool {
		if state.ZFSPool.Frag >= 0 {
			pool.FragPercent = IntPtr(state.ZFSPool.Frag)
		}
		if state.ZFSPool.Dedup > 0 {
			pool.DedupRatio = Float64Ptr(state.ZFSPool.Dedup)
		}
		if state.ZFSPool.Size > 0 {
			sizeBytes = Int64Ptr(state.ZFSPool.Size)
		}
		if state.ZFSPool.Alloc >= 0 {
			usedBytes = Int64Ptr(state.ZFSPool.Alloc)
		}
		if state.ZFSPool.Free >= 0 {
			freeBytes = Int64Ptr(state.ZFSPool.Free)
		}
		if state.ZFSPool.Size > 0 && state.ZFSPool.Alloc >= 0 {
			usedPercent = Float64Ptr(ClampPercent((float64(state.ZFSPool.Alloc) / float64(state.ZFSPool.Size)) * 100))
		}
	}

	if state.Status != nil {
		if total, ok := ParseAnyInt64(state.Status["total"]); ok && total > 0 {
			sizeBytes = Int64Ptr(total)
		}
		if used, ok := ParseAnyInt64(state.Status["used"]); ok && used >= 0 {
			usedBytes = Int64Ptr(used)
		}
		if avail, ok := ParseAnyInt64(state.Status["avail"]); ok && avail >= 0 {
			freeBytes = Int64Ptr(avail)
		}
		if maxDisk, ok := ParseAnyInt64(state.Status["maxdisk"]); ok && maxDisk > 0 && sizeBytes == nil {
			sizeBytes = Int64Ptr(maxDisk)
		}
		if disk, ok := ParseAnyInt64(state.Status["disk"]); ok && disk >= 0 && usedBytes == nil {
			usedBytes = Int64Ptr(disk)
		}
		if lastScrubTS, ok := ParseAnyInt64(state.Status["last_scrub"]); ok && lastScrubTS > 0 {
			pool.Scrub.LastCompletedAt = time.Unix(lastScrubTS, 0).UTC().Format(time.RFC3339)
		}
		if scrubOverdueRaw, ok := ParseAnyInt64(state.Status["scrub_overdue"]); ok && scrubOverdueRaw > 0 {
			pool.Scrub.Overdue = true
		}
	}

	if usedPercent == nil && sizeBytes != nil && usedBytes != nil && *sizeBytes > 0 {
		usedPercent = Float64Ptr(ClampPercent((float64(*usedBytes) / float64(*sizeBytes)) * 100))
	}

	if state.HasAsset {
		if usedPercent == nil {
			if value, ok := ParseMetadataFloat(state.Asset.Metadata, "disk_used_percent", "disk_percent"); ok {
				usedPercent = Float64Ptr(ClampPercent(value))
			}
		}

		scrubOverdue := strings.ToLower(strings.TrimSpace(state.Asset.Metadata["scrub_overdue"]))
		if scrubOverdue == "1" || scrubOverdue == "true" || scrubOverdue == "yes" {
			pool.Scrub.Overdue = true
		}
		if pool.Scrub.LastCompletedAt == "" {
			if lastScrubTS, ok := ParseAnyInt64(state.Asset.Metadata["last_scrub"]); ok && lastScrubTS > 0 {
				pool.Scrub.LastCompletedAt = time.Unix(lastScrubTS, 0).UTC().Format(time.RFC3339)
			}
		}
	}

	growthPerDay, confidence, latestTS := AnalyzeDiskGrowth(state.DiskSeries)
	pool.Forecast.Confidence = confidence
	if usedPercent == nil && len(state.DiskSeries) > 0 {
		last := state.DiskSeries[len(state.DiskSeries)-1].Value
		usedPercent = Float64Ptr(ClampPercent(last))
	}

	if usedPercent != nil && growthPerDay > 0 {
		daysTo80 := (80 - *usedPercent) / growthPerDay
		if daysTo80 < 0 {
			daysTo80 = 0
		}
		daysToFull := (100 - *usedPercent) / growthPerDay
		pool.Forecast.DaysTo80 = Float64Ptr(RoundToSingleDecimal(daysTo80))
		pool.Forecast.DaysToFull = Float64Ptr(RoundToSingleDecimal(daysToFull))
	}

	if sizeBytes != nil && growthPerDay != 0 {
		growthBytes := int64(math.Round((growthPerDay * 7 / 100) * float64(*sizeBytes)))
		pool.GrowthBytes7D = Int64Ptr(growthBytes)
	}

	vmIDs := make(map[int]struct{})
	ctIDs := make(map[int]struct{})
	for _, item := range state.Content {
		content := strings.ToLower(strings.TrimSpace(item.Content))
		volID := strings.ToLower(strings.TrimSpace(item.VolID))

		if content == "backup" || strings.Contains(volID, "snapshot") {
			pool.Snapshots.Count++
			if item.Size > 0 {
				pool.Snapshots.Bytes += item.Size
			}
		}

		if item.VMID <= 0 {
			continue
		}
		if content == "rootdir" || strings.Contains(volID, "subvol-") || strings.Contains(volID, "lxc") {
			ctIDs[item.VMID] = struct{}{}
			continue
		}
		vmIDs[item.VMID] = struct{}{}
	}
	pool.DependentWorkloads.VMCount = len(vmIDs)
	pool.DependentWorkloads.CTCount = len(ctIDs)
	if len(vmIDs) > 0 {
		pool.DependentWorkloads.VMIDs = make([]int, 0, len(vmIDs))
		for vmid := range vmIDs {
			pool.DependentWorkloads.VMIDs = append(pool.DependentWorkloads.VMIDs, vmid)
		}
		sort.Ints(pool.DependentWorkloads.VMIDs)
	}
	if len(ctIDs) > 0 {
		pool.DependentWorkloads.CTIDs = make([]int, 0, len(ctIDs))
		for vmid := range ctIDs {
			pool.DependentWorkloads.CTIDs = append(pool.DependentWorkloads.CTIDs, vmid)
		}
		sort.Ints(pool.DependentWorkloads.CTIDs)
	}

	pool.SizeBytes = sizeBytes
	pool.UsedBytes = usedBytes
	pool.FreeBytes = freeBytes
	pool.UsedPercent = usedPercent

	if latestTS <= 0 {
		pool.TelemetryStale = true
	} else {
		age := now.Sub(time.Unix(latestTS, 0).UTC())
		pool.TelemetryStale = age > 5*time.Minute
	}

	pool.RiskScore, pool.RiskState, pool.Reasons = ComputeStorageRisk(pool)
	return pool
}
