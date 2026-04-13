package resources

import (
	"context"
	"errors"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

const fileTransferAPIPrefix = "/api/v1/file-transfers"

// progressThrottleInterval is the minimum interval between DB progress updates.
const progressThrottleInterval = 500 * time.Millisecond

// progressThrottleBytes is the minimum bytes between DB progress updates.
const progressThrottleBytes int64 = 1 << 20 // 1 MB

func (d *Deps) fileTransferActorID(ctx context.Context) string {
	if d.PrincipalActorID != nil {
		actorID := strings.TrimSpace(d.PrincipalActorID(ctx))
		if actorID != "" {
			return actorID
		}
	}
	return "system"
}

// HandleFileTransfers dispatches /api/v1/file-transfers requests.
func (d *Deps) HandleFileTransfers(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, fileTransferAPIPrefix)
	trimmed = strings.TrimPrefix(trimmed, "/")

	if d.FileTransferStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "file transfer store unavailable")
		return
	}

	// Collection routes: /api/v1/file-transfers
	if trimmed == "" {
		switch r.Method {
		case http.MethodPost:
			d.handleStartFileTransfer(w, r)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Single-resource routes: /api/v1/file-transfers/{id}
	parts := strings.SplitN(trimmed, "/", 2)
	transferID := strings.TrimSpace(parts[0])
	if transferID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if len(parts) > 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.handleGetFileTransfer(w, r, transferID)
	case http.MethodDelete:
		d.handleCancelFileTransfer(w, r, transferID)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// --- Start Transfer ---

type fileTransferStartRequest struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	SourcePath string `json:"source_path"`
	DestType   string `json:"dest_type"`
	DestID     string `json:"dest_id"`
	DestPath   string `json:"dest_path"`
}

func (d *Deps) handleStartFileTransfer(w http.ResponseWriter, r *http.Request) {
	if d.FileProtoPool == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "file protocol pool not initialized")
		return
	}
	if d.FileConnectionStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "file connection store unavailable")
		return
	}

	var req fileTransferStartRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.SourceType = strings.TrimSpace(req.SourceType)
	req.SourceID = strings.TrimSpace(req.SourceID)
	req.SourcePath = strings.TrimSpace(req.SourcePath)
	req.DestType = strings.TrimSpace(req.DestType)
	req.DestID = strings.TrimSpace(req.DestID)
	req.DestPath = strings.TrimSpace(req.DestPath)

	if err := validateTransferRequest(req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Only connection-to-connection is supported for now.
	if req.SourceType == "agent" || req.DestType == "agent" {
		servicehttp.WriteError(w, http.StatusNotImplemented, "agent-to-connection transfers are not yet supported; only connection-to-connection transfers are available")
		return
	}

	// Create the pending transfer record.
	ft := &persistence.FileTransfer{
		ActorID:    d.fileTransferActorID(r.Context()),
		SourceType: req.SourceType,
		SourceID:   req.SourceID,
		SourcePath: req.SourcePath,
		DestType:   req.DestType,
		DestID:     req.DestID,
		DestPath:   req.DestPath,
		FileName:   path.Base(req.SourcePath),
		Status:     "pending",
	}
	if err := d.FileTransferStore.CreateFileTransfer(r.Context(), ft); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create file transfer record")
		return
	}

	// Launch the transfer goroutine with a cancellable context.
	transferCtx, cancelTransfer := context.WithCancel(context.WithoutCancel(r.Context()))
	if d.ActiveTransfers != nil {
		d.ActiveTransfers.Store(ft.ID, cancelTransfer)
	}
	go d.runFileTransfer(transferCtx, cancelTransfer, ft.ID, req) // #nosec G118 -- File transfers intentionally outlive the initiating HTTP request and use an explicit cancel handle.

	servicehttp.WriteJSON(w, http.StatusAccepted, map[string]any{"transfer": ft})
}

// --- Get Transfer ---

