package remotewrite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Cursor is the durable, insertion-ordered replay position for the two
// telemetry tables. Independent IDs avoid timestamp pagination gaps when more
// than one batch shares a timestamp or delayed samples arrive out of order.
type Cursor struct {
	AssetSampleID int64
	HubSampleID   int64
}

func (c Cursor) valid() bool { return c.AssetSampleID >= 0 && c.HubSampleID >= 0 }

func (c Cursor) advances(previous Cursor) bool {
	return c.AssetSampleID > previous.AssetSampleID || c.HubSampleID > previous.HubSampleID
}

func (c Cursor) notBehind(previous Cursor) bool {
	return c.AssetSampleID >= previous.AssetSampleID && c.HubSampleID >= previous.HubSampleID
}

// Batch is one bounded replay page plus the cursor that becomes durable only
// after the receiver accepts the page.
type Batch struct {
	Samples []SampleWithLabels
	Next    Cursor
	// More indicates that the source filled its requested page and the worker
	// should continue catching up without waiting for the normal interval.
	More bool
}

// SampleSource loads insertion-ordered telemetry after a durable cursor.
type SampleSource interface {
	SamplesAfter(ctx context.Context, cursor Cursor, limit int) (Batch, error)
}

// CursorStore persists the replay position for one endpoint fingerprint.
type CursorStore interface {
	LoadRemoteWriteCursor(ctx context.Context, endpointFingerprint string) (Cursor, error)
	SaveRemoteWriteCursor(ctx context.Context, endpointFingerprint string, cursor Cursor, advancedAt time.Time) error
}

// Worker is the background push loop that periodically drains new samples
// from a SampleSource and forwards them to a Prometheus remote_write endpoint.
//
// Call Run with a cancellable context to start the loop; cancel the context to
// stop it gracefully.
type Worker struct {
	config      Config
	source      SampleSource
	cursor      CursorStore
	fingerprint string
}

// NewWorker validates all dependencies before any background work starts.
func NewWorker(config Config, source SampleSource, cursor CursorStore) (*Worker, error) {
	normalized, err := NormalizeConfig(config)
	if err != nil {
		return nil, err
	}
	if !normalized.Enabled {
		return nil, fmt.Errorf("remote write worker requires an enabled configuration")
	}
	if source == nil || cursor == nil {
		return nil, fmt.Errorf("remote write source and cursor store are required")
	}
	fingerprint, err := normalized.EndpointFingerprint()
	if err != nil {
		return nil, err
	}
	return &Worker{config: normalized, source: source, cursor: cursor, fingerprint: fingerprint}, nil
}

const (
	// pushBatchLimit caps the number of samples per push to bound request size.
	pushBatchLimit = MaxSamplesPerRequest
	// maxBackoff is the ceiling for exponential back-off on push failures.
	maxBackoff = 5 * time.Minute
	// pushOperationTimeout bounds the database read, serialization, HTTP request,
	// and durable cursor update as one operation.
	pushOperationTimeout = 25 * time.Second
	// catchUpDelay bounds a backlog drain loop while allowing substantially more
	// throughput than the steady-state collection interval.
	catchUpDelay = 100 * time.Millisecond
)

// Run starts the push loop and blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	if w == nil || ctx == nil {
		return
	}

	interval := w.config.Interval
	failureBackoff := interval
	timer := time.NewTimer(0) // Enabling the runtime drains a page immediately.
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			opCtx, cancel := context.WithTimeout(ctx, pushOperationTimeout)
			more, err := w.pushPage(opCtx)
			cancel()
			if ctx.Err() != nil {
				return
			}
			delay := interval
			if err != nil {
				// Never attach the URL, configuration, request, or credentials.
				slog.Warn("remote write push failed", "error", err)
				// Exponential back-off capped at maxBackoff.
				failureBackoff = min(failureBackoff*2, maxBackoff)
				delay = failureBackoff
			} else {
				failureBackoff = interval
				if more {
					delay = catchUpDelay
				}
			}
			timer.Reset(delay)
		}
	}
}

// pushBatch is the single-page test and compatibility wrapper.
func (w *Worker) pushBatch(ctx context.Context) error {
	_, err := w.pushPage(ctx)
	return err
}

// pushPage fetches one replay page, adaptively reduces it when either the
// local body cap or receiver rejects its size, and advances the cursor only
// after the accepted subset is durable.
func (w *Worker) pushPage(ctx context.Context) (bool, error) {
	current, err := w.cursor.LoadRemoteWriteCursor(ctx, w.fingerprint)
	if err != nil {
		return false, fmt.Errorf("remotewrite: load replay cursor: %w", err)
	}
	if !current.valid() {
		return false, fmt.Errorf("remotewrite: invalid persisted replay cursor")
	}

	pageLimit := pushBatchLimit
	for {
		batch, loadErr := w.source.SamplesAfter(ctx, current, pageLimit)
		if loadErr != nil {
			return false, fmt.Errorf("remotewrite: load replay page: %w", loadErr)
		}
		if len(batch.Samples) > pageLimit || !batch.Next.valid() || !batch.Next.notBehind(current) {
			return false, fmt.Errorf("remotewrite: invalid replay page")
		}
		if len(batch.Samples) == 0 && !batch.Next.advances(current) {
			return false, nil
		}

		if len(batch.Samples) > 0 {
			body, serializeErr := SerializeWriteRequest(batch.Samples)
			if serializeErr != nil {
				if errors.Is(serializeErr, errRequestBodyTooLarge) && pageLimit > 1 {
					pageLimit = max(1, pageLimit/2)
					continue
				}
				return false, fmt.Errorf("remotewrite: serialize replay page: %w", serializeErr)
			}
			if pushErr := Push(ctx, w.config.URL, body, w.config.Username, w.config.Password); pushErr != nil {
				var statusErr *receiverStatusError
				if errors.As(pushErr, &statusErr) && statusErr.statusCode == 413 && pageLimit > 1 {
					pageLimit = max(1, pageLimit/2)
					continue
				}
				return false, pushErr
			}
		}

		// A receiver success is not reported as a completed page until this durable
		// update succeeds. Failure causes an at-least-once retry, never a gap.
		if saveErr := w.cursor.SaveRemoteWriteCursor(ctx, w.fingerprint, batch.Next, time.Now().UTC()); saveErr != nil {
			return false, fmt.Errorf("remotewrite: persist replay cursor: %w", saveErr)
		}
		return batch.More, nil
	}
}
