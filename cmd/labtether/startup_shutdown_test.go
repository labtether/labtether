package main

import (
	"context"
	"testing"
	"time"

	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

type shutdownLeaseStub struct {
	released bool
}

func (s *shutdownLeaseStub) Release(context.Context) error {
	s.released = true
	return nil
}

type shutdownPostgresStub struct {
	closed bool
}

func (s *shutdownPostgresStub) Close() {
	s.closed = true
}

type shutdownLeaseErrorStub struct {
	err error
}

func (s *shutdownLeaseErrorStub) Release(context.Context) error {
	return s.err
}

func TestRuntimeResourcesReleasedOnlyAfterSuccessfulDrain(t *testing.T) {
	lease := &shutdownLeaseStub{}
	store := &shutdownPostgresStub{}

	releaseHubRuntimeLease(true, lease)
	closeHubPostgresStore(true, store)

	if !lease.released {
		t.Fatal("runtime lease was not released after a successful drain")
	}
	if !store.closed {
		t.Fatal("postgres store was not closed after a successful drain")
	}
}

func TestRuntimeResourcesRemainFencedWhenDrainTimesOut(t *testing.T) {
	lease := &shutdownLeaseStub{}
	store := &shutdownPostgresStub{}

	releaseHubRuntimeLease(false, lease)
	closeHubPostgresStore(false, store)

	if lease.released {
		t.Fatal("runtime lease released while tracked work could still be active")
	}
	if store.closed {
		t.Fatal("postgres store closed while tracked work could still be active")
	}
}

func TestFinalizeHubRuntimeDrainFailsClosedAfterHTTPDrainError(t *testing.T) {
	runtimeStopped := false
	if ok := finalizeHubRuntimeDrain(nil, func() { runtimeStopped = true }, time.Second, false); ok {
		t.Fatal("combined runtime drain unexpectedly succeeded after HTTP drain failure")
	}
	if !runtimeStopped {
		t.Fatal("managed runtime was not canceled before forced process termination")
	}

	lease := &shutdownLeaseStub{}
	store := &shutdownPostgresStub{}
	releaseHubRuntimeLease(false, lease)
	closeHubPostgresStore(false, store)
	if lease.released {
		t.Fatal("runtime lease released after HTTP drain failure")
	}
	if store.closed {
		t.Fatal("postgres store closed after HTTP drain failure")
	}
}

func TestRuntimeResourceCleanupToleratesLostLeaseAndNilResources(t *testing.T) {
	releaseHubRuntimeLease(true, &shutdownLeaseErrorStub{err: persistence.ErrHubRuntimeLeaseLost})
	releaseHubRuntimeLease(true, nil)
	closeHubPostgresStore(true, nil)
}

func TestShutdownHubRuntimeWaitsForActiveCollector(t *testing.T) {
	const collectorType = "test-shutdown-drain"
	original, hadOriginal := collectorspkg.CollectorExecutorRegistry[collectorType]
	started := make(chan struct{})
	finished := make(chan struct{})
	collectorspkg.CollectorExecutorRegistry[collectorType] = func(_ *collectorspkg.Deps, ctx context.Context, _ hubcollector.Collector) {
		close(started)
		<-ctx.Done()
		close(finished)
	}
	t.Cleanup(func() {
		if hadOriginal {
			collectorspkg.CollectorExecutorRegistry[collectorType] = original
		} else {
			delete(collectorspkg.CollectorExecutorRegistry, collectorType)
		}
	})

	runtimeCtx, stopRuntime := context.WithCancel(context.Background())
	deps := &collectorspkg.Deps{
		CollectorRuntimeContext: runtimeCtx,
		DetectLinkSuggestions:   func() error { return nil },
	}
	srv := &apiServer{collectorsDeps: deps}
	collector := hubcollector.Collector{ID: "shutdown-drain", CollectorType: collectorType}
	go deps.ExecuteCollector(runtimeCtx, collector)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("collector did not start")
	}
	if ok := shutdownHubRuntime(srv, stopRuntime, time.Second); !ok {
		t.Fatal("runtime drain timed out")
	}
	select {
	case <-finished:
	default:
		t.Fatal("shutdown returned before the active collector finished")
	}
}

func TestShutdownHubRuntimeBoundsCollectorThatIgnoresCancellation(t *testing.T) {
	const collectorType = "test-shutdown-timeout"
	original, hadOriginal := collectorspkg.CollectorExecutorRegistry[collectorType]
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	collectorspkg.CollectorExecutorRegistry[collectorType] = func(_ *collectorspkg.Deps, _ context.Context, _ hubcollector.Collector) {
		close(started)
		<-release
		close(finished)
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		if hadOriginal {
			collectorspkg.CollectorExecutorRegistry[collectorType] = original
		} else {
			delete(collectorspkg.CollectorExecutorRegistry, collectorType)
		}
	})

	runtimeCtx, stopRuntime := context.WithCancel(context.Background())
	deps := &collectorspkg.Deps{
		CollectorRuntimeContext: runtimeCtx,
		DetectLinkSuggestions:   func() error { return nil },
	}
	srv := &apiServer{collectorsDeps: deps}
	collector := hubcollector.Collector{ID: "shutdown-timeout", CollectorType: collectorType}
	go deps.ExecuteCollector(runtimeCtx, collector)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("collector did not start")
	}
	startedAt := time.Now()
	if ok := shutdownHubRuntime(srv, stopRuntime, 20*time.Millisecond); ok {
		t.Fatal("runtime drain unexpectedly succeeded while collector was blocked")
	}
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("bounded runtime drain took %s", elapsed)
	}
	select {
	case <-finished:
		t.Fatal("blocked collector unexpectedly finished before release")
	default:
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("collector did not finish after release")
	}
}
