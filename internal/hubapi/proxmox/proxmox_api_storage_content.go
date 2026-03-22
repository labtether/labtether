package proxmox

import (
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxStorageContent handles storage content operations.
//
//	GET    /proxmox/nodes/{node}/storage/{storage}/content         — list content
//	POST   /proxmox/nodes/{node}/storage/{storage}/content         — download URL into storage
//	DELETE /proxmox/nodes/{node}/storage/{storage}/content/{volid} — delete content item
func (d *Deps) handleProxmoxStorageContent(w http.ResponseWriter, r *http.Request) {
	// path after "/proxmox/nodes/" is: {node}/storage/{storage}/content[/{volid}]
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/nodes/")
	// Split into: [node, storage, {storage}, content, ...]
	parts := strings.SplitN(path, "/", 5)
	// parts[0]=node, parts[1]="storage", parts[2]=storageName, parts[3]="content", parts[4]=volid(optional)
	if len(parts) < 4 {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/nodes/{node}/storage/{storage}/content")
		return
	}
	node := strings.TrimSpace(parts[0])
	storageName := strings.TrimSpace(parts[2])
	volid := ""
	if len(parts) >= 5 {
		volid = strings.TrimSpace(parts[4])
	}
	if node == "" || storageName == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node and storage are required")
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

	switch r.Method {
	case http.MethodGet:
		content, getErr := runtime.client.GetStorageContent(ctx, node, storageName)
		if getErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch storage content: "+shared.SanitizeUpstreamError(getErr.Error()))
			return
		}
		if content == nil {
			content = []proxmox.StorageContent{}
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"content": content})

	case http.MethodPost:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		var req struct {
			Filename string `json:"filename"`
			URL      string `json:"url"`
		}
		if decErr := json.NewDecoder(r.Body).Decode(&req); decErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body: "+decErr.Error())
			return
		}
		if strings.TrimSpace(req.Filename) == "" || strings.TrimSpace(req.URL) == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "filename and url are required")
			return
		}
		upid, postErr := runtime.client.DownloadStorageURL(ctx, node, storageName, req.Filename, req.URL)
		if postErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to start download: "+shared.SanitizeUpstreamError(postErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "started",
			"upid":   upid,
		})

	case http.MethodDelete:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if volid == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "volid is required for DELETE")
			return
		}
		if delErr := runtime.client.DeleteStorageContent(ctx, node, storageName, volid); delErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to delete storage content: "+shared.SanitizeUpstreamError(delErr.Error()))
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
