package proxmox

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxClusterResources handles GET /proxmox/cluster/resources
// Returns all cluster resources including nodes, VMs, containers, and storage.
func (d *Deps) HandleProxmoxClusterResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
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

	resources, err := runtime.client.GetClusterResources(ctx)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch cluster resources: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"resources": resources})
}
