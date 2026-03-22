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

// handleProxmoxNodeNetworkCRUD handles network interface management.
//
//	GET  /proxmox/nodes/{node}/network         — list interfaces (existing handler)
//	POST /proxmox/nodes/{node}/network         — create interface
//	PUT  /proxmox/nodes/{node}/network         — apply pending changes
//	PUT  /proxmox/nodes/{node}/network/{iface} — update interface
func (d *Deps) handleProxmoxNodeNetworkCRUD(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/nodes/")
	// parts[0]=node, parts[1]="network", parts[2]=iface (optional)
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/nodes/{node}/network")
		return
	}
	node := strings.TrimSpace(parts[0])
	iface := ""
	if len(parts) >= 3 {
		iface = strings.TrimSpace(parts[2])
	}
	if node == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node is required")
		return
	}

	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))
	runtime, err := d.LoadProxmoxRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox runtime unavailable: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		ifaces, getErr := runtime.client.GetNodeNetwork(ctx, node)
		if getErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch node network: "+shared.SanitizeUpstreamError(getErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"interfaces": ifaces})

	case http.MethodPost:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		var config map[string]any
		if decErr := json.NewDecoder(r.Body).Decode(&config); decErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body: "+decErr.Error())
			return
		}
		if createErr := runtime.client.CreateNodeNetwork(ctx, node, config); createErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to create network interface: "+shared.SanitizeUpstreamError(createErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "created"})

	case http.MethodPut:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if iface == "" {
			// No interface specified — apply pending changes.
			if applyErr := runtime.client.ApplyNodeNetworkChanges(ctx, node); applyErr != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to apply network changes: "+shared.SanitizeUpstreamError(applyErr.Error()))
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "applied"})
			return
		}
		var config map[string]any
		if decErr := json.NewDecoder(r.Body).Decode(&config); decErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body: "+decErr.Error())
			return
		}
		if updateErr := runtime.client.UpdateNodeNetwork(ctx, node, iface, config); updateErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to update network interface: "+shared.SanitizeUpstreamError(updateErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
