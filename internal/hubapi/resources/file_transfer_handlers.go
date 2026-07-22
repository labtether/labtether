package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

const fileTransferAPIPrefix = "/api/v1/file-transfers"

const (
	maxFileTransferEndpointIDBytes = 255
	maxFileTransferPathBytes       = 4096
)

// progressThrottleInterval is the minimum interval between DB progress updates.
const progressThrottleInterval = 500 * time.Millisecond

// progressThrottleBytes is the minimum bytes between DB progress updates.
const progressThrottleBytes int64 = 1 << 20 // 1 MB

var errFileConnectionAccessDenied = errors.New("file connection access denied")

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
		case http.MethodGet:
			d.handleListFileTransfers(w, r)
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

// --- List Transfers ---

func (d *Deps) handleListFileTransfers(w http.ResponseWriter, r *http.Request) {
	status, limit, offset, ok := parseFileTransferListQuery(w, r)
	if !ok {
		return
	}

	transfers, total, err := d.FileTransferStore.ListFileTransfers(
		r.Context(),
		d.fileTransferActorID(r.Context()),
		status,
		limit,
		offset,
	)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list file transfers")
		return
	}
	if transfers == nil {
		transfers = []persistence.FileTransfer{}
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"transfers": transfers,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

func parseFileTransferListQuery(w http.ResponseWriter, r *http.Request) (status string, limit, offset int, ok bool) {
	limit = persistence.FileTransferListDefaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > persistence.FileTransferListMaxLimit {
			servicehttp.WriteError(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return "", 0, 0, false
		}
		limit = parsed
	}

	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 || parsed > persistence.FileTransferListMaxOffset {
			servicehttp.WriteError(w, http.StatusBadRequest, "offset must be between 0 and 10000")
			return "", 0, 0, false
		}
		offset = parsed
	}

	status = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	switch status {
	case "", "pending", "in_progress", "completed", "failed":
		return status, limit, offset, true
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "status must be pending, in_progress, completed, or failed")
		return "", 0, 0, false
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
	var req fileTransferStartRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.SourceType = strings.TrimSpace(req.SourceType)
	req.SourceType = strings.ToLower(req.SourceType)
	req.SourceID = strings.TrimSpace(req.SourceID)
	req.SourcePath = strings.TrimSpace(req.SourcePath)
	req.DestType = strings.TrimSpace(req.DestType)
	req.DestType = strings.ToLower(req.DestType)
	req.DestID = strings.TrimSpace(req.DestID)
	req.DestPath = strings.TrimSpace(req.DestPath)

	if err := validateTransferRequest(req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	// A transfer always reads one endpoint and writes another. Enforce both
	// capabilities here so a write-only key cannot use the transfer API to
	// exfiltrate source data.
	if !requireAPIScope(w, r, "files:read") || !requireAPIScope(w, r, "files:write") {
		return
	}

	hasConnectionEndpoint := req.SourceType == "connection" || req.DestType == "connection"
	if hasConnectionEndpoint && d.FileProtoPool == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "file protocol pool not initialized")
		return
	}
	if hasConnectionEndpoint && d.FileConnectionStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "file connection store unavailable")
		return
	}
	hasAgentEndpoint := req.SourceType == "agent" || req.DestType == "agent"
	if hasAgentEndpoint && (d.AgentMgr == nil || d.FileBridges == nil) {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "agent file transfer runtime unavailable")
		return
	}

	actorID := d.fileTransferActorID(r.Context())
	if req.SourceType == "connection" {
		if err := d.ensureCanAccessTransferConnection(r.Context(), req.SourceID, actorID); err != nil {
			d.writeTransferConnectionError(w, err)
			return
		}
	} else {
		if !requireAssetAccess(w, r, req.SourceID) {
			return
		}
		if !d.AgentMgr.IsConnected(req.SourceID) {
			servicehttp.WriteError(w, http.StatusBadGateway, "source agent disconnected")
			return
		}
	}
	if req.DestType == "connection" {
		if err := d.ensureCanAccessTransferConnection(r.Context(), req.DestID, actorID); err != nil {
			d.writeTransferConnectionError(w, err)
			return
		}
	} else {
		if !requireAssetAccess(w, r, req.DestID) {
			return
		}
		if !d.enforceAssetActionGuard(w, req.DestID) {
			return
		}
		if !d.AgentMgr.IsConnected(req.DestID) {
			servicehttp.WriteError(w, http.StatusBadGateway, "destination agent disconnected")
			return
		}
	}

	releaseAdmission, admitted := fileTransferAdmission.tryAcquire()
	if !admitted {
		writeFileOperationCapacityError(w)
		return
	}

	// Create the pending transfer record.
	ft := &persistence.FileTransfer{
		ActorID:    actorID,
		SourceType: req.SourceType,
		SourceID:   req.SourceID,
		SourcePath: req.SourcePath,
		DestType:   req.DestType,
		DestID:     req.DestID,
		DestPath:   req.DestPath,
		FileName:   remoteFileBase(req.SourcePath),
		Status:     "pending",
	}
	if err := d.FileTransferStore.CreateFileTransfer(r.Context(), ft); err != nil {
		releaseAdmission()
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create file transfer record")
		return
	}

	// Transfers intentionally outlive the initiating HTTP request, but each one
	// still has a hard execution ceiling so stalled remote servers cannot retain
	// goroutines and pooled connections indefinitely.
	transferCtx, cancelTransfer := context.WithTimeout(context.WithoutCancel(r.Context()), fileproto.MaxOperationDuration)
	if d.ActiveTransfers != nil {
		d.ActiveTransfers.Store(ft.ID, cancelTransfer)
	}
	go func() {
		defer releaseAdmission()
		d.runFileTransfer(transferCtx, cancelTransfer, ft.ID, req)
	}() // #nosec G118 -- File transfers intentionally outlive the initiating HTTP request and use an explicit cancel handle plus bounded admission.

	servicehttp.WriteJSON(w, http.StatusAccepted, map[string]any{"transfer": ft})
}

