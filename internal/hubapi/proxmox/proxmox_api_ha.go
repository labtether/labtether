package proxmox

import (
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxAssetHA handles HA operations for a Proxmox asset.
//
//	POST /proxmox/assets/{id}/ha/migrate — migrate VM/CT to target node via HA
//	PUT  /proxmox/assets/{id}/ha        — update HA resource config
func (d *Deps) handleProxmoxAssetHA(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/assets/")
	// parts[0]=id, parts[1]="ha", parts[2]=sub-action (optional)
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/assets/{id}/ha")
		return
	}
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id is required")
		return
	}
	subAction := ""
	if len(parts) >= 3 {
		subAction = strings.TrimSpace(parts[2])
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

	runtime, err := d.LoadProxmoxRuntime(target.CollectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to load proxmox runtime: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch {
	case r.Method == http.MethodPost && subAction == "migrate":
		if !d.RequireAdminAuth(w, r) {
			return
		}
		d.proxmoxHAMigrate(w, r, ctx, target, runtime)

	case r.Method == http.MethodPut && subAction == "":
		if !d.RequireAdminAuth(w, r) {
			return
		}
		d.proxmoxHAUpdateConfig(w, r, ctx, target, runtime)

	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown HA action")
	}
}

func (d *Deps) proxmoxHAMigrate(w http.ResponseWriter, r *http.Request, ctx context.Context, target ProxmoxSessionTarget, runtime *ProxmoxRuntime) {
	var req struct {
		TargetNode string `json:"target_node"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.TargetNode) == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "target_node is required")
		return
	}

	kind := strings.ToLower(strings.TrimSpace(target.Kind))
	if kind != "qemu" && kind != "lxc" {
		servicehttp.WriteError(w, http.StatusBadRequest, "migrate only supported for qemu and lxc assets")
		return
	}

	var upid string
	var err error
	switch kind {
	case "qemu":
		upid, err = runtime.client.MigrateVM(ctx, target.Node, target.VMID, req.TargetNode)
	case "lxc":
		upid, err = runtime.client.MigrateCT(ctx, target.Node, target.VMID, req.TargetNode)
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to migrate: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
		"status":      "started",
		"upid":        upid,
		"target_node": req.TargetNode,
	})
}

func (d *Deps) proxmoxHAUpdateConfig(w http.ResponseWriter, r *http.Request, ctx context.Context, target ProxmoxSessionTarget, runtime *ProxmoxRuntime) {
	var config map[string]any
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Derive the HA SID from the asset kind and vmid.
	sid, ok := proxmoxHASIDFromTarget(target)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadRequest, "cannot determine HA SID for this asset kind")
		return
	}

	if err := runtime.client.UpdateHAResource(ctx, sid, config); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to update HA resource: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated", "sid": sid})
}

// proxmoxHASIDFromTarget derives the HA SID string from a session target.
// Returns the SID and true on success; empty string and false otherwise.
func proxmoxHASIDFromTarget(target ProxmoxSessionTarget) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(target.Kind)) {
	case "qemu":
		vmid := strings.TrimSpace(target.VMID)
		if vmid == "" {
			return "", false
		}
		return "vm:" + vmid, true
	case "lxc":
		vmid := strings.TrimSpace(target.VMID)
		if vmid == "" {
			return "", false
		}
		return "ct:" + vmid, true
	default:
		return "", false
	}
}