func (d *Deps) handleGetFileTransfer(w http.ResponseWriter, r *http.Request, transferID string) {
	ft, err := d.FileTransferStore.GetFileTransfer(r.Context(), transferID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "file transfer not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load file transfer")
		return
	}
	if strings.TrimSpace(ft.ActorID) != d.fileTransferActorID(r.Context()) {
		servicehttp.WriteError(w, http.StatusNotFound, "file transfer not found")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"transfer": ft})
}

// --- Cancel Transfer ---

func (d *Deps) handleCancelFileTransfer(w http.ResponseWriter, r *http.Request, transferID string) {
	ft, err := d.FileTransferStore.GetFileTransfer(r.Context(), transferID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "file transfer not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load file transfer")
		return
	}
	if strings.TrimSpace(ft.ActorID) != d.fileTransferActorID(r.Context()) {
		servicehttp.WriteError(w, http.StatusNotFound, "file transfer not found")
		return
	}

	// Only active/pending transfers can be cancelled.
	if ft.Status != "pending" && ft.Status != "in_progress" {
		servicehttp.WriteError(w, http.StatusConflict, "transfer is not active")
		return
	}

	// Cancel the running goroutine if tracked.
	// The goroutine is the sole writer of terminal status — we only signal
	// cancellation here and return the current state. The goroutine will
	// detect context.Canceled and write the "cancelled" status itself.
	if d.ActiveTransfers != nil {
		if cancelVal, ok := d.ActiveTransfers.Load(ft.ID); ok {
			if cancelFn, ok := cancelVal.(context.CancelFunc); ok {
				cancelFn()
			}
		}
	}

	// Return current state — the goroutine will finalize the status asynchronously.
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"transfer": ft})
}

// --- Background Transfer Execution ---

