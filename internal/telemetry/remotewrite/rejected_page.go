package remotewrite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	MaxRejectedPages        = 64
	MaxRejectedPayloadBytes = 64 * 1024 * 1024
)

var (
	ErrRejectedStoreFull    = errors.New("remotewrite: rejected-page storage is full; export blocked until operator recovery")
	ErrRejectedOwnerChanged = errors.New("remotewrite: rejected-page endpoint, cursor, or payload does not match durable state")
)

// RejectedPage preserves the exact sent snappy WriteRequest, not credentials or
// the receiver body. A receiver may have accepted part of a rejected request;
// this is a durable rejected/partial-unknown disposition, never delivery proof.
type RejectedPage struct {
	Previous    Cursor
	Next        Cursor
	StatusCode  int
	SampleCount int
	Payload     []byte
	RejectedAt  time.Time
}

func (p RejectedPage) Validate() error {
	if !p.Previous.valid() || !p.Next.valid() || !p.Next.notBehind(p.Previous) || !p.Next.advances(p.Previous) ||
		!permanentReceiverStatus(p.StatusCode) || p.SampleCount < 1 || p.SampleCount > MaxSamplesPerRequest ||
		len(p.Payload) == 0 || len(p.Payload) > MaxRequestBodyBytes || p.RejectedAt.IsZero() {
		return fmt.Errorf("remotewrite: invalid rejected page")
	}
	return nil
}

func permanentReceiverStatus(status int) bool { return status >= 400 && status < 500 && status != 429 }

type pendingRejectedPage struct {
	page RejectedPage
	more bool
}

// Keep one bounded rejected body while persistence is unavailable/full. Retrying
// this operation must not resend a known permanent rejection. The store must
// recognize an already committed exact page after a lost commit response.
func (w *Worker) persistRejectedPage(ctx context.Context) (bool, error) {
	pending := w.pendingRejection
	if err := w.cursor.RejectRemoteWritePage(ctx, w.fingerprint, pending.page); err != nil {
		return false, err
	}
	w.pendingRejection = nil
	slog.Warn("remote write page retained as rejected; receiver partial acceptance unknown",
		"status", pending.page.StatusCode, "samples", pending.page.SampleCount)
	return pending.more, nil
}
