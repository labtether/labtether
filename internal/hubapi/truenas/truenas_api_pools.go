package truenas

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

type TrueNASPoolsResponse struct {
	AssetID   string           `json:"asset_id"`
	Pools     []map[string]any `json:"pools"`
	Warnings  []string         `json:"warnings,omitempty"`
	FetchedAt string           `json:"fetched_at"`
}

type TrueNASPoolActionResponse struct {
	AssetID   string   `json:"asset_id"`
	PoolName  string   `json:"pool_name"`
	Action    string   `json:"action"`
	Message   string   `json:"message"`
	Status    string   `json:"status"`
	JobID     int64    `json:"job_id"`
	Warnings  []string `json:"warnings,omitempty"`
	FetchedAt string   `json:"fetched_at"`
}

func (d *Deps) HandleTrueNASPools(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *TruenasRuntime, subParts []string) {
	// GET /truenas/assets/{id}/pools            → list pools
	// POST /truenas/assets/{id}/pools/{name}/scrub → run scrub
	if len(subParts) == 0 {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		pools := make([]map[string]any, 0, 8)
		warnings := make([]string, 0, 2)
		if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "pool.query", &pools); err != nil {
			warnings = AppendTrueNASWarning(warnings, trueNASWarning("pools unavailable", err))
			pools = nil
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASPoolsResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			Pools:     pools,
			Warnings:  warnings,
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	// subParts[0] = pool name, subParts[1] = action
	poolName := strings.TrimSpace(subParts[0])
	if poolName == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "pool name is required")
		return
	}
	if len(subParts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown pool action")
		return
	}

	poolAction := strings.TrimSpace(subParts[1])
	switch poolAction {
	case "scrub":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if !d.enforceMutation(w, asset, runtime, "pool.scrub", map[string]string{"pool_name": poolName}) {
			return
		}
		jobID, err := runtime.Client.StartPoolScrub(ctx, poolName)
		if err != nil {
			writeTrueNASMutationError(w, http.StatusBadGateway, "failed to start pool scrub", err)
			return
		}
		d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASPoolActionResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			PoolName:  poolName,
			Action:    "scrub",
			Message:   "scrub request accepted for pool " + poolName,
			Status:    "accepted",
			JobID:     jobID,
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown pool action: "+poolAction)
	}
}
