package truenas

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

type TrueNASDisksResponse struct {
	AssetID      string           `json:"asset_id"`
	Disks        []map[string]any `json:"disks"`
	Temperatures map[string]any   `json:"temperatures,omitempty"`
	Warnings     []string         `json:"warnings,omitempty"`
	FetchedAt    string           `json:"fetched_at"`
}

type TrueNASDiskSMARTTestResponse struct {
	AssetID   string   `json:"asset_id"`
	DiskName  string   `json:"disk_name"`
	TestType  string   `json:"test_type"`
	Message   string   `json:"message"`
	Warnings  []string `json:"warnings,omitempty"`
	FetchedAt string   `json:"fetched_at"`
}

type TrueNASDiskSMARTDetailsResponse struct {
	AssetID   string           `json:"asset_id"`
	DiskName  string           `json:"disk_name"`
	Results   []map[string]any `json:"results"`
	Warnings  []string         `json:"warnings,omitempty"`
	FetchedAt string           `json:"fetched_at"`
}

func (d *Deps) HandleTrueNASDisks(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *TruenasRuntime, subParts []string) {
	// GET  /truenas/assets/{id}/disks                     → list disks with temperatures
	// POST /truenas/assets/{id}/disks/{name}/smart-test   → run SMART test
	// GET  /truenas/assets/{id}/disks/{name}/smart-details → get SMART test results

	if len(subParts) == 0 {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		warnings := make([]string, 0, 2)
		disks := make([]map[string]any, 0, 32)
		if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "disk.query", &disks); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to query disks: "+err.Error())
			return
		}
		temps := map[string]any{}
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "disk.temperatures", nil, &temps); err != nil {
			warnings = AppendTrueNASWarning(warnings, "disk temperatures unavailable: "+err.Error())
			temps = nil
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASDisksResponse{
			AssetID:      strings.TrimSpace(asset.ID),
			Disks:        disks,
			Temperatures: temps,
			Warnings:     warnings,
			FetchedAt:    time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	diskName := strings.TrimSpace(subParts[0])
	if diskName == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "disk name is required")
		return
	}

	if len(subParts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown disk action")
		return
	}

	diskAction := strings.TrimSpace(subParts[1])
	switch diskAction {
	case "smart-test":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		testType := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type")))
		if testType == "" {
			testType = "SHORT"
		}
		params := []any{[]any{map[string]any{"identifier": diskName, "type": testType}}}
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "smart.test.manual_test", params, nil); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to start SMART test: "+err.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASDiskSMARTTestResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			DiskName:  diskName,
			TestType:  testType,
			Message:   testType + " SMART test started on disk " + diskName,
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
	case "smart-details":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		allResults := make([]map[string]any, 0, 32)
		if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "smart.test.results", &allResults); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to query SMART results: "+err.Error())
			return
		}
		// Filter to the requested disk.
		filtered := make([]map[string]any, 0, len(allResults))
		for _, entry := range allResults {
			entryDisk := ""
			switch dv := entry["disk"].(type) {
			case map[string]any:
				entryDisk = strings.TrimSpace(shared.CollectorAnyString(dv["name"]))
			default:
				entryDisk = strings.TrimSpace(shared.CollectorAnyString(dv))
			}
			if strings.EqualFold(entryDisk, diskName) {
				filtered = append(filtered, entry)
			}
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASDiskSMARTDetailsResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			DiskName:  diskName,
			Results:   filtered,
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown disk action: "+diskAction)
	}
}
