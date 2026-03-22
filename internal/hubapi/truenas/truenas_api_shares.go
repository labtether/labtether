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

type TrueNASSharesResponse struct {
	AssetID   string           `json:"asset_id"`
	SMB       []map[string]any `json:"smb"`
	NFS       []map[string]any `json:"nfs"`
	Warnings  []string         `json:"warnings,omitempty"`
	FetchedAt string           `json:"fetched_at"`
}

type TrueNASShareActionResponse struct {
	AssetID   string         `json:"asset_id"`
	ShareType string         `json:"share_type"`
	Share     map[string]any `json:"share,omitempty"`
	Message   string         `json:"message,omitempty"`
	Warnings  []string       `json:"warnings,omitempty"`
	FetchedAt string         `json:"fetched_at"`
}

func (d *Deps) HandleTrueNASShares(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *TruenasRuntime, subParts []string) {
	// GET /truenas/assets/{id}/shares                      → list SMB + NFS shares
	// POST /truenas/assets/{id}/shares/{smb|nfs}           → create share
	// PUT  /truenas/assets/{id}/shares/{smb|nfs}/{shareID} → update share
	// DELETE /truenas/assets/{id}/shares/{smb|nfs}/{shareID} → delete share

	if len(subParts) == 0 {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		warnings := make([]string, 0, 2)
		smbShares := make([]map[string]any, 0, 16)
		if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "sharing.smb.query", &smbShares); err != nil {
			warnings = AppendTrueNASWarning(warnings, "SMB shares unavailable: "+err.Error())
			smbShares = nil
		}
		nfsShares := make([]map[string]any, 0, 16)
		if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "sharing.nfs.query", &nfsShares); err != nil {
			warnings = AppendTrueNASWarning(warnings, "NFS shares unavailable: "+err.Error())
			nfsShares = nil
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASSharesResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			SMB:       smbShares,
			NFS:       nfsShares,
			Warnings:  warnings,
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	shareType := strings.ToLower(strings.TrimSpace(subParts[0]))
	if shareType != "smb" && shareType != "nfs" {
		servicehttp.WriteError(w, http.StatusBadRequest, "share type must be smb or nfs")
		return
	}
	createMethod := "sharing." + shareType + ".create"
	updateMethod := "sharing." + shareType + ".update"
	deleteMethod := "sharing." + shareType + ".delete"

	if len(subParts) == 1 {
		// POST /truenas/assets/{id}/shares/{smb|nfs}  → create
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		var params map[string]any
		if err := shared.DecodeJSONBody(w, r, &params); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		var created map[string]any
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, createMethod, []any{params}, &created); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to create share: "+err.Error())
			return
		}
		d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASShareActionResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			ShareType: shareType,
			Share:     created,
			Message:   shareType + " share created",
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	shareID := strings.TrimSpace(subParts[1])
	if shareID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "share id is required")
		return
	}

	switch r.Method {
	case http.MethodPut:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		var params map[string]any
		if err := shared.DecodeJSONBody(w, r, &params); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		var updated map[string]any
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, updateMethod, []any{shareID, params}, &updated); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to update share: "+err.Error())
			return
		}
		d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASShareActionResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			ShareType: shareType,
			Share:     updated,
			Message:   shareType + " share updated",
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
	case http.MethodDelete:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, deleteMethod, []any{shareID}, nil); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to delete share: "+err.Error())
			return
		}
		d.InvalidateTrueNASCaches(asset.ID, runtime.CollectorID)
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASShareActionResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			ShareType: shareType,
			Message:   shareType + " share deleted",
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
