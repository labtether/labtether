package proxmox

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxNodeCertificates handles certificate operations for a node.
//
//   GET  /proxmox/nodes/{node}/certificates         — list certificates
//   POST /proxmox/nodes/{node}/certificates/renew   — renew ACME certificate
func (d *Deps) handleProxmoxNodeCertificates(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/nodes/")
	// parts[0]=node, parts[1]="certificates", parts[2]=sub-action (optional)
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/nodes/{node}/certificates")
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

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	switch {
	case r.Method == http.MethodGet && subAction == "":
		certs, listErr := runtime.client.ListCertificates(ctx, node)
		if listErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to list certificates: "+shared.SanitizeUpstreamError(listErr.Error()))
			return
		}
		if certs == nil {
			certs = []map[string]any{}
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"certificates": certs})

	case r.Method == http.MethodPost && subAction == "renew":
		if !d.RequireAdminAuth(w, r) {
			return
		}
		upid, renewErr := runtime.client.RenewACMECert(ctx, node)
		if renewErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to renew certificate: "+shared.SanitizeUpstreamError(renewErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "started",
			"upid":   upid,
		})

	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown certificates action")
	}
}
