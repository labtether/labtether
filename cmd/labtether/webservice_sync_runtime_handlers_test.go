package main

import (
	"context"
	"testing"
	"time"

	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
)

func TestRunWebServiceCleanupTicksUntilContextCancelled(t *testing.T) {
	sut := newTestAPIServer(t)

	oldInterval := collectorspkg.WebServiceCleanupInterval
	oldStep := collectorspkg.WebServiceCleanupStep
	t.Cleanup(func() {
		collectorspkg.WebServiceCleanupInterval = oldInterval
		collectorspkg.WebServiceCleanupStep = oldStep
	})

	collectorspkg.WebServiceCleanupInterval = 5 * time.Millisecond
	cleanupTick := make(chan struct{}, 4)
	collectorspkg.WebServiceCleanupStep = func(_ *collectorsDeps) {
		select {
		case cleanupTick <- struct{}{}:
		default:
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sut.runWebServiceCleanup(ctx)
	}()

	select {
	case <-cleanupTick:
		// success — at least one tick fired
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for web-service cleanup tick")
	}

	cancel()
	<-done
}
