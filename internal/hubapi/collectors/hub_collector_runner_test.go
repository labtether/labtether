package collectors_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestExecuteCollectorUnsupportedTypeUpdatesStatus(t *testing.T) {
	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store

	collector := hubcollector.Collector{
		ID:            "collector-unsupported",
		CollectorType: "unknown-type",
		Enabled:       true,
	}

	sut.CollectorsDeps().ExecuteCollector(context.Background(), collector)

	updated, ok, err := store.GetHubCollector(collector.ID)
	if err != nil {
		t.Fatalf("GetHubCollector() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected collector status update to be recorded")
	}
	if updated.LastStatus != "error" {
		t.Fatalf("LastStatus = %q, want error", updated.LastStatus)
	}
	if updated.LastError != "unsupported type: unknown-type" {
		t.Fatalf("LastError = %q, want unsupported type message", updated.LastError)
	}
}

func TestExecuteCollectorRecoversPanicAndUpdatesStatus(t *testing.T) {
	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	sut.ConnectorRegistry.Register(&testutil.FakeDockerDiscoverConnector{
		Assets: []connectorsdk.Asset{
			{
				ID:     "docker-ct-panic",
				Type:   "docker-container",
				Name:   "panic-target",
				Source: "docker",
			},
		},
	})

	// Force a panic in downstream collector processing to verify executeCollector
	// recovery and status updates stay intact.
	sut.AssetStore = nil

	collector := hubcollector.Collector{
		ID:              "collector-panic",
		AssetID:         "docker-cluster-panic",
		CollectorType:   hubcollector.CollectorTypeDocker,
		Enabled:         true,
		IntervalSeconds: 60,
	}

	sut.CollectorsDeps().ExecuteCollector(context.Background(), collector)

	updated, ok, err := store.GetHubCollector(collector.ID)
	if err != nil {
		t.Fatalf("GetHubCollector() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected collector status update to be recorded")
	}
	if updated.LastStatus != "error" {
		t.Fatalf("LastStatus = %q, want error", updated.LastStatus)
	}
	if !strings.HasPrefix(updated.LastError, "panic: ") {
		t.Fatalf("LastError = %q, want panic prefix", updated.LastError)
	}
	if ok := sut.CollectorsDeps().TryBeginCollectorRun(collector.ID); !ok {
		t.Fatalf("expected collector run guard to be released after panic")
	}
	sut.CollectorsDeps().FinishCollectorRun(collector.ID)
}

func TestRunPendingCollectorsStartsDueCollectorsConcurrently(t *testing.T) {
	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store

	const collectorType = "test-concurrent"
	original, hadOriginal := collectorspkg.CollectorExecutorRegistry[collectorType]
	started := make(chan string, 2)
	release := make(chan struct{})
	collectorspkg.CollectorExecutorRegistry[collectorType] = func(d *collectorspkg.Deps, ctx context.Context, collector hubcollector.Collector) {
		started <- collector.ID
		<-release
		d.UpdateCollectorStatus(collector.ID, "ok", "")
	}
	t.Cleanup(func() {
		close(release)
		if hadOriginal {
			collectorspkg.CollectorExecutorRegistry[collectorType] = original
		} else {
			delete(collectorspkg.CollectorExecutorRegistry, collectorType)
		}
	})

	store.StatusByID["collector-a"] = hubcollector.Collector{ID: "collector-a", CollectorType: collectorType, Enabled: true}
	store.StatusByID["collector-b"] = hubcollector.Collector{ID: "collector-b", CollectorType: collectorType, Enabled: true}

	if startedRuns := sut.CollectorsDeps().RunPendingCollectors(context.Background()); startedRuns != 2 {
		t.Fatalf("runPendingCollectors() started %d runs, want 2", startedRuns)
	}

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out waiting for concurrent collector starts, saw=%v", seen)
		}
	}
}
