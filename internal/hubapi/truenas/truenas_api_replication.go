package truenas

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

type TrueNASReplicationResponse struct {
	AssetID      string           `json:"asset_id"`
	Replications []map[string]any `json:"replications"`
	Warnings     []string         `json:"warnings,omitempty"`
	FetchedAt    string           `json:"fetched_at"`
}

type TrueNASReplicationActionResponse struct {
	AssetID       string   `json:"asset_id"`
	ReplicationID int      `json:"replication_id"`
	Action        string   `json:"action"`
	Message       string   `json:"message"`
	Warnings      []string `json:"warnings,omitempty"`
	FetchedAt     string   `json:"fetched_at"`
}

func (d *Deps) HandleTrueNASReplication(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *TruenasRuntime, subParts []string) {
	// GET  /truenas/assets/{id}/replication              → list replication tasks
	// POST /truenas/assets/{id}/replication/{id}/run     → run replication task

	if len(subParts) == 0 {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		replications := make([]map[string]any, 0, 16)
		warnings := make([]string, 0, 2)
		if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "replication.query", &replications); err != nil {
			warnings = AppendTrueNASWarning(warnings, "replication tasks unavailable: "+err.Error())
			replications = nil
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASReplicationResponse{
			AssetID:      strings.TrimSpace(asset.ID),
			Replications: replications,
			Warnings:     warnings,
			FetchedAt:    time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	idStr := strings.TrimSpace(subParts[0])
	if idStr == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "replication id is required")
		return
	}
	replID, err := strconv.Atoi(idStr)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "replication id must be an integer")
		return
	}

	if len(subParts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown replication action")
		return
	}

	replAction := strings.TrimSpace(subParts[1])
	switch replAction {
	case "run":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "replication.run", []any{replID}, nil); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to run replication task: "+err.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASReplicationActionResponse{
			AssetID:       strings.TrimSpace(asset.ID),
			ReplicationID: replID,
			Action:        "run",
			Message:       "replication task " + idStr + " started",
			FetchedAt:     time.Now().UTC().Format(time.RFC3339),
		})
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown replication action: "+replAction)
	}
}