func (d *Deps) ensureCanAccessTransferConnection(ctx context.Context, connectionID, actorID string) error {
	fc, err := d.FileConnectionStore.GetFileConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	if d.PrincipalActorID != nil && strings.TrimSpace(fc.ActorID) != strings.TrimSpace(actorID) {
		return errFileConnectionAccessDenied
	}
	return nil
}

func (d *Deps) writeTransferConnectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, persistence.ErrNotFound):
		servicehttp.WriteError(w, http.StatusNotFound, "file connection not found")
	case errors.Is(err, errFileConnectionAccessDenied):
		servicehttp.WriteError(w, http.StatusForbidden, "file connection access denied")
	default:
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load file connection")
	}
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
	transfer, err := d.FileTransferStore.GetFileTransfer(bgCtx, transferID)
	if err != nil {
		log.Printf("file-transfers: failed to load transfer %s: %v", transferID, err)
		return
	}
	actorID := strings.TrimSpace(transfer.ActorID)

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
	if req.SourceType == "agent" || req.DestType == "agent" {
		d.runAgentBackedFileTransfer(ctx, transferID, req, actorID, markFailed)
		return
	}

	// Resolve source connection config.
	srcConfig, err := d.resolveConnectionConfig(bgCtx, req.SourceID, actorID)
	if err != nil {
		markFailed("failed to resolve source connection: " + err.Error())
		return
	}

	// Resolve dest connection config.
	dstConfig, err := d.resolveConnectionConfig(bgCtx, req.DestID, actorID)
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
		} else if errors.Is(transferErr, context.DeadlineExceeded) {
			errStr := "transfer timed out"
			ft.Error = &errStr
		} else {
			errStr := transferErr.Error()
			ft.Error = &errStr
		}
	} else {
		ft.Status = "completed"
		fileSize := transferred
		ft.FileSize = &fileSize
	}

	if err := d.FileTransferStore.UpdateFileTransfer(bgCtx, ft); err != nil {
		log.Printf("file-transfers: failed to update transfer %s final status: %v", transferID, err)
	}
}

