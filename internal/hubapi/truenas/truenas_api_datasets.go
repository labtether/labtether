package truenas

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/shared"
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
				warnings = AppendTrueNASWarning(warnings, trueNASWarning("datasets unavailable", err))
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
			if err := shared.DecodeJSONBody(w, r, &params); err != nil {
				writeTrueNASError(w, http.StatusBadRequest, "invalid request body", err)
				return
			}
			if !d.enforceMutation(w, asset, runtime, "dataset.create", map[string]string{"dataset": shared.CollectorAnyString(params["name"])}) {
				return
			}
			var created map[string]any
			if err := runtime.Client.CallMutation(ctx, "pool.dataset.create", []any{params}, &created); err != nil {
				writeTrueNASMutationError(w, http.StatusBadGateway, "failed to create dataset", err)
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

	// The router decodes one provider ID after splitting escaped segments.
	if len(subParts) != 1 {
		servicehttp.WriteError(w, http.StatusBadRequest, "dataset id must be one encoded path segment")
		return
	}
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
		if err := shared.DecodeJSONBody(w, r, &params); err != nil {
			writeTrueNASError(w, http.StatusBadRequest, "invalid request body", err)
			return
		}
		if !d.enforceMutation(w, asset, runtime, "dataset.update", map[string]string{"dataset": datasetID}) {
			return
		}
		var updated map[string]any
		if err := runtime.Client.CallMutation(ctx, "pool.dataset.update", []any{datasetID, params}, &updated); err != nil {
			writeTrueNASMutationError(w, http.StatusBadGateway, "failed to update dataset", err)
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
		if !d.enforceMutation(w, asset, runtime, "dataset.delete", map[string]string{"dataset": datasetID}) {
			return
		}
		if err := runtime.Client.CallMutation(ctx, "pool.dataset.delete", []any{datasetID}, nil); err != nil {
			writeTrueNASMutationError(w, http.StatusBadGateway, "failed to delete dataset", err)
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
