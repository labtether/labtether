package truenas

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"sort"
	"strings"
	"time"

	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/servicehttp"
)

var (
	ErrTrueNASAssetNotFound = errors.New("truenas asset not found")
	ErrAssetNotTrueNAS      = errors.New("asset is not truenas-backed")
)

const TrueNASMethodCallRetryAttempts = 5

var TrueNASMethodCallRetryBackoff = 200 * time.Millisecond

type TrueNASAssetEventsResponse struct {
	AssetID   string       `json:"asset_id"`
	Events    []logs.Event `json:"events"`
	Window    string       `json:"window"`
	FetchedAt string       `json:"fetched_at"`
}

type TrueNASAssetSMARTResponse struct {
	AssetID     string              `json:"asset_id"`
	CollectorID string              `json:"collector_id,omitempty"`
	Hostname    string              `json:"hostname,omitempty"`
	Summary     TrueNASSMARTSummary `json:"summary"`
	Disks       []TrueNASDiskHealth `json:"disks"`
	Warnings    []string            `json:"warnings,omitempty"`
	FetchedAt   string              `json:"fetched_at"`
}

type TrueNASSMARTSummary struct {
	Total    int `json:"total"`
	Healthy  int `json:"healthy"`
	Warning  int `json:"warning"`
	Critical int `json:"critical"`
	Unknown  int `json:"unknown"`
}

type TrueNASDiskHealth struct {
	Name               string   `json:"name"`
	Serial             string   `json:"serial,omitempty"`
	Model              string   `json:"model,omitempty"`
	Type               string   `json:"type,omitempty"`
	SizeBytes          *int64   `json:"size_bytes,omitempty"`
	TemperatureCelsius *float64 `json:"temperature_celsius,omitempty"`
	Status             string   `json:"status"`
	SmartEnabled       *bool    `json:"smart_enabled,omitempty"`
	SmartHealth        string   `json:"smart_health,omitempty"`
	LastTestType       string   `json:"last_test_type,omitempty"`
	LastTestStatus     string   `json:"last_test_status,omitempty"`
	LastTestAt         string   `json:"last_test_at,omitempty"`
}

type TrueNASFilesystemResponse struct {
	AssetID    string                   `json:"asset_id"`
	Path       string                   `json:"path"`
	ParentPath string                   `json:"parent_path,omitempty"`
	Entries    []TrueNASFilesystemEntry `json:"entries"`
	Warnings   []string                 `json:"warnings,omitempty"`
	FetchedAt  string                   `json:"fetched_at"`
}

type TrueNASFilesystemEntry struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Type         string `json:"type"`
	SizeBytes    *int64 `json:"size_bytes,omitempty"`
	Mode         string `json:"mode,omitempty"`
	ModifiedAt   string `json:"modified_at,omitempty"`
	User         string `json:"user,omitempty"`
	Group        string `json:"group,omitempty"`
	IsDirectory  bool   `json:"is_directory"`
	IsSymbolic   bool   `json:"is_symbolic,omitempty"`
	SymbolicLink string `json:"symbolic_link,omitempty"`
}

