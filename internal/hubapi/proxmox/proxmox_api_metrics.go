package proxmox

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxAssetMetrics handles GET /proxmox/assets/{id}/metrics?timeframe= (or ?window=)
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
		writeProxmoxUpstreamError(w, "failed to resolve proxmox asset: ", err)
		return
	}
	if !ok {
		d.writeNotProxmoxDevice(w, assetID)
		return
	}

	runtime, err := d.LoadProxmoxRuntime(target.CollectorID)
	if err != nil {
		writeProxmoxRuntimeError(w, err, true)
		return
	}

	timeframe := proxmoxMetricsTimeframe(r.URL.Query())

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(target.Kind)) {
	case "node":
		points, rrdErr := runtime.client.GetNodeRRDData(ctx, target.Node, timeframe)
		if rrdErr != nil {
			writeProxmoxUpstreamError(w, "failed to fetch node rrd data: ", rrdErr)
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
			writeProxmoxUpstreamError(w, "failed to fetch qemu rrd data: ", rrdErr)
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
			writeProxmoxUpstreamError(w, "failed to fetch lxc rrd data: ", rrdErr)
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

// proxmoxMetricsTimeframe picks the RRD range a caller asked for. The console's
// Metrics tab sends timeframe=hour|day|week|month; API clients may send
// window=1h|24h|7d|30d. Anything else is the last hour.
func proxmoxMetricsTimeframe(query url.Values) string {
	switch timeframe := strings.ToLower(strings.TrimSpace(query.Get("timeframe"))); timeframe {
	case "hour", "day", "week", "month", "year":
		return timeframe
	}
	return proxmoxMetricsWindowToTimeframe(query.Get("window"))
}

// proxmoxMetricsWindowToTimeframe maps window query params to Proxmox RRD timeframe names.
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
