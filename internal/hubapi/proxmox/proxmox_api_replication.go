package proxmox

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxNodeReplication handles replication operations for a node.
//
//   GET  /proxmox/nodes/{node}/replication         — list replication jobs
//   POST /proxmox/nodes/{node}/replication/{id}/run — trigger replication job now
func (d *Deps) handleProxmoxNodeReplication(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/nodes/")
	// parts[0]=node, parts[1]="replication", parts[2]=id (optional), parts[3]="run" (optional)
	parts := strings.SplitN(path, "/", 5)
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/nodes/{node}/replication")
		return
	}
	node := strings.TrimSpace(parts[0])
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

	// POST /proxmox/nodes/{node}/replication/{id}/run
	if len(parts) >= 4 && strings.TrimSpace(parts[3]) == "run" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		jobID := strings.TrimSpace(parts[2])
		if jobID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "replication job id is required")
			return
		}
		upid, runErr := runtime.client.RunReplication(ctx, node, jobID)
		if runErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to run replication: "+shared.SanitizeUpstreamError(runErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "started",
			"upid":   upid,
		})
		return
	}

	// GET /proxmox/nodes/{node}/replication
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	jobs, listErr := runtime.client.ListReplication(ctx, node)
	if listErr != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to list replication jobs: "+shared.SanitizeUpstreamError(listErr.Error()))
		return
	}
	if jobs == nil {
		jobs = []map[string]any{}
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}
