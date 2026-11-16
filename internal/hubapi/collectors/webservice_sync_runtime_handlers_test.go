package collectors_test

import (
	"context"
	"testing"
	"time"

	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
)

func TestRunWebServiceCleanupTicksUntilContextCancelled(t *testing.T) {
	sut := hubdeps.New(t)

	oldInterval := collectorspkg.WebServiceCleanupInterval
	oldStep := collectorspkg.WebServiceCleanupStep
	t.Cleanup(func() {
		collectorspkg.WebServiceCleanupInterval = oldInterval
		collectorspkg.WebServiceCleanupStep = oldStep
	})

	collectorspkg.WebServiceCleanupInterval = 5 * time.Millisecond
	cleanupTick := make(chan struct{}, 4)
	collectorspkg.WebServiceCleanupStep = func(_ *collectorspkg.Deps) {
		select {
		case cleanupTick <- struct{}{}:
		default:
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sut.CollectorsDeps().RunWebServiceCleanup(ctx)
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