func (d *Deps) runAgentBackedFileTransfer(
	ctx context.Context,
	transferID string,
	req fileTransferStartRequest,
	actorID string,
	markFailed func(string),
) {
	bgCtx := context.Background()
	ft, err := d.FileTransferStore.GetFileTransfer(bgCtx, transferID)
	if err != nil {
		log.Printf("file-transfers: failed to load transfer %s: %v", transferID, err)
		return
	}
	startedAt := time.Now().UTC()
	ft.Status = "in_progress"
	ft.StartedAt = &startedAt
	if err := d.FileTransferStore.UpdateFileTransfer(bgCtx, ft); err != nil {
		log.Printf("file-transfers: failed to mark transfer %s as in_progress: %v", transferID, err)
		return
	}

	progressFn := d.fileTransferProgressCallback(transferID)
	var source io.ReadCloser
	var sourceSize int64
	var sourceCleanup func()

	if req.SourceType == "agent" {
		sourceFile, size, cleanup, readErr := d.spoolAgentTransferSource(ctx, req.SourceID, req.SourcePath)
		if readErr != nil {
			markFailed("failed to read source agent: " + readErr.Error())
			return
		}
		source = sourceFile
		sourceSize = size
		sourceCleanup = cleanup
	} else {
		srcConfig, resolveErr := d.resolveConnectionConfig(bgCtx, req.SourceID, actorID)
		if resolveErr != nil {
			markFailed("failed to resolve source connection: " + resolveErr.Error())
			return
		}
		srcPoolID := "transfer-src-" + transferID
		srcFS, connectErr := d.FileProtoPool.Get(ctx, srcPoolID, srcConfig)
		if connectErr != nil {
			markFailed("failed to connect to source: " + connectErr.Error())
			return
		}
		sourceCleanup = func() { d.FileProtoPool.Remove(srcPoolID) }
		source, sourceSize, err = srcFS.Read(ctx, req.SourcePath)
		if err != nil {
			sourceCleanup()
			markFailed("failed to read source: " + err.Error())
			return
		}
		if sourceSize > fileproto.MaxTransferBytes {
			_ = source.Close()
			sourceCleanup()
			markFailed(fileproto.ErrTransferTooLarge.Error())
			return
		}
	}
	defer sourceCleanup()
	defer source.Close()

	var transferred int64
	var transferErr error
	if req.DestType == "agent" {
		transferred, transferErr = d.writeAgentTransferDestination(ctx, req.DestID, req.DestPath, source, sourceSize, progressFn)
	} else {
		dstConfig, resolveErr := d.resolveConnectionConfig(bgCtx, req.DestID, actorID)
		if resolveErr != nil {
			markFailed("failed to resolve destination connection: " + resolveErr.Error())
			return
		}
		dstPoolID := "transfer-dst-" + transferID
		dstFS, connectErr := d.FileProtoPool.Get(ctx, dstPoolID, dstConfig)
		if connectErr != nil {
			markFailed("failed to connect to destination: " + connectErr.Error())
			return
		}
		defer d.FileProtoPool.Remove(dstPoolID)
		progressReader := &fileTransferProgressReader{ctx: ctx, reader: newFileTransferBoundedReader(source), total: sourceSize, progress: progressFn}
		transferErr = dstFS.Write(ctx, req.DestPath, progressReader, sourceSize)
		transferred = progressReader.transferred
		if transferErr == nil {
			transferErr = progressReader.terminalError()
		}
	}

	d.finalizeFileTransfer(transferID, transferred, transferErr)
}

func (d *Deps) fileTransferProgressCallback(transferID string) fileproto.TransferProgress {
	var mu sync.Mutex
	lastUpdate := time.Now()
	var lastBytes int64
	return func(bytesTransferred, totalSize int64) {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(lastUpdate) < progressThrottleInterval && bytesTransferred-lastBytes < progressThrottleBytes {
			return
		}
		lastUpdate = time.Now()
		lastBytes = bytesTransferred
		ft, err := d.FileTransferStore.GetFileTransfer(context.Background(), transferID)
		if err != nil {
			return
		}
		ft.BytesTransferred = bytesTransferred
		if totalSize >= 0 {
			ft.FileSize = &totalSize
		}
		_ = d.FileTransferStore.UpdateFileTransfer(context.Background(), ft)
	}
}

func (d *Deps) finalizeFileTransfer(transferID string, transferred int64, transferErr error) {
	ft, err := d.FileTransferStore.GetFileTransfer(context.Background(), transferID)
	if err != nil {
		log.Printf("file-transfers: failed to load transfer %s for final update: %v", transferID, err)
		return
	}
	completedAt := time.Now().UTC()
	ft.BytesTransferred = transferred
	ft.CompletedAt = &completedAt
	if transferErr == nil {
		ft.Status = "completed"
		ft.Error = nil
		fileSize := transferred
		ft.FileSize = &fileSize
	} else {
		ft.Status = "failed"
		errMessage := transferErr.Error()
		switch {
		case errors.Is(transferErr, context.Canceled):
			errMessage = "cancelled"
		case errors.Is(transferErr, context.DeadlineExceeded):
			errMessage = "transfer timed out"
		}
		ft.Error = &errMessage
	}
	if err := d.FileTransferStore.UpdateFileTransfer(context.Background(), ft); err != nil {
		log.Printf("file-transfers: failed to update transfer %s final status: %v", transferID, err)
	}
}

