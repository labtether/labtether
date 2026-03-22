package truenas

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

type TrueNASDatasetsResponse struct {
	AssetID   string           `json:"asset_id"`
	Datasets  []map[string]any `json:"datasets"`
	Warnings  []string         `json:"warnings,omitempty"`
	FetchedAt string           `json:"fetched_at"`
}

type TrueNASDatasetActionResponse struct {
	AssetID   string         `json:"asset_id"`
	Dataset   map[string]any `json:"dataset,omitempty"`
	Message   string         `json:"message,omitempty"`
	Warnings  []string       `json:"warnings,omitempty"`
	FetchedAt string         `json:"fetched_at"`
}

func (d *Deps) HandleTrueNASDatasets(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *TruenasRuntime, subParts []string) {
	// GET /truenas/assets/{id}/datasets               → list datasets
	// POST /truenas/assets/{id}/datasets              → create dataset (body: dataset params)
	// PUT  /truenas/assets/{id}/datasets/{encoded-id} → update dataset
	// DELETE /truenas/assets/{id}/datasets/{encoded-id} → delete dataset

	if len(subParts) == 0 {
		switch r.Method {
		case http.MethodGet:
			datasets := make([]map[string]any, 0, 32)
			warnings := make([]string, 0, 2)
			if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "pool.dataset.query", &datasets); err != nil {
				warnings = AppendTrueNASWarning(warnings, "datasets unavailable: "+err.Error())
				datasets = nil
			}
			servicehttp.WriteJSON(w, http.StatusOK, TrueNASDatasetsResponse{
				AssetID:   strings.TrimSpace(asset.ID),
				Datasets:  datasets,
				Warnings:  warnings,
				FetchedAt: time.Now().UTC().Format(time.RFC3339),
			})
		case http.MethodPost:
			if !d.RequireAdminAuth(w, r) {
				return
			}
			var params map[string]any
			if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
				return
			}
			var created map[string]any
			if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "pool.dataset.create", []any{params}, &created); err != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to create dataset: "+err.Error())
				return
			}
			d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
			servicehttp.WriteJSON(w, http.StatusOK, TrueNASDatasetActionResponse{
				AssetID:   strings.TrimSpace(asset.ID),
				Dataset:   created,
				Message:   "dataset created",
				FetchedAt: time.Now().UTC().Format(time.RFC3339),
			})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// subParts[0] is the dataset ID (may contain URL-encoded slashes but passed as-is here).
	datasetID := strings.TrimSpace(subParts[0])
	if datasetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "dataset id is required")
		return
	}

	switch r.Method {
	case http.MethodPut:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		var params map[string]any
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		var updated map[string]any
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "pool.dataset.update", []any{datasetID, params}, &updated); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to update dataset: "+err.Error())
			return
		}
		d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASDatasetActionResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			Dataset:   updated,
			Message:   "dataset updated",
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
	case http.MethodDelete:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "pool.dataset.delete", []any{datasetID}, nil); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to delete dataset: "+err.Error())
			return
		}
		d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASDatasetActionResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			Message:   "dataset deleted",
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