func (d *Deps) HandleTrueNASAssets(w http.ResponseWriter, r *http.Request) {
	pathValue := strings.TrimPrefix(r.URL.Path, "/truenas/assets/")
	if pathValue == r.URL.Path || pathValue == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "truenas asset path not found")
		return
	}
	parts := strings.Split(pathValue, "/")
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "truenas asset path not found")
		return
	}
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown truenas asset action")
		return
	}

	action := strings.TrimSpace(parts[1])
	subParts := parts[2:]

	switch action {
	case "capabilities":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		asset, runtime, err := d.ResolveTrueNASAssetRuntime(assetID)
		if err != nil {
			WriteTrueNASResolveError(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		d.HandleTrueNASCapabilities(ctx, w, asset, runtime)
	case "events":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.HandleTrueNASAssetEvents(w, r, assetID)
	case "smart":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.HandleTrueNASAssetSMART(w, r, assetID)
	case "filesystem":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.HandleTrueNASAssetFilesystem(w, r, assetID)
	case "overview", "pools", "datasets", "shares", "disks", "services", "snapshots", "replication", "vms":
		asset, runtime, err := d.ResolveTrueNASAssetRuntime(assetID)
		if err != nil {
			WriteTrueNASResolveError(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		switch action {
		case "overview":
			d.HandleTrueNASOverview(ctx, w, r, asset, runtime)
		case "pools":
			d.HandleTrueNASPools(ctx, w, r, asset, runtime, subParts)
		case "datasets":
			d.HandleTrueNASDatasets(ctx, w, r, asset, runtime, subParts)
		case "shares":
			d.HandleTrueNASShares(ctx, w, r, asset, runtime, subParts)
		case "disks":
			d.HandleTrueNASDisks(ctx, w, r, asset, runtime, subParts)
		case "services":
			d.HandleTrueNASServices(ctx, w, r, asset, runtime, subParts)
		case "snapshots":
			d.HandleTrueNASSnapshots(ctx, w, r, asset, runtime, subParts)
		case "replication":
			d.HandleTrueNASReplication(ctx, w, r, asset, runtime, subParts)
		case "vms":
			d.HandleTrueNASVMs(ctx, w, r, asset, runtime, subParts)
		}
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown truenas asset action")
	}
}

func (d *Deps) HandleTrueNASAssetEvents(w http.ResponseWriter, r *http.Request, assetID string) {
	asset, err := d.ResolveTrueNASAsset(assetID)
	if err != nil {
		WriteTrueNASResolveError(w, err)
		return
	}
	if d.LogStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "log store unavailable")
		return
	}

	limit := 100
	if parsed, ok := shared.ParsePositiveInt(r.URL.Query().Get("limit")); ok {
		limit = parsed
	}
	if limit > 500 {
		limit = 500
	}
	window := ParseTrueNASEventsWindow(r.URL.Query().Get("window"))
	now := time.Now().UTC()
	events, err := d.LogStore.QueryEvents(logs.QueryRequest{
		AssetID:       strings.TrimSpace(asset.ID),
		Source:        "truenas",
		From:          now.Add(-window),
		To:            now,
		Limit:         limit,
		ExcludeFields: true,
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to load truenas events: "+err.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, TrueNASAssetEventsResponse{
		AssetID:   strings.TrimSpace(asset.ID),
		Events:    events,
		Window:    shared.FormatStorageInsightsWindow(window),
		FetchedAt: now.Format(time.RFC3339),
	})
}

func (d *Deps) HandleTrueNASAssetSMART(w http.ResponseWriter, r *http.Request, assetID string) {
	asset, runtime, err := d.ResolveTrueNASAssetRuntime(assetID)
	if err != nil {
		WriteTrueNASResolveError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	disks := make([]map[string]any, 0, 32)
	if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "disk.query", &disks); err != nil {
		if cached, ok := d.GetCachedTrueNASSMART(asset.ID, runtime.CollectorID); ok {
			cached.AssetID = strings.TrimSpace(asset.ID)
			cached.CollectorID = strings.TrimSpace(runtime.CollectorID)
			cached.Hostname = strings.TrimSpace(asset.Metadata["hostname"])
			cached.Warnings = AppendTrueNASWarning(cached.Warnings, StaleTrueNASReadWarning("live disk health unavailable: "+err.Error(), cached.FetchedAt))
			servicehttp.WriteJSON(w, http.StatusOK, cached)
			return
		}
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to query truenas disks: "+err.Error())
		return
	}

	diskTemps := map[string]any{}
	warnings := make([]string, 0, 2)
	if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "disk.temperatures", nil, &diskTemps); err != nil {
		warnings = append(warnings, "disk temperatures unavailable: "+err.Error())
		diskTemps = map[string]any{}
	}

	smartResults := make([]map[string]any, 0, 32)
	if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "smart.test.results", &smartResults); err != nil {
		if !tnconnector.IsMethodNotFound(err) {
			warnings = append(warnings, "SMART test history unavailable: "+err.Error())
		}
		smartResults = nil
	}
	smartByDisk := LatestSmartResultsByDisk(smartResults)

	diskViews := make([]TrueNASDiskHealth, 0, len(disks))
	summary := TrueNASSMARTSummary{}
	for _, disk := range disks {
		name := strings.TrimSpace(shared.CollectorAnyString(disk["name"]))
		if name == "" {
			continue
		}

		view := TrueNASDiskHealth{
			Name:   name,
			Serial: strings.TrimSpace(shared.CollectorAnyString(disk["serial"])),
			Model:  strings.TrimSpace(shared.CollectorAnyString(disk["model"])),
			Type:   strings.TrimSpace(shared.CollectorAnyString(disk["type"])),
			Status: "unknown",
		}
		if sizeBytes, ok := shared.ParseAnyInt64(disk["size"]); ok && sizeBytes > 0 {
			view.SizeBytes = &sizeBytes
		}
		if tempValue := shared.AnyToFloat64(diskTemps[name]); tempValue > 0 {
			view.TemperatureCelsius = &tempValue
		}

		if smartEnabled, ok := shared.ParseAnyBoolLoose(disk["smart_enabled"]); ok {
			view.SmartEnabled = &smartEnabled
		}
		if view.SmartEnabled == nil {
			if toggled, ok := shared.ParseAnyBoolLoose(disk["togglesmart"]); ok {
				view.SmartEnabled = &toggled
			}
		}
		view.SmartHealth = strings.TrimSpace(shared.CollectorAnyString(disk["smart_status"]))
		if view.SmartHealth == "" {
			view.SmartHealth = strings.TrimSpace(shared.CollectorAnyString(disk["status"]))
		}

		if smartResult, ok := smartByDisk[name]; ok {
			view.LastTestType = strings.TrimSpace(shared.CollectorAnyString(smartResult["type"]))
			view.LastTestStatus = strings.TrimSpace(shared.CollectorAnyString(smartResult["status"]))
			resultTime := shared.CollectorAnyTime(smartResult["created_at"])
			if resultTime.IsZero() {
				resultTime = shared.CollectorAnyTime(smartResult["end_time"])
			}
			if !resultTime.IsZero() {
				view.LastTestAt = resultTime.UTC().Format(time.RFC3339)
			}
		}

		view.Status = DeriveTrueNASDiskHealthStatus(view)
		summary.Total++
		switch view.Status {
		case "critical":
			summary.Critical++
		case "warning":
			summary.Warning++
		case "healthy":
			summary.Healthy++
		default:
			summary.Unknown++
		}

		diskViews = append(diskViews, view)
	}

	sort.SliceStable(diskViews, func(i, j int) bool {
		if diskViews[i].Status == diskViews[j].Status {
			return diskViews[i].Name < diskViews[j].Name
		}
		return TrueNASDiskHealthSeverity(diskViews[i].Status) > TrueNASDiskHealthSeverity(diskViews[j].Status)
	})

	response := TrueNASAssetSMARTResponse{
		AssetID:     strings.TrimSpace(asset.ID),
		CollectorID: strings.TrimSpace(runtime.CollectorID),
		Hostname:    strings.TrimSpace(asset.Metadata["hostname"]),
		Summary:     summary,
		Disks:       diskViews,
		Warnings:    warnings,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	d.SetCachedTrueNASSMART(asset.ID, runtime.CollectorID, response)
	servicehttp.WriteJSON(w, http.StatusOK, response)
}