func (d *Deps) runFileTransfer(ctx context.Context, cancel context.CancelFunc, transferID string, req fileTransferStartRequest) {
	defer cancel()
	defer func() {
		if d.ActiveTransfers != nil {
			d.ActiveTransfers.Delete(transferID)
		}
	}()

	bgCtx := context.Background()

	// Helper to mark failure.
	markFailed := func(errMsg string) {
		ft, err := d.FileTransferStore.GetFileTransfer(bgCtx, transferID)
		if err != nil {
			log.Printf("file-transfers: failed to load transfer %s for failure update: %v", transferID, err)
			return
		}
		now := time.Now().UTC()
		ft.Status = "failed"
		ft.Error = &errMsg
		ft.CompletedAt = &now
		if err := d.FileTransferStore.UpdateFileTransfer(bgCtx, ft); err != nil {
			log.Printf("file-transfers: failed to update transfer %s as failed: %v", transferID, err)
		}
	}

	// Resolve source connection config.
	srcConfig, err := d.resolveConnectionConfig(bgCtx, req.SourceID)
	if err != nil {
		markFailed("failed to resolve source connection: " + err.Error())
		return
	}

	// Resolve dest connection config.
	dstConfig, err := d.resolveConnectionConfig(bgCtx, req.DestID)
	if err != nil {
		markFailed("failed to resolve destination connection: " + err.Error())
		return
	}

	// Get RemoteFS sessions from the pool. Use transfer-scoped IDs to avoid
	// interfering with interactive browsing sessions on the same connections.
	srcPoolID := "transfer-src-" + transferID
	srcFS, err := d.FileProtoPool.Get(ctx, srcPoolID, srcConfig)
	if err != nil {
		markFailed("failed to connect to source: " + err.Error())
		return
	}
	defer d.FileProtoPool.Remove(srcPoolID)

	dstPoolID := "transfer-dst-" + transferID
	dstFS, err := d.FileProtoPool.Get(ctx, dstPoolID, dstConfig)
	if err != nil {
		markFailed("failed to connect to destination: " + err.Error())
		return
	}
	defer d.FileProtoPool.Remove(dstPoolID)

	// Mark transfer as in_progress.
	ft, err := d.FileTransferStore.GetFileTransfer(bgCtx, transferID)
	if err != nil {
		log.Printf("file-transfers: failed to load transfer %s: %v", transferID, err)
		return
	}
	now := time.Now().UTC()
	ft.Status = "in_progress"
	ft.StartedAt = &now
	if err := d.FileTransferStore.UpdateFileTransfer(bgCtx, ft); err != nil {
		log.Printf("file-transfers: failed to mark transfer %s as in_progress: %v", transferID, err)
		return
	}

	// Throttled progress callback.
	var progressMu sync.Mutex
	lastProgressTime := time.Now()
	var lastProgressBytes int64

	progressFn := func(bytesTransferred int64, totalSize int64) {
		progressMu.Lock()
		defer progressMu.Unlock()

		elapsed := time.Since(lastProgressTime)
		bytesDelta := bytesTransferred - lastProgressBytes

		if elapsed < progressThrottleInterval && bytesDelta < progressThrottleBytes {
			return
		}

		lastProgressTime = time.Now()
		lastProgressBytes = bytesTransferred

		// Update DB in the background to avoid blocking the transfer.
		ft, loadErr := d.FileTransferStore.GetFileTransfer(bgCtx, transferID)
		if loadErr != nil {
			return
		}
		ft.BytesTransferred = bytesTransferred
		if totalSize > 0 {
			ft.FileSize = &totalSize
		}
		_ = d.FileTransferStore.UpdateFileTransfer(bgCtx, ft)
	}

	// Execute the transfer.
	transferred, transferErr := fileproto.Transfer(ctx, srcFS, req.SourcePath, dstFS, req.DestPath, progressFn)

	// Load final state for update.
	ft, err = d.FileTransferStore.GetFileTransfer(bgCtx, transferID)
	if err != nil {
		log.Printf("file-transfers: failed to load transfer %s for final update: %v", transferID, err)
		return
	}

	completedAt := time.Now().UTC()
	ft.BytesTransferred = transferred
	ft.CompletedAt = &completedAt

	if transferErr != nil {
		ft.Status = "failed"
		// Distinguish cancellation from real errors for a clean UI message.
		if errors.Is(transferErr, context.Canceled) {
			errStr := "cancelled"
			ft.Error = &errStr
		} else {
			errStr := transferErr.Error()
			ft.Error = &errStr
		}
	} else {
		ft.Status = "completed"
	}

	if err := d.FileTransferStore.UpdateFileTransfer(bgCtx, ft); err != nil {
		log.Printf("file-transfers: failed to update transfer %s final status: %v", transferID, err)
	}
}

// resolveConnectionConfig loads a file connection by ID and builds its ConnectionConfig.
func (d *Deps) resolveConnectionConfig(ctx context.Context, connectionID string) (fileproto.ConnectionConfig, error) {
	fc, err := d.FileConnectionStore.GetFileConnection(ctx, connectionID)
	if err != nil {
		return fileproto.ConnectionConfig{}, err
	}
	return d.buildConnectionConfig(fc)
}

// --- Validation ---

func validateTransferRequest(req fileTransferStartRequest) error {
	validTypes := map[string]bool{"connection": true, "agent": true}

	if req.SourceType == "" {
		return errors.New("source_type is required")
	}
	if !validTypes[req.SourceType] {
		return errors.New("source_type must be 'connection' or 'agent'")
	}
	if req.SourceID == "" {
		return errors.New("source_id is required")
	}
	if req.SourcePath == "" {
		return errors.New("source_path is required")
	}
	if req.DestType == "" {
		return errors.New("dest_type is required")
	}
	if !validTypes[req.DestType] {
		return errors.New("dest_type must be 'connection' or 'agent'")
	}
	if req.DestID == "" {
		return errors.New("dest_id is required")
	}
	if req.DestPath == "" {
		return errors.New("dest_path is required")
	}
	return nil
}
