package proxmox

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxNodeSyslog handles GET /proxmox/nodes/{node}/syslog
//
// Query params:
//
//	limit — max number of lines (default 500)
//	since — start timestamp string (optional, passed directly to Proxmox)
func (d *Deps) handleProxmoxNodeSyslog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/proxmox/nodes/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[1] != "syslog" {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/nodes/{node}/syslog")
		return
	}
	node := strings.TrimSpace(parts[0])
	if node == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node is required")
		return
	}

	limit := 500
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if parsed, parseErr := strconv.Atoi(rawLimit); parseErr == nil && parsed > 0 {
			limit = parsed
		}
	}
	since := strings.TrimSpace(r.URL.Query().Get("since"))
	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))

	runtime, err := d.LoadProxmoxRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox runtime unavailable: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	lines, err := runtime.client.GetNodeSyslog(ctx, node, limit, since)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch syslog: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	if lines == nil {
		lines = []map[string]any{}
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"node":  node,
		"lines": lines,
	})
}