func (d *Deps) HandleTrueNASAssetFilesystem(w http.ResponseWriter, r *http.Request, assetID string) {
	asset, runtime, err := d.ResolveTrueNASAssetRuntime(assetID)
	if err != nil {
		WriteTrueNASResolveError(w, err)
		return
	}

	requestPath := NormalizeTrueNASFilesystemPath(r.URL.Query().Get("path"))
	limit := 1000
	if parsed, ok := shared.ParsePositiveInt(r.URL.Query().Get("limit")); ok {
		limit = parsed
	}
	if limit > 5000 {
		limit = 5000
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	entries, err := CallTrueNASListDirWithRetries(ctx, runtime.Client, requestPath)
	if err != nil {
		if cached, ok := d.GetCachedTrueNASFilesystem(asset.ID, runtime.CollectorID, requestPath); ok {
			cached.AssetID = strings.TrimSpace(asset.ID)
			cached.Path = requestPath
			cached.ParentPath = ParentTrueNASFilesystemPath(requestPath)
			cached.Warnings = AppendTrueNASWarning(cached.Warnings, StaleTrueNASReadWarning("live filesystem browse unavailable: "+err.Error(), cached.FetchedAt))
			servicehttp.WriteJSON(w, http.StatusOK, cached)
			return
		}
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to browse filesystem: "+err.Error())
		return
	}

	mapped := make([]TrueNASFilesystemEntry, 0, len(entries))
	for _, entry := range entries {
		mapped = append(mapped, MapTrueNASFilesystemEntry(entry, requestPath))
	}
	sort.SliceStable(mapped, func(i, j int) bool {
		if mapped[i].IsDirectory != mapped[j].IsDirectory {
			return mapped[i].IsDirectory
		}
		return strings.ToLower(mapped[i].Name) < strings.ToLower(mapped[j].Name)
	})
	if len(mapped) > limit {
		mapped = mapped[:limit]
	}

	response := TrueNASFilesystemResponse{
		AssetID:    strings.TrimSpace(asset.ID),
		Path:       requestPath,
		ParentPath: ParentTrueNASFilesystemPath(requestPath),
		Entries:    mapped,
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	d.SetCachedTrueNASFilesystem(asset.ID, runtime.CollectorID, requestPath, response)
	servicehttp.WriteJSON(w, http.StatusOK, response)
}
