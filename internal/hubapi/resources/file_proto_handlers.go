package resources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// fileProtoOps is the set of path actions handled as file operations
// (as opposed to CRUD or test actions on the connection resource itself).
var fileProtoOps = map[string]bool{
	"list":     true,
	"download": true,
	"upload":   true,
	"mkdir":    true,
	"delete":   true,
	"rename":   true,
	"copy":     true,
}

// IsFileProtoOp returns true if the action string is a file operation
// that should be dispatched to the file protocol handlers.
func IsFileProtoOp(action string) bool {
	return fileProtoOps[action]
}

// dispatchFileProtoOp routes a file operation to the appropriate handler.
func (d *Deps) dispatchFileProtoOp(w http.ResponseWriter, r *http.Request, connID, action string) {
	switch action {
	case "list":
		d.handleFileConnectionList(w, r, connID)
	case "download":
		d.handleFileConnectionDownload(w, r, connID)
	case "upload":
		d.handleFileConnectionUpload(w, r, connID)
	case "mkdir":
		d.handleFileConnectionMkdir(w, r, connID)
	case "delete":
		d.handleFileConnectionDeleteOp(w, r, connID)
	case "rename":
		d.handleFileConnectionRename(w, r, connID)
	case "copy":
		d.handleFileConnectionCopy(w, r, connID)
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown file operation")
	}
}

// getPooledFS loads a file connection from the DB, builds the connection
// config (decrypting credentials), and obtains a pooled RemoteFS session.
func (d *Deps) getPooledFS(w http.ResponseWriter, r *http.Request, connID string) (fileproto.RemoteFS, *persistence.FileConnection, bool) {
	if d.FileProtoPool == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "file protocol pool not initialized")
		return nil, nil, false
	}
	if d.SecretsManager == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential encryption not configured")
		return nil, nil, false
	}
	if d.CredentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return nil, nil, false
	}

	fc, err := d.FileConnectionStore.GetFileConnection(r.Context(), connID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "file connection not found")
			return nil, nil, false
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load file connection")
		return nil, nil, false
	}

	config, err := d.buildConnectionConfig(fc)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to build connection config: %s", err.Error()))
		return nil, nil, false
	}

	fs, err := d.FileProtoPool.Get(r.Context(), connID, config)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, fmt.Sprintf("connection failed: %s", err.Error()))
		return nil, nil, false
	}

	return fs, fc, true
}

// --- List ---

func (d *Deps) handleFileConnectionList(w http.ResponseWriter, r *http.Request, connID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	dirPath := strings.TrimSpace(r.URL.Query().Get("path"))
	showHidden := r.URL.Query().Get("show_hidden") == "true"

	fs, fc, ok := d.getPooledFS(w, r, connID)
	if !ok {
		return
	}

	// Default to the connection's initial path if none provided.
	if dirPath == "" {
		dirPath = fc.InitialPath
		if dirPath == "" {
			dirPath = "/"
		}
	}

	entries, err := fs.List(r.Context(), dirPath, showHidden)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("listing failed: %s", err.Error()))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"path":    dirPath,
		"entries": entries,
	})
}

// --- Download ---

func (d *Deps) handleFileConnectionDownload(w http.ResponseWriter, r *http.Request, connID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if filePath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}

	fs, _, ok := d.getPooledFS(w, r, connID)
	if !ok {
		return
	}

	reader, size, err := fs.Read(r.Context(), filePath)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("read failed: %s", err.Error()))
		return
	}
	defer reader.Close()

	filename := path.Base(filePath)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if size >= 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	}
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, reader); err != nil {
		// Headers already sent, cannot write an error response.
		// The client will see a truncated body.
		return
	}
}

// --- Upload ---

func (d *Deps) handleFileConnectionUpload(w http.ResponseWriter, r *http.Request, connID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if filePath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}

	// Enforce 512MB upload limit.
	const maxUploadBytes int64 = 512 * 1024 * 1024
	if r.ContentLength > maxUploadBytes {
		servicehttp.WriteError(w, http.StatusRequestEntityTooLarge, "file exceeds 512 MB limit")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	fs, _, ok := d.getPooledFS(w, r, connID)
	if !ok {
		return
	}

	// Pass -1 for HTTP uploads: the Content-Length from the client is untrusted
	// and forwarding it to protocol adapters (especially WebDAV) can cause
	// server rejections on size mismatches. Let the adapter handle buffering.
	if err := fs.Write(r.Context(), filePath, r.Body, -1); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("upload failed: %s", err.Error()))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"path": filePath,
	})
}

// --- Mkdir ---

func (d *Deps) handleFileConnectionMkdir(w http.ResponseWriter, r *http.Request, connID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	dirPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if dirPath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}

	fs, _, ok := d.getPooledFS(w, r, connID)
	if !ok {
		return
	}

	if err := fs.Mkdir(r.Context(), dirPath); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("mkdir failed: %s", err.Error()))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"path": dirPath,
	})
}

// --- Delete ---

func (d *Deps) handleFileConnectionDeleteOp(w http.ResponseWriter, r *http.Request, connID string) {
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if filePath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}

	fs, _, ok := d.getPooledFS(w, r, connID)
	if !ok {
		return
	}

	if err := fs.Delete(r.Context(), filePath); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("delete failed: %s", err.Error()))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"path": filePath,
	})
}

// --- Rename ---

type fileConnectionRenameRequest struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

func (d *Deps) handleFileConnectionRename(w http.ResponseWriter, r *http.Request, connID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req fileConnectionRenameRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.OldPath = strings.TrimSpace(req.OldPath)
	req.NewPath = strings.TrimSpace(req.NewPath)

	if req.OldPath == "" || req.NewPath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "old_path and new_path are required")
		return
	}

	fs, _, ok := d.getPooledFS(w, r, connID)
	if !ok {
		return
	}

	if err := fs.Rename(r.Context(), req.OldPath, req.NewPath); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("rename failed: %s", err.Error()))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"old_path": req.OldPath,
		"new_path": req.NewPath,
	})
}

// --- Copy ---

type fileConnectionCopyRequest struct {
	SrcPath string `json:"src_path"`
	DstPath string `json:"dst_path"`
}

func (d *Deps) handleFileConnectionCopy(w http.ResponseWriter, r *http.Request, connID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req fileConnectionCopyRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.SrcPath = strings.TrimSpace(req.SrcPath)
	req.DstPath = strings.TrimSpace(req.DstPath)

	if req.SrcPath == "" || req.DstPath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "src_path and dst_path are required")
		return
	}

	fs, _, ok := d.getPooledFS(w, r, connID)
	if !ok {
		return
	}

	err := fs.Copy(r.Context(), req.SrcPath, req.DstPath)
	if err != nil && errors.Is(err, fileproto.ErrNotSupported) {
		// Fallback: read src and write to dst.
		err = d.copyViaReadWrite(r.Context(), fs, req.SrcPath, req.DstPath)
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("copy failed: %s", err.Error()))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"src_path": req.SrcPath,
		"dst_path": req.DstPath,
	})
}

// copyViaReadWrite implements a server-side copy fallback by reading from src
// and writing to dst through the same RemoteFS session.
func (d *Deps) copyViaReadWrite(ctx context.Context, fs fileproto.RemoteFS, srcPath, dstPath string) error {
	reader, size, err := fs.Read(ctx, srcPath)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	defer reader.Close()

	if err := fs.Write(ctx, dstPath, reader, size); err != nil {
		return fmt.Errorf("write destination: %w", err)
	}
	return nil
}
