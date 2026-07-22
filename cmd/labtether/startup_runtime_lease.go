package main

import (
	"context"
	"time"
)

const (
	hubRuntimeLeaseCheckInterval = time.Second
	hubRuntimeLeasePingTimeout   = 3 * time.Second
)

type runtimeLeasePinger interface {
	Ping(context.Context) error
}

// monitorHubRuntimeLease cancels the complete hub runtime as soon as the
// dedicated PostgreSQL session that owns the single-active lease is lost.
// Continuing to serve after that session drops would permit another hub to
// acquire the lock while this process still owns replica-local agent sockets.
func monitorHubRuntimeLease(
	ctx context.Context,
	lease runtimeLeasePinger,
	interval time.Duration,
	pingTimeout time.Duration,
	cancel context.CancelCauseFunc,
) {
	if lease == nil || cancel == nil {
		return
	}
	if interval <= 0 {
		interval = hubRuntimeLeaseCheckInterval
	}
	if pingTimeout <= 0 {
		pingTimeout = hubRuntimeLeasePingTimeout
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, stopPing := context.WithTimeout(ctx, pingTimeout)
			err := lease.Ping(pingCtx)
			stopPing()
			if err != nil {
				// A normal SIGTERM can cancel an in-flight database Ping. Do not
				// race that intentional cancellation and relabel it as lease loss.
				if ctx.Err() != nil {
					return
				}
				cancel(err)
				return
			}
		}
	}
}
