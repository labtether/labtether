package proxmox

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxNodeUpdates handles package update operations for a node.
//
//	GET  /proxmox/nodes/{node}/updates         — list available updates
//	POST /proxmox/nodes/{node}/updates/refresh — refresh apt package cache
func (d *Deps) handleProxmoxNodeUpdates(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/nodes/")
	// parts[0]=node, parts[1]="updates", parts[2]=sub-action (optional)
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/nodes/{node}/updates")
		return
	}
	node := strings.TrimSpace(parts[0])
	if node == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node is required")
		return
	}
	subAction := ""
	if len(parts) >= 3 {
		subAction = strings.TrimSpace(parts[2])
	}

	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))
	runtime, err := d.LoadProxmoxRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox runtime unavailable: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	switch {
	case r.Method == http.MethodGet && subAction == "":
		updates, listErr := runtime.client.ListUpdates(ctx, node)
		if listErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to list updates: "+shared.SanitizeUpstreamError(listErr.Error()))
			return
		}
		if updates == nil {
			updates = []map[string]any{}
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"updates": updates})

	case r.Method == http.MethodPost && subAction == "refresh":
		if !d.RequireAdminAuth(w, r) {
			return
		}
		upid, refreshErr := runtime.client.RefreshUpdates(ctx, node)
		if refreshErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to refresh updates: "+shared.SanitizeUpstreamError(refreshErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "started",
			"upid":   upid,
		})

	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown updates action")
	}
}
