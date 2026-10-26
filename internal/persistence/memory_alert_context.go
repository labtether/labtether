package persistence

import (
	"context"
	"sync"
	"time"
)

// Admit a Memory-store read or write without leaving a waiter goroutine behind
// after cancellation. Callers hold the lock through their existing operation.
func lockAlertEvaluation(ctx context.Context, mu *sync.RWMutex, read bool) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		locked := false
		if read {
			locked = mu.TryRLock()
		} else {
			locked = mu.TryLock()
		}
		if locked {
			if err := ctx.Err(); err != nil {
				if read {
					mu.RUnlock()
				} else {
					mu.Unlock()
				}
				return err
			}
			return nil
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
