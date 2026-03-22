package proxmox

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxTaskRoutes dispatches /proxmox/tasks/{node}/{upid}/{action}
func (d *Deps) HandleProxmoxTaskRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/tasks/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing task path")
		return
	}
	if strings.HasSuffix(path, "/log") {
		d.HandleProxmoxTaskLog(w, r)
		return
	}
	if strings.HasSuffix(path, "/stop") {
		d.HandleProxmoxTaskStop(w, r)
		return
	}
	servicehttp.WriteError(w, http.StatusNotFound, "unknown proxmox task action")
}

func (d *Deps) HandleProxmoxAssets(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/assets/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "proxmox asset path not found")
		return
	}
	parts := strings.Split(path, "/")
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "proxmox asset path not found")
		return
	}
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown proxmox asset action")
		return
	}

	action := strings.TrimSpace(parts[1])

	// Dispatch to dedicated handlers for multi-method resource groups.
	switch action {
	case "capabilities":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.handleProxmoxCapabilities(w, r)
		return
	case "metrics":
		d.handleProxmoxAssetMetrics(w, r)
		return
	case "firewall":
		d.handleProxmoxAssetFirewall(w, r)
		return
	case "backup":
		d.handleProxmoxAssetBackup(w, r)
		return
	case "ha":
		d.handleProxmoxAssetHA(w, r)
		return
	}

	storageSubAction := ""
	if action == "storage" && len(parts) >= 3 {
		storageSubAction = strings.TrimSpace(parts[2])
	}
	if action != "details" && !(action == "storage" && storageSubAction == "insights") {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown proxmox asset action")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
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

	runtime, err := d.LoadProxmoxRuntime(target.CollectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to load proxmox runtime: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if action == "details" {
		resp, loadErr := d.LoadProxmoxAssetDetails(ctx, assetID, target, runtime)
		if loadErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to load proxmox details: "+loadErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, resp)
		return
	}

	window := ParseStorageInsightsWindow(r.URL.Query().Get("window"))
	resp, loadErr := d.LoadProxmoxStorageInsights(ctx, assetID, target, runtime, window)
	if loadErr != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to load proxmox storage insights: "+loadErr.Error())
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, resp)
}
