package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

func writeFileProtocolError(w http.ResponseWriter, status int, clientMessage string, err error) {
	securityruntime.Logf("file protocol: %s: %v", clientMessage, err)
	servicehttp.WriteError(w, status, clientMessage)
}

func writeFileOperationError(w http.ResponseWriter, fallbackStatus int, clientMessage string, err error) {
	switch {
	case errors.Is(err, errFileOperationCapacity):
		securityruntime.Logf("file protocol: operation capacity reached: %v", err)
		writeFileOperationCapacityError(w)
	case errors.Is(err, fileproto.ErrListLimitExceeded),
		errors.Is(err, fileproto.ErrResponseTooLarge),
		errors.Is(err, fileproto.ErrTransferTooLarge),
		errors.Is(err, fileproto.ErrDeleteLimitExceeded):
		writeFileProtocolError(w, http.StatusRequestEntityTooLarge, "operation exceeds configured resource limits", err)
	case errors.Is(err, context.DeadlineExceeded):
		writeFileProtocolError(w, http.StatusGatewayTimeout, "file operation timed out", err)
	default:
		writeFileProtocolError(w, fallbackStatus, clientMessage, err)
	}
}

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
	release, ok := interactiveFileOperationAdmission.tryAcquire()
	if !ok {
		writeFileOperationCapacityError(w)
		return
	}
	defer release()

	opCtx, cancel := fileproto.WithOperationTimeout(r.Context())
	defer cancel()
	r = r.WithContext(opCtx)

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
		writeFileProtocolError(w, http.StatusInternalServerError, "failed to load file connection", err)
		return nil, nil, false
	}
	if !d.canAccessFileConnection(r, fc) {
		servicehttp.WriteError(w, http.StatusForbidden, "file connection access denied")
		return nil, nil, false
	}

	config, err := d.buildConnectionConfig(fc)
	if err != nil {
		writeFileProtocolError(w, http.StatusInternalServerError, "failed to build connection config", err)
		return nil, nil, false
	}

	fs, err := d.FileProtoPool.Get(r.Context(), connID, config)
	if err != nil {
		writeFileProtocolError(w, http.StatusBadGateway, "file connection failed", err)
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
		writeFileOperationError(w, http.StatusBadRequest, "listing failed", err)
		return
	}

	writeBoundedFileListing(w, dirPath, entries)
}

func writeBoundedFileListing(w http.ResponseWriter, dirPath string, entries []fileproto.FileEntry) {
	payload, err := json.Marshal(map[string]any{
		"path":    dirPath,
		"entries": entries,
	})
	if err != nil {
		writeFileProtocolError(w, http.StatusInternalServerError, "failed to encode listing", err)
		return
	}
	if len(payload) > fileproto.MaxListResponseBytes {
		writeFileProtocolError(w, http.StatusRequestEntityTooLarge, "operation exceeds configured resource limits", fileproto.ErrResponseTooLarge)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(payload); err != nil {
		securityruntime.Logf("file protocol: failed to write listing response: %v", err)
	}
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
		writeFileOperationError(w, http.StatusBadRequest, "read failed", err)
		return
	}
	defer reader.Close()
	if size > fileproto.MaxTransferBytes {
		writeFileOperationError(w, http.StatusBadRequest, "read failed", fileproto.ErrTransferTooLarge)
		return
	}

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
		securityruntime.Logf("file protocol: download stream failed: %v", err)
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

	// Enforce the transfer limit both from the declared size and while reading.
	if r.ContentLength > fileproto.MaxTransferBytes {
		servicehttp.WriteError(w, http.StatusRequestEntityTooLarge, "file exceeds 512 MB limit")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, fileproto.MaxTransferBytes)

	fs, _, ok := d.getPooledFS(w, r, connID)
	if !ok {
		return
	}

	// Pass -1 for HTTP uploads: the Content-Length from the client is untrusted
	// and forwarding it to protocol adapters (especially WebDAV) can cause
	// server rejections on size mismatches. Let the adapter handle buffering.
	if err := fs.Write(r.Context(), filePath, r.Body, -1); err != nil {
		writeFileConnectionUploadError(w, err)
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"path": filePath,
	})
}

func writeFileConnectionUploadError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) || errors.Is(err, fileproto.ErrTransferTooLarge) {
		securityruntime.Logf("file protocol: upload exceeded size limit: %v", err)
		servicehttp.WriteError(w, http.StatusRequestEntityTooLarge, "file exceeds 512 MB limit")
		return
	}
	writeFileOperationError(w, http.StatusBadRequest, "upload failed", err)
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
		writeFileOperationError(w, http.StatusBadRequest, "mkdir failed", err)
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
		writeFileOperationError(w, http.StatusBadRequest, "delete failed", err)
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
		writeFileOperationError(w, http.StatusBadRequest, "rename failed", err)
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
		writeFileOperationError(w, http.StatusBadRequest, "copy failed", err)
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"src_path": req.SrcPath,
		"dst_path": req.DstPath,
	})
}

// copyViaReadWrite implements a copy fallback through a bounded, private
// temporary file. Staging is necessary because FTP/SMB sessions serialize
// operations: retaining the source stream while opening the destination on the
// same session would deadlock. It also ensures the source is complete and
// within limits before the destination is touched.
func (d *Deps) copyViaReadWrite(ctx context.Context, fs fileproto.RemoteFS, srcPath, dstPath string) error {
	release, ok := stagedFileCopyAdmission.tryAcquire()
	if !ok {
		return errFileOperationCapacity
	}
	defer release()

	reader, size, err := fs.Read(ctx, srcPath)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	readerClosed := false
	defer func() {
		if !readerClosed {
			_ = reader.Close()
		}
	}()
	if size > fileproto.MaxTransferBytes {
		return fileproto.ErrTransferTooLarge
	}

	tmp, err := os.CreateTemp("", "labtether-file-copy-*")
	if err != nil {
		return fmt.Errorf("stage source: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	written, err := io.CopyBuffer(tmp, io.LimitReader(reader, fileproto.MaxTransferBytes), make([]byte, 64*1024))
	if err != nil {
		return fmt.Errorf("stage source: %w", err)
	}
	if written == fileproto.MaxTransferBytes {
		var probe [1]byte
		n, probeErr := reader.Read(probe[:])
		if n > 0 {
			return fileproto.ErrTransferTooLarge
		}
		if probeErr != nil && !errors.Is(probeErr, io.EOF) {
			return fmt.Errorf("stage source: %w", probeErr)
		}
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("close source: %w", err)
	}
	readerClosed = true
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind staged source: %w", err)
	}

	if err := fs.Write(ctx, dstPath, tmp, written); err != nil {
		return fmt.Errorf("write destination: %w", err)
	}
	return nil
}
