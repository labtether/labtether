package proxmox

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/shared"
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
		writeProxmoxUpstreamError(w, "failed to resolve proxmox asset: ", err)
		return
	}
	if !ok {
		d.writeNotProxmoxDevice(w, assetID)
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
	// The selected storage and resulting task are collector-wide resources and
	// cannot be proven to affect only the VM named in the URL.
	if !d.requireProxmoxCollectorAccess(w, r, target.CollectorID) {
		return
	}

	var req struct {
		Storage string `json:"storage"`
		Mode    string `json:"mode"`
	}
	if _, decErr := shared.DecodeOptionalJSONBody(w, r, &req); decErr != nil {
		servicehttp.WriteError(w, shared.JSONDecodeErrorStatus(decErr), "invalid backup payload")
		return
	}

	if strings.TrimSpace(req.Mode) == "" {
		req.Mode = "snapshot"
	}

	runtime, err := d.LoadProxmoxRuntime(target.CollectorID)
	if err != nil {
		writeProxmoxRuntimeError(w, err, true)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	upid, err := runtime.client.TriggerBackup(ctx, target.Node, target.VMID, req.Storage, req.Mode)
	if err != nil {
		writeProxmoxUpstreamError(w, "failed to trigger backup: ", err)
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "started",
		"upid":   upid,
		"node":   target.Node,
	})
}
