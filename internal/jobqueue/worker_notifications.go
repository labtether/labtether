package jobqueue

import (
	"context"
	"errors"
	"time"
)

// runNotificationLoop drains available work, then waits for a notification or
// the normal polling deadline. A failed connection must leave this loop: pgx
// returns immediately on a closed connection, which would otherwise hammer
// Claim and the database in a tight loop.
func runNotificationLoop(ctx context.Context, interval time.Duration, process func(context.Context) bool, wait func(context.Context) error) error {
	for {
		for ctx.Err() == nil && process(ctx) {
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		waitCtx, cancel := context.WithTimeout(ctx, interval)
		err := wait(waitCtx)
		cancel()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}
}
