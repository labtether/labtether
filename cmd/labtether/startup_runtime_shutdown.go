package main

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/persistence"
	"log"
	"time"
)

const hubRuntimeShutdownTimeout = 30 * time.Second

type hubRuntimeLeaseReleaser interface {
	Release(context.Context) error
}

type hubPostgresCloser interface {
	Close()
}

// releaseHubRuntimeLease releases the single-active fence only after every
// tracked runtime worker has stopped. If the bounded drain times out, keeping
// the lease until process exit prevents a replacement hub from starting while
// runaway work from this runtime can still touch shared state.
func releaseHubRuntimeLease(runtimeDrained bool, runtimeLease hubRuntimeLeaseReleaser) {
	if runtimeLease == nil {
		return
	}
	if !runtimeDrained {
		log.Printf("labtether warning: runtime drain incomplete; keeping live runtime lease until process exit")
		return
	}
	releaseCtx, stopRelease := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopRelease()
	if err := runtimeLease.Release(releaseCtx); err != nil && !errors.Is(err, persistence.ErrHubRuntimeLeaseLost) {
		log.Printf("labtether warning: live runtime lease release failed")
	}
}

// closeHubPostgresStore closes the pool only after every tracked runtime worker
// has stopped using it. pgxpool.Close waits for checked-out connections, so
// calling it after a timed-out drain both exposes workers to a closed pool and
// defeats the shutdown bound. Process exit safely closes the pool in that
// exceptional path while the runtime lease remains held until the same exit.
func closeHubPostgresStore(runtimeDrained bool, pgStore hubPostgresCloser) {
	if pgStore == nil {
		return
	}
	if !runtimeDrained {
		log.Printf("labtether warning: runtime drain incomplete; keeping postgres pool available until process exit")
		return
	}
	pgStore.Close()
}

// finalizeHubRuntimeDrain combines the HTTP and managed-worker drain results.
// Once HTTP reports an incomplete drain, waiting for other workers cannot make
// the process safe to hand over: cancel all remaining admissions and return
// immediately so main terminates the process while the lease and pool remain
// fenced until that exit.
func finalizeHubRuntimeDrain(
	srv *apiServer,
	stopRuntime context.CancelFunc,
	timeout time.Duration,
	httpConnectionsDrained bool,
) bool {
	if !httpConnectionsDrained {
		beginHubRuntimeShutdown(srv, stopRuntime)
		log.Printf("labtether warning: HTTP drain incomplete; forcing process termination with runtime resources fenced")
		return false
	}
	return shutdownHubRuntime(srv, stopRuntime, timeout)
}

func beginHubRuntimeShutdown(srv *apiServer, stopRuntime context.CancelFunc) <-chan struct{} {
	var collectorsDone <-chan struct{}
	if srv != nil && srv.collectorsDeps != nil {
		collectorsDone = srv.collectorsDeps.BeginCollectorShutdown()
	} else {
		idle := make(chan struct{})
		close(idle)
		collectorsDone = idle
	}
	if stopRuntime != nil {
		stopRuntime()
	}
	return collectorsDone
}

// shutdownHubRuntime closes collector admission before canceling the runtime,
// then waits for both managed background loops and already-active collector
// executions. PostgreSQL and the runtime lease are deferred outside this
// function, so they stay available until this bounded drain completes.
func shutdownHubRuntime(srv *apiServer, stopRuntime context.CancelFunc, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = hubRuntimeShutdownTimeout
	}

	collectorsDone := beginHubRuntimeShutdown(srv, stopRuntime)

	backgroundDone := make(chan struct{})
	go func() {
		if srv != nil {
			srv.backgroundWG.Wait()
		}
		close(backgroundDone)
	}()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for backgroundDone != nil || collectorsDone != nil {
		select {
		case <-backgroundDone:
			backgroundDone = nil
		case <-collectorsDone:
			collectorsDone = nil
		case <-deadline.C:
			log.Printf("labtether warning: runtime shutdown drain timed out after %s", timeout)
			return false
		}
	}
	return true
}
