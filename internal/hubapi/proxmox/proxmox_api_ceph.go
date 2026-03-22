package proxmox

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxCeph handles Ceph management endpoints.
//
//   GET  /proxmox/ceph/pools               — list Ceph pools
//   POST /proxmox/ceph/osd/{node}/{id}/in  — mark OSD in
//   POST /proxmox/ceph/osd/{node}/{id}/out — mark OSD out
func (d *Deps) HandleProxmoxCeph(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/ceph/")
	parts := strings.SplitN(path, "/", 5)
	if len(parts) == 0 || parts[0] == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/ceph/{pools|osd/...}")
		return
	}

	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))
	runtime, err := d.LoadProxmoxRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox runtime unavailable: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch parts[0] {
	case "pools":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		pools, listErr := runtime.client.ListCephPools(ctx)
		if listErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to list ceph pools: "+shared.SanitizeUpstreamError(listErr.Error()))
			return
		}
		if pools == nil {
			pools = []map[string]any{}
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"pools": pools})

	case "osd":
		// path: osd/{node}/{id}/{in|out}
		// parts: [osd, node, id, state]
		if len(parts) < 4 {
			servicehttp.WriteError(w, http.StatusBadRequest, "expected /proxmox/ceph/osd/{node}/{id}/{in|out}")
			return
		}
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		node := strings.TrimSpace(parts[1])
		osdIDStr := strings.TrimSpace(parts[2])
		state := strings.TrimSpace(parts[3])
		if node == "" || osdIDStr == "" || state == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "node, osd id, and state are required")
			return
		}
		if state != "in" && state != "out" {
			servicehttp.WriteError(w, http.StatusBadRequest, "state must be 'in' or 'out'")
			return
		}
		osdID, parseErr := strconv.Atoi(osdIDStr)
		if parseErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "osd id must be numeric")
			return
		}
		if setErr := runtime.client.SetCephOSDState(ctx, node, osdID, state); setErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to set osd state: "+shared.SanitizeUpstreamError(setErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"state":  state,
		})

	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown ceph action")
	}
}

// handleProxmoxCephStatus handles GET /proxmox/ceph/status
func (d *Deps) HandleProxmoxCephStatus(w http.ResponseWriter, r *http.Request) {
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

	status, err := runtime.client.GetCephStatus(ctx)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch ceph status: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	osds, osdErr := runtime.client.GetCephOSDs(ctx)
	if osdErr != nil {
		// Ceph OSDs are non-fatal — return status without OSD list.
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": status})
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"osds":   osds,
	})
}

