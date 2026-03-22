package proxmox

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxAssetBackup handles POST /proxmox/assets/{id}/backup
// Triggers an immediate vzdump backup for the asset.
//
// Request body (JSON):
//
//	{ "storage": "local", "mode": "snapshot" }
//
// Both fields are optional. mode defaults to "snapshot" if empty.
func (d *Deps) handleProxmoxAssetBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.RequireAdminAuth(w, r) {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/proxmox/assets/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[1] != "backup" {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/assets/{id}/backup")
		return
	}
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id is required")
		return
	}

	target, ok, err := d.ResolveProxmoxSessionTarget(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to resolve proxmox asset: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "asset is not proxmox-backed")
		return
	}

	kind := strings.ToLower(strings.TrimSpace(target.Kind))
	if kind != "qemu" && kind != "lxc" {
		servicehttp.WriteError(w, http.StatusBadRequest, "backup only supported for qemu and lxc assets")
		return
	}
	if strings.TrimSpace(target.VMID) == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset missing vmid")
		return
	}

	var req struct {
		Storage string `json:"storage"`
		Mode    string `json:"mode"`
	}
	if decErr := json.NewDecoder(r.Body).Decode(&req); decErr != nil {
		// Body is optional — use empty defaults.
		req.Storage = ""
		req.Mode = ""
	}

	if strings.TrimSpace(req.Mode) == "" {
		req.Mode = "snapshot"
	}

	runtime, err := d.LoadProxmoxRuntime(target.CollectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to load proxmox runtime: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	upid, err := runtime.client.TriggerBackup(ctx, target.Node, target.VMID, req.Storage, req.Mode)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to trigger backup: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "started",
		"upid":   upid,
		"node":   target.Node,
	})
}