func (d *Deps) spoolAgentTransferSource(ctx context.Context, assetID, sourcePath string) (*os.File, int64, func(), error) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return nil, 0, nil, errors.New("source agent disconnected")
	}
	requestID := generateRequestID()
	bridge := newFileBridge(64, assetID)
	d.FileBridges.Store(requestID, bridge)
	cleanupBridge := func() {
		bridge.Close()
		d.FileBridges.Delete(requestID)
	}

	data, err := json.Marshal(agentmgr.FileReadData{RequestID: requestID, Path: sourcePath})
	if err != nil {
		cleanupBridge()
		return nil, 0, nil, err
	}
	if err := agentConn.Send(agentmgr.Message{Type: agentmgr.MsgFileRead, ID: requestID, Data: data}); err != nil {
		cleanupBridge()
		return nil, 0, nil, fmt.Errorf("send file read request: %w", err)
	}

	spool, err := os.CreateTemp("", "labtether-transfer-*")
	if err != nil {
		cleanupBridge()
		return nil, 0, nil, errors.New("create transfer spool")
	}
	cleanup := func() {
		cleanupBridge()
		_ = spool.Close()
		_ = os.Remove(spool.Name())
	}
	var written int64
	for {
		chunk, receiveErr := receiveAgentTransferChunk(ctx, bridge)
		if receiveErr != nil {
			cleanup()
			return nil, written, nil, receiveErr
		}
		if chunk.Error != "" {
			cleanup()
			return nil, written, nil, errors.New(chunk.Error)
		}
		payload, decodeErr := DecodeFileDownloadChunk(chunk)
		if decodeErr != nil {
			cleanup()
			return nil, written, nil, errFileDownloadInvalidAgentResponse
		}
		if writeErr := writeFileDownloadChunk(spool, payload, chunk.Offset, &written, fileproto.MaxTransferBytes); writeErr != nil {
			cleanup()
			return nil, written, nil, writeErr
		}
		if chunk.Done {
			break
		}
	}
	if err := spool.Sync(); err != nil {
		cleanup()
		return nil, written, nil, errors.New("sync transfer spool")
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, written, nil, errors.New("rewind transfer spool")
	}
	return spool, written, cleanup, nil
}

func receiveAgentTransferChunk(ctx context.Context, bridge *FileBridge) (agentmgr.FileDataPayload, error) {
	timer := time.NewTimer(fileRequestTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return agentmgr.FileDataPayload{}, ctx.Err()
	case msg := <-bridge.Ch:
		var chunk agentmgr.FileDataPayload
		if err := json.Unmarshal(msg.Data, &chunk); err != nil {
			return agentmgr.FileDataPayload{}, errFileDownloadInvalidAgentResponse
		}
		return chunk, nil
	case <-bridge.Done:
		if err := bridge.Err(); err != nil {
			return agentmgr.FileDataPayload{}, err
		}
		return agentmgr.FileDataPayload{}, errors.New("agent file stream closed")
	case <-timer.C:
		return agentmgr.FileDataPayload{}, errFileDownloadTimedOut
	}
}

func (d *Deps) writeAgentTransferDestination(
	ctx context.Context,
	assetID string,
	destPath string,
	source io.Reader,
	totalSize int64,
	progress fileproto.TransferProgress,
) (int64, error) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return 0, errors.New("destination agent disconnected")
	}
	requestID := generateRequestID()
	bridge := newFileBridge(1, assetID)
	d.FileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer d.FileBridges.Delete(requestID)

	progressReader := &fileTransferProgressReader{
		ctx:      ctx,
		reader:   newFileTransferBoundedReader(source),
		total:    totalSize,
		progress: progress,
	}
	sent, err := RelayFileUploadChunks(progressReader, requestID, destPath, fileChunkSizeHub, func(payload agentmgr.FileWriteData) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case msg := <-bridge.Ch:
			var result agentmgr.FileWrittenData
			if err := json.Unmarshal(msg.Data, &result); err != nil {
				return UploadAgentResponseError{err: err}
			}
			if result.Error != "" {
				return UploadAgentWriteError{message: result.Error}
			}
			return UploadAgentWriteError{message: "destination agent completed upload before source reached EOF"}
		default:
		}
		data, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return marshalErr
		}
		return agentConn.Send(agentmgr.Message{Type: agentmgr.MsgFileWrite, ID: requestID, Data: data})
	})
	if err != nil {
		return sent, err
	}
	if err := progressReader.terminalError(); err != nil {
		return sent, err
	}

	timer := time.NewTimer(fileRequestTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return sent, ctx.Err()
	case msg := <-bridge.Ch:
		var result agentmgr.FileWrittenData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			return sent, errFileDownloadInvalidAgentResponse
		}
		if result.Error != "" {
			return sent, errors.New(result.Error)
		}
		if result.BytesWritten != sent {
			return sent, fmt.Errorf("destination agent byte count mismatch: wrote %d, expected %d", result.BytesWritten, sent)
		}
		return sent, nil
	case <-bridge.Done:
		if err := bridge.Err(); err != nil {
			return sent, err
		}
		return sent, errors.New("destination agent response stream closed")
	case <-timer.C:
		return sent, errors.New("destination agent did not confirm file write in time")
	}
}

