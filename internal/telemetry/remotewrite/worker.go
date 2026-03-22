package remotewrite

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Config holds the remote_write configuration.
type Config struct {
	// Enabled controls whether the worker runs at all.
	Enabled bool
	// URL is the remote_write endpoint (e.g. https://prometheus.example.com/api/v1/write).
	URL string
	// Username and Password are optional HTTP Basic Auth credentials.
	Username string
	Password string // #nosec G117 -- Runtime remote_write credential, not a hardcoded secret.
	// Interval is the target push cadence. Defaults to 30s if zero.
	Interval time.Duration
}

func (c Config) interval() time.Duration {
	if c.Interval <= 0 {
		return 30 * time.Second
	}
	return c.Interval
}

// SampleSource provides metric samples that occurred after a given time.
// Implementations query the telemetry store and must return samples in
// ascending timestamp order.
type SampleSource interface {
	SamplesSince(ctx context.Context, since time.Time, limit int) ([]SampleWithLabels, error)
}

// HighWaterMark persists the timestamp of the last successfully pushed sample
// across worker restarts, preventing duplicate pushes and data gaps.
type HighWaterMark interface {
	Get(ctx context.Context) (time.Time, error)
	Set(ctx context.Context, t time.Time) error
}

// Worker is the background push loop that periodically drains new samples
// from a SampleSource and forwards them to a Prometheus remote_write endpoint.
//
// Call Run with a cancellable context to start the loop; cancel the context to
// stop it gracefully.
type Worker struct {
	config Config
	source SampleSource
	hwm    HighWaterMark
}

// NewWorker creates a Worker.  All three parameters are required.
func NewWorker(config Config, source SampleSource, hwm HighWaterMark) *Worker {
	return &Worker{
		config: config,
		source: source,
		hwm:    hwm,
	}
}

const (
	// pushBatchLimit caps the number of samples per push to bound request size.
	pushBatchLimit = 500
	// maxBackoff is the ceiling for exponential back-off on push failures.
	maxBackoff = 5 * time.Minute
)

// Run starts the push loop and blocks until ctx is cancelled.
// If Config.Enabled is false the function returns immediately.
func (w *Worker) Run(ctx context.Context) {
	if !w.config.Enabled {
		return
	}

	interval := w.config.interval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	backoff := interval

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.pushBatch(ctx); err != nil {
				slog.Warn("remote write push failed",
					"url", w.config.URL,
					"error", err,
				)
				// Exponential back-off capped at maxBackoff.
				backoff = min(backoff*2, maxBackoff)
				ticker.Reset(backoff)
			} else {
				// Reset to normal cadence on success.
				backoff = interval
				ticker.Reset(interval)
			}
		}
	}
}

// pushBatch fetches samples since the high-water mark, serializes them, pushes
// them, and advances the high-water mark on success.
func (w *Worker) pushBatch(ctx context.Context) error {
	since, err := w.hwm.Get(ctx)
	if err != nil {
		// Non-fatal: start from zero time (will push everything).
		slog.Warn("remote write: hwm.Get failed, starting from zero", "error", err)
		since = time.Time{}
	}

	samples, err := w.source.SamplesSince(ctx, since, pushBatchLimit)
	if err != nil {
		return fmt.Errorf("remotewrite: SamplesSince: %w", err)
	}
	if len(samples) == 0 {
		return nil
	}

	body, err := SerializeWriteRequest(samples)
	if err != nil {
		return fmt.Errorf("remotewrite: serialize: %w", err)
	}

	if err := Push(ctx, w.config.URL, body, w.config.Username, w.config.Password); err != nil {
		return err
	}

	// Advance high-water mark to the maximum timestamp in the batch so we do
	// not re-push the same samples on the next tick.
	maxTS := since
	for _, s := range samples {
		if t := TimeFromMillis(s.Timestamp); t.After(maxTS) {
			maxTS = t
		}
	}
	if err := w.hwm.Set(ctx, maxTS); err != nil {
		// Log but do not fail: the push succeeded; we may push duplicates on
		// the next tick but data is not lost.
		slog.Warn("remote write: hwm.Set failed", "error", err)
	}
	return nil
}
