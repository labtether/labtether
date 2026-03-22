package proxmox

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxAssetMetrics handles GET /proxmox/assets/{id}/metrics?window=
// Returns RRD time-series data for a Proxmox asset (node, qemu, or lxc).
func (d *Deps) handleProxmoxAssetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/proxmox/assets/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[1] != "metrics" {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/assets/{id}/metrics")
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

	runtime, err := d.LoadProxmoxRuntime(target.CollectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to load proxmox runtime: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	timeframe := proxmoxMetricsWindowToTimeframe(r.URL.Query().Get("window"))

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(target.Kind)) {
	case "node":
		points, rrdErr := runtime.client.GetNodeRRDData(ctx, target.Node, timeframe)
		if rrdErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch node rrd data: "+shared.SanitizeUpstreamError(rrdErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"asset_id":  assetID,
			"kind":      target.Kind,
			"node":      target.Node,
			"timeframe": timeframe,
			"points":    points,
		})
	case "qemu":
		points, rrdErr := runtime.client.GetQemuRRDData(ctx, target.Node, target.VMID, timeframe)
		if rrdErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch qemu rrd data: "+shared.SanitizeUpstreamError(rrdErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"asset_id":  assetID,
			"kind":      target.Kind,
			"node":      target.Node,
			"vmid":      target.VMID,
			"timeframe": timeframe,
			"points":    points,
		})
	case "lxc":
		points, rrdErr := runtime.client.GetLXCRRDData(ctx, target.Node, target.VMID, timeframe)
		if rrdErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch lxc rrd data: "+shared.SanitizeUpstreamError(rrdErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"asset_id":  assetID,
			"kind":      target.Kind,
			"node":      target.Node,
			"vmid":      target.VMID,
			"timeframe": timeframe,
			"points":    points,
		})
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "metrics not supported for kind: "+target.Kind)
	}
}

// proxmoxMetricsWindowToTimeframe maps UI window query params to Proxmox RRD timeframe names.
// Proxmox accepts: hour, day, week, month, year.
func proxmoxMetricsWindowToTimeframe(window string) string {
	switch strings.ToLower(strings.TrimSpace(window)) {
	case "1h":
		return "hour"
	case "24h":
		return "day"
	case "7d":
		return "week"
	case "30d":
		return "month"
	default:
		return "hour"
	}
}
