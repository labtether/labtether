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

type TrueNASSnapshotsResponse struct {
	AssetID   string           `json:"asset_id"`
	Snapshots []map[string]any `json:"snapshots"`
	Warnings  []string         `json:"warnings,omitempty"`
	FetchedAt string           `json:"fetched_at"`
}

type TrueNASSnapshotActionResponse struct {
	AssetID    string         `json:"asset_id"`
	SnapshotID string         `json:"snapshot_id,omitempty"`
	Snapshot   map[string]any `json:"snapshot,omitempty"`
	Message    string         `json:"message,omitempty"`
	Warnings   []string       `json:"warnings,omitempty"`
	FetchedAt  string         `json:"fetched_at"`
}

func (d *Deps) HandleTrueNASSnapshots(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *TruenasRuntime, subParts []string) {
	// GET    /truenas/assets/{id}/snapshots                      → list snapshots
	// POST   /truenas/assets/{id}/snapshots                      → create snapshot (body: {dataset, name})
	// POST   /truenas/assets/{id}/snapshots/{name}/rollback      → rollback snapshot
	// POST   /truenas/assets/{id}/snapshots/{name}/clone         → clone snapshot (body: {dataset_dst})
	// DELETE /truenas/assets/{id}/snapshots/{name}               → delete snapshot

	if len(subParts) == 0 {
		switch r.Method {
		case http.MethodGet:
			snapshots := make([]map[string]any, 0, 64)
			warnings := make([]string, 0, 2)
			if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "zfs.snapshot.query", &snapshots); err != nil {
				warnings = AppendTrueNASWarning(warnings, "snapshots unavailable: "+err.Error())
				snapshots = nil
			}
			servicehttp.WriteJSON(w, http.StatusOK, TrueNASSnapshotsResponse{
				AssetID:   strings.TrimSpace(asset.ID),
				Snapshots: snapshots,
				Warnings:  warnings,
				FetchedAt: time.Now().UTC().Format(time.RFC3339),
			})
		case http.MethodPost:
			if !d.RequireAdminAuth(w, r) {
				return
			}
			var body struct {
				Dataset string `json:"dataset"`
				Name    string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
				return
			}
			body.Dataset = strings.TrimSpace(body.Dataset)
			body.Name = strings.TrimSpace(body.Name)
			if body.Dataset == "" || body.Name == "" {
				servicehttp.WriteError(w, http.StatusBadRequest, "dataset and name are required")
				return
			}
			var created map[string]any
			params := []any{map[string]any{"dataset": body.Dataset, "name": body.Name}}
			if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "zfs.snapshot.create", params, &created); err != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to create snapshot: "+err.Error())
				return
			}
			d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
			servicehttp.WriteJSON(w, http.StatusOK, TrueNASSnapshotActionResponse{
				AssetID:   strings.TrimSpace(asset.ID),
				Snapshot:  created,
				Message:   "snapshot " + body.Dataset + "@" + body.Name + " created",
				FetchedAt: time.Now().UTC().Format(time.RFC3339),
			})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	snapshotID := strings.TrimSpace(subParts[0])
	if snapshotID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "snapshot id is required")
		return
	}

	if len(subParts) == 1 {
		// DELETE /truenas/assets/{id}/snapshots/{name}
		if r.Method != http.MethodDelete {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "zfs.snapshot.delete", []any{snapshotID}, nil); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to delete snapshot: "+err.Error())
			return
		}
		d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASSnapshotActionResponse{
			AssetID:    strings.TrimSpace(asset.ID),
			SnapshotID: snapshotID,
			Message:    "snapshot " + snapshotID + " deleted",
			FetchedAt:  time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	snapAction := strings.TrimSpace(subParts[1])
	switch snapAction {
	case "rollback":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "zfs.snapshot.rollback", []any{snapshotID}, nil); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to rollback snapshot: "+err.Error())
			return
		}
		d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASSnapshotActionResponse{
			AssetID:    strings.TrimSpace(asset.ID),
			SnapshotID: snapshotID,
			Message:    "dataset rolled back to snapshot " + snapshotID,
			FetchedAt:  time.Now().UTC().Format(time.RFC3339),
		})
	case "clone":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		var body struct {
			DatasetDst string `json:"dataset_dst"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		body.DatasetDst = strings.TrimSpace(body.DatasetDst)
		if body.DatasetDst == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "dataset_dst is required")
			return
		}
		params := []any{map[string]any{"snapshot": snapshotID, "dataset_dst": body.DatasetDst}}
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "zfs.snapshot.clone", params, nil); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to clone snapshot: "+err.Error())
			return
		}
		d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASSnapshotActionResponse{
			AssetID:    strings.TrimSpace(asset.ID),
			SnapshotID: snapshotID,
			Message:    "snapshot " + snapshotID + " cloned to " + body.DatasetDst,
			FetchedAt:  time.Now().UTC().Format(time.RFC3339),
		})
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown snapshot action: "+snapAction)
	}
}
