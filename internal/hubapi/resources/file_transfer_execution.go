package resources

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/fileproto"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

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
