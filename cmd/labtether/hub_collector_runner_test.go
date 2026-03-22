package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestExecuteCollectorUnsupportedTypeUpdatesStatus(t *testing.T) {
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	collector := hubcollector.Collector{
		ID:            "collector-unsupported",
		CollectorType: "unknown-type",
		Enabled:       true,
	}

	sut.executeCollector(context.Background(), collector)

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
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	sut.connectorRegistry.Register(&fakeDockerDiscoverConnector{
		assets: []connectorsdk.Asset{
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
	sut.assetStore = nil

	collector := hubcollector.Collector{
		ID:              "collector-panic",
		AssetID:         "docker-cluster-panic",
		CollectorType:   hubcollector.CollectorTypeDocker,
		Enabled:         true,
		IntervalSeconds: 60,
	}

	sut.executeCollector(context.Background(), collector)

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
	if ok := sut.tryBeginCollectorRun(collector.ID); !ok {
		t.Fatalf("expected collector run guard to be released after panic")
	}
	sut.finishCollectorRun(collector.ID)
}

func TestRunPendingCollectorsStartsDueCollectorsConcurrently(t *testing.T) {
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	const collectorType = "test-concurrent"
	original, hadOriginal := collectorExecutorRegistry[collectorType]
	started := make(chan string, 2)
	release := make(chan struct{})
	collectorExecutorRegistry[collectorType] = func(d *collectorsDeps, ctx context.Context, collector hubcollector.Collector) {
		started <- collector.ID
		<-release
		d.UpdateCollectorStatus(collector.ID, "ok", "")
	}
	t.Cleanup(func() {
		close(release)
		if hadOriginal {
			collectorExecutorRegistry[collectorType] = original
		} else {
			delete(collectorExecutorRegistry, collectorType)
		}
	})

	store.statusByID["collector-a"] = hubcollector.Collector{ID: "collector-a", CollectorType: collectorType, Enabled: true}
	store.statusByID["collector-b"] = hubcollector.Collector{ID: "collector-b", CollectorType: collectorType, Enabled: true}

	if startedRuns := sut.runPendingCollectors(context.Background()); startedRuns != 2 {
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
