package resources

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strconv"
	"strings"
	"time"
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
