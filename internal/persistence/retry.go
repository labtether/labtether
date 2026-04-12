// internal/persistence/retry.go
package persistence

import (
	"context"
	"log"
	"time"
)

// retryBackoffs defines the sleep durations between retry attempts.
var retryBackoffs = []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}

// RetryDB retries fn up to maxAttempts times with exponential backoff.
// Intended for internal background operations (heartbeat, job queue, retention)
// — NOT for user-facing API calls which should fail fast.
func RetryDB(ctx context.Context, maxAttempts int, fn func(ctx context.Context) error) error {
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if ctx.Err() != nil {
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		}
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}
		if i < maxAttempts-1 {
			backoff := retryBackoffs[0]
			if i < len(retryBackoffs) {
				backoff = retryBackoffs[i]
			}
			log.Printf("db retry: attempt %d/%d failed: %v (backoff %v)", i+1, maxAttempts, lastErr, backoff)
			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(backoff):
			}
		}
	}
	return lastErr
}
