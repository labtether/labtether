package resources

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	fileRequestTimeout = 30 * time.Second
	fileChunkSizeHub   = 64 * 1024
)

// newFileBridge is a convenience alias for NewFileBridge.
func newFileBridge(buffer int, expectedAssetID string) *FileBridge {
	return NewFileBridge(buffer, expectedAssetID)
}

// handleFiles dispatches /files/{assetId}/{action} requests.
func (d *Deps) HandleFiles(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/files/")
	if path == "" || path == r.URL.Path {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	action := ""
	if len(parts) > 1 {
		action = strings.TrimSpace(parts[1])
	}

	// Check agent is connected.
	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent not connected")
		return
	}

	switch action {
	case "list":
		d.HandleFileList(w, r, assetID)
	case "download":
		d.HandleFileDownload(w, r, assetID)
	case "upload":
		d.HandleFileUpload(w, r, assetID)
	case "mkdir":
		d.HandleFileMkdir(w, r, assetID)
	case "delete":
		d.HandleFileDelete(w, r, assetID)
	case "rename":
		d.HandleFileRename(w, r, assetID)
	case "copy":
		d.HandleFileCopy(w, r, assetID)
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown file action")
	}
}
