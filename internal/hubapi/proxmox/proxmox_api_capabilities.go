package proxmox

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// proxmoxCapabilities describes the tabs and features available for a Proxmox asset.
type proxmoxCapabilities struct {
	Tabs           []string `json:"tabs"`
	Kind           string   `json:"kind"` // "node", "qemu", "lxc", or "storage"
	HasCeph        bool     `json:"has_ceph"`
	HasHA          bool     `json:"has_ha"`
	HasReplication bool     `json:"has_replication"`
	FetchedAt      string   `json:"fetched_at"`
}

// handleProxmoxCapabilities handles GET /proxmox/assets/{id}/capabilities.
func (d *Deps) handleProxmoxCapabilities(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/assets/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "proxmox asset path not found")
		return
	}
	assetID := strings.TrimSpace(parts[0])

	target, ok, err := d.ResolveProxmoxSessionTarget(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to resolve proxmox asset: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "asset is not proxmox-backed")
		return
	}

	kind := strings.ToLower(strings.TrimSpace(target.Kind))
	servicehttp.WriteJSON(w, http.StatusOK, proxmoxCapabilities{
		Tabs:      BuildProxmoxCapabilityTabs(kind),
		Kind:      kind,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// BuildProxmoxCapabilityTabs returns the ordered set of tabs supported by the
// asset kind without issuing extra upstream feature probes on page load.
func BuildProxmoxCapabilityTabs(kind string) []string {
	switch kind {
	case "node":
		return []string{
			"overview", "snapshots", "tasks", "firewall", "backup", "storage",
			"network", "ha", "ceph", "replication", "cluster",
			"metrics", "logs", "updates", "certificates",
		}
	case "qemu", "lxc":
		return []string{
			"overview", "snapshots", "tasks", "firewall", "backup", "storage",
			"ha", "console", "replication",
			"metrics", "logs",
		}
	case "storage":
		return []string{
			"overview", "tasks", "storage", "backup",
			"metrics", "logs",
		}
	default:
		return []string{
			"overview", "snapshots", "tasks", "storage",
			"metrics", "logs",
		}
	}
}
