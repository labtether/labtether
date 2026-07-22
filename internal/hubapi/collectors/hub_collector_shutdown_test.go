package collectors

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

type cancelBlockingPBSClient struct {
	usageStarted    chan struct{}
	postCancelCalls atomic.Int32
}

func (c *cancelBlockingPBSClient) ListDatastores(context.Context) ([]pbs.Datastore, error) {
	return []pbs.Datastore{{Store: "qa-store"}}, nil
}

func (c *cancelBlockingPBSClient) ListDatastoreUsage(ctx context.Context) ([]pbs.DatastoreUsage, error) {
	close(c.usageStarted)
	<-ctx.Done()
	return nil, context.Cause(ctx)
}

func (c *cancelBlockingPBSClient) GetVersion(context.Context) (pbs.Version, error) {
	c.postCancelCalls.Add(1)
	return pbs.Version{}, nil
}

func (c *cancelBlockingPBSClient) GetDatastoreStatus(context.Context, string, bool) (pbs.DatastoreStatus, error) {
	c.postCancelCalls.Add(1)
	return pbs.DatastoreStatus{}, nil
}

func (c *cancelBlockingPBSClient) ListDatastoreGroups(context.Context, string) ([]pbs.BackupGroup, error) {
	c.postCancelCalls.Add(1)
	return nil, nil
}

func (c *cancelBlockingPBSClient) ListDatastoreSnapshots(context.Context, string) ([]pbs.BackupSnapshot, error) {
	c.postCancelCalls.Add(1)
	return nil, nil
}

func (c *cancelBlockingPBSClient) ListNodeTasks(context.Context, string, int) ([]pbs.Task, error) {
	c.postCancelCalls.Add(1)
	return nil, nil
}

func TestBeginCollectorShutdownDrainsAdmittedRunAndRejectsNewRun(t *testing.T) {
	const collectorType = "test-collector-shutdown-gate"
	original, hadOriginal := CollectorExecutorRegistry[collectorType]
	started := make(chan struct{})
	release := make(chan struct{})
	CollectorExecutorRegistry[collectorType] = func(_ *Deps, _ context.Context, _ hubcollector.Collector) {
		close(started)
		<-release
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		if hadOriginal {
			CollectorExecutorRegistry[collectorType] = original
		} else {
			delete(CollectorExecutorRegistry, collectorType)
		}
	})

	deps := &Deps{DetectLinkSuggestions: func() error { return nil }}
	first := hubcollector.Collector{ID: "first", CollectorType: collectorType}
	if !deps.startCollectorRun(context.Background(), first, false) {
		t.Fatal("first collector was not admitted")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first collector did not start")
	}

	idle := deps.BeginCollectorShutdown()
	second := hubcollector.Collector{ID: "second", CollectorType: collectorType}
	if deps.startCollectorRun(context.Background(), second, false) {
		t.Fatal("collector was admitted after shutdown gate closed")
	}
	select {
	case <-idle:
		t.Fatal("shutdown drain completed while first collector was active")
	default:
	}
	close(release)
	select {
	case <-idle:
	case <-time.After(time.Second):
		t.Fatal("shutdown drain did not complete after active collector exited")
	}
}

func TestPBSCollectorLeaseLossCancellationStopsFollowOnCallsAndDrains(t *testing.T) {
	const collectorType = "test-pbs-lease-loss-cancellation"
	original, hadOriginal := CollectorExecutorRegistry[collectorType]
	client := &cancelBlockingPBSClient{usageStarted: make(chan struct{})}
	CollectorExecutorRegistry[collectorType] = func(d *Deps, ctx context.Context, collector hubcollector.Collector) {
		lifecycle := NewCollectorLifecycle(d, collector, "pbs", hubcollector.CollectorTypePBS)
		d.executePBSCollectorWithClient(ctx, collector, lifecycle, "https://pbs.qa.invalid:8007", client)
	}
	t.Cleanup(func() {
		if hadOriginal {
			CollectorExecutorRegistry[collectorType] = original
		} else {
			delete(CollectorExecutorRegistry, collectorType)
		}
	})

	var linkSuggestionCalls atomic.Int32
	deps := &Deps{
		DetectLinkSuggestions: func() error {
			linkSuggestionCalls.Add(1)
			return nil
		},
	}
	runtimeCtx, cancelRuntime := context.WithCancelCause(context.Background())
	collector := hubcollector.Collector{ID: "pbs-lease-loss", CollectorType: collectorType}
	if !deps.startCollectorRun(runtimeCtx, collector, false) {
		t.Fatal("PBS collector was not admitted")
	}
	select {
	case <-client.usageStarted:
	case <-time.After(time.Second):
		t.Fatal("PBS collector did not reach the blocking usage request")
	}

	// Mirror shutdownHubRuntime: close admission first, then cancel the shared
	// runtime context. Only after the returned drain closes may the pool close.
	idle := deps.BeginCollectorShutdown()
	cancelRuntime(persistence.ErrHubRuntimeLeaseLost)
	select {
	case <-idle:
	case <-time.After(time.Second):
		t.Fatal("PBS collector did not drain after runtime lease loss")
	}

	if got := client.postCancelCalls.Load(); got != 0 {
		t.Fatalf("PBS collector started %d follow-on API calls after cancellation", got)
	}
	if got := linkSuggestionCalls.Load(); got != 0 {
		t.Fatalf("canceled collector scheduled %d post-run link suggestion writes", got)
	}
	deps.LinkSuggestionScanMu.Lock()
	linkScanStarted := !deps.LinkSuggestionScanLastStarted.IsZero()
	deps.LinkSuggestionScanMu.Unlock()
	if linkScanStarted {
		t.Fatal("canceled collector marked a link suggestion scan as started")
	}
	deps.CollectorLifecycleMu.Lock()
	activeRuns := deps.CollectorActiveRuns
	deps.CollectorLifecycleMu.Unlock()
	if activeRuns != 0 {
		t.Fatalf("collector drain closed with %d active runs", activeRuns)
	}
}
