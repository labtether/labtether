package resources

import (
	"context"
	"log"
	"runtime/debug"
	"time"

	"github.com/labtether/labtether/internal/persistence"
)

const (
	transferPanicMessage       = "transfer failed: internal error"
	transferInterruptedMessage = "transfer interrupted by a Hub restart"
)

// runFileTransferSafely runs one background transfer. A panic in a protocol
// client must fail that transfer, not terminate the whole Hub process.
// runFileTransfer's own deferred cleanup (cancel, active-transfer removal)
// runs while the panic unwinds, before this recovery.
func (d *Deps) runFileTransferSafely(ctx context.Context, cancel context.CancelFunc, transferID string, req fileTransferStartRequest) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		log.Printf("file-transfers: transfer %s panicked: %v\n%s", transferID, recovered, debug.Stack())
		if err := d.failActiveFileTransfer(transferID, transferPanicMessage); err != nil {
			log.Printf("file-transfers: failed to mark panicked transfer %s as failed: %v", transferID, err)
		}
	}()
	d.runFileTransfer(ctx, cancel, transferID, req)
}

// failActiveFileTransfer records a failure unless the transfer already reached
// a terminal state.
func (d *Deps) failActiveFileTransfer(transferID, message string) error {
	if d == nil || d.FileTransferStore == nil {
		return nil
	}
	ctx := context.Background()
	ft, err := d.FileTransferStore.GetFileTransfer(ctx, transferID)
	if err != nil {
		return err
	}
	if !fileTransferActive(ft.Status) {
		return nil
	}
	markFileTransferFailed(ft, message)
	return d.FileTransferStore.UpdateFileTransfer(ctx, ft)
}

// FailInterruptedFileTransfers marks transfers that a previous Hub process
// left pending or in progress as failed. Transfers run inside the Hub process
// that started them, so after a restart none of them can still be running.
// Call it once at startup, after this process owns the Hub runtime.
func FailInterruptedFileTransfers(ctx context.Context, store persistence.FileTransferStore) (int, error) {
	if store == nil {
		return 0, nil
	}
	active, err := store.ListActiveFileTransfers(ctx)
	if err != nil {
		return 0, err
	}
	marked := 0
	for i := range active {
		ft := active[i]
		if !fileTransferActive(ft.Status) {
			continue
		}
		markFileTransferFailed(&ft, transferInterruptedMessage)
		if err := store.UpdateFileTransfer(ctx, &ft); err != nil {
			return marked, err
		}
		marked++
	}
	return marked, nil
}

func fileTransferActive(status string) bool {
	return status == "pending" || status == "in_progress"
}

func markFileTransferFailed(ft *persistence.FileTransfer, message string) {
	now := time.Now().UTC()
	ft.Status = "failed"
	ft.Error = &message
	ft.CompletedAt = &now
}