type fileTransferBoundedReader struct {
	reader      io.Reader
	remaining   int64
	terminalErr error
	finished    bool
}

func newFileTransferBoundedReader(reader io.Reader) *fileTransferBoundedReader {
	return &fileTransferBoundedReader{reader: reader, remaining: fileproto.MaxTransferBytes}
}

func (r *fileTransferBoundedReader) Read(payload []byte) (int, error) {
	if r.terminalErr != nil {
		return 0, r.terminalErr
	}
	if r.finished {
		return 0, io.EOF
	}
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			r.terminalErr = fileproto.ErrTransferTooLarge
			return 0, r.terminalErr
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				r.finished = true
			}
			return 0, err
		}
		return 0, nil
	}
	if int64(len(payload)) > r.remaining {
		payload = payload[:r.remaining]
	}
	n, err := r.reader.Read(payload)
	r.remaining -= int64(n)
	if err != nil {
		if errors.Is(err, io.EOF) {
			r.finished = true
		} else {
			r.terminalErr = err
		}
	}
	return n, err
}

type fileTransferProgressReader struct {
	ctx         context.Context
	reader      *fileTransferBoundedReader
	transferred int64
	total       int64
	progress    fileproto.TransferProgress
}

func (r *fileTransferProgressReader) Read(payload []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(payload)
	r.transferred += int64(n)
	if n > 0 && r.progress != nil {
		r.progress(r.transferred, r.total)
	}
	return n, err
}

func (r *fileTransferProgressReader) terminalError() error {
	return r.reader.terminalErr
}

func remoteFileBase(value string) string {
	normalized := strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(value), `\`, "/"), "/")
	if normalized == "" {
		return "transfer"
	}
	parts := strings.Split(normalized, "/")
	if parts[len(parts)-1] == "" {
		return "transfer"
	}
	return parts[len(parts)-1]
}

// resolveConnectionConfig loads a file connection by ID and builds its ConnectionConfig.
func (d *Deps) resolveConnectionConfig(ctx context.Context, connectionID, actorID string) (fileproto.ConnectionConfig, error) {
	fc, err := d.FileConnectionStore.GetFileConnection(ctx, connectionID)
	if err != nil {
		return fileproto.ConnectionConfig{}, err
	}
	if d.PrincipalActorID != nil && strings.TrimSpace(fc.ActorID) != strings.TrimSpace(actorID) {
		return fileproto.ConnectionConfig{}, fmt.Errorf("file connection access denied")
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
	if len(req.SourceID) > maxFileTransferEndpointIDBytes {
		return fmt.Errorf("source_id exceeds %d byte limit", maxFileTransferEndpointIDBytes)
	}
	if req.SourcePath == "" {
		return errors.New("source_path is required")
	}
	if len(req.SourcePath) > maxFileTransferPathBytes || strings.ContainsRune(req.SourcePath, '\x00') {
		return fmt.Errorf("source_path must be a valid path of at most %d bytes", maxFileTransferPathBytes)
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
	if len(req.DestID) > maxFileTransferEndpointIDBytes {
		return fmt.Errorf("dest_id exceeds %d byte limit", maxFileTransferEndpointIDBytes)
	}
	if req.DestPath == "" {
		return errors.New("dest_path is required")
	}
	if len(req.DestPath) > maxFileTransferPathBytes || strings.ContainsRune(req.DestPath, '\x00') {
		return fmt.Errorf("dest_path must be a valid path of at most %d bytes", maxFileTransferPathBytes)
	}
	return nil
}
