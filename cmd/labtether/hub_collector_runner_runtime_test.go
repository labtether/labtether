package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
)

type fakeDockerDiscoverConnector struct {
	assets []connectorsdk.Asset
	err    error
}

func (f *fakeDockerDiscoverConnector) ID() string { return hubcollector.CollectorTypeDocker }

func (f *fakeDockerDiscoverConnector) DisplayName() string { return "Docker Test Connector" }

func (f *fakeDockerDiscoverConnector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{DiscoverAssets: true}
}

func (f *fakeDockerDiscoverConnector) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	return f.assets, f.err
}

func (f *fakeDockerDiscoverConnector) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	return connectorsdk.Health{Status: "ok", Message: "test connector"}, nil
}

func (f *fakeDockerDiscoverConnector) Actions() []connectorsdk.ActionDescriptor {
	return nil
}

func (f *fakeDockerDiscoverConnector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	return connectorsdk.ActionResult{Status: "succeeded", Message: "noop"}, nil
}

type blockingDockerConnector struct {
	calls   atomic.Int32
	release chan struct{}
}

func (b *blockingDockerConnector) ID() string { return hubcollector.CollectorTypeDocker }

func (b *blockingDockerConnector) DisplayName() string { return "Blocking Docker Test Connector" }

func (b *blockingDockerConnector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{DiscoverAssets: true}
}

func (b *blockingDockerConnector) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	b.calls.Add(1)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.release:
	}
	return nil, nil
}

func (b *blockingDockerConnector) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	return connectorsdk.Health{Status: "ok", Message: "blocking connector"}, nil
}

func (b *blockingDockerConnector) Actions() []connectorsdk.ActionDescriptor {
	return nil
}

func (b *blockingDockerConnector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	return connectorsdk.ActionResult{Status: "succeeded", Message: "noop"}, nil
}

type recordingHubCollectorStore struct {
	mu         sync.Mutex
	statusByID map[string]hubcollector.Collector
}

func newRecordingHubCollectorStore() *recordingHubCollectorStore {
	return &recordingHubCollectorStore{
		statusByID: make(map[string]hubcollector.Collector),
	}
}

func (s *recordingHubCollectorStore) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *recordingHubCollectorStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	collector, ok := s.statusByID[id]
	return collector, ok, nil
}

func (s *recordingHubCollectorStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]hubcollector.Collector, 0, len(s.statusByID))
	for _, collector := range s.statusByID {
		if enabledOnly && !collector.Enabled {
			continue
		}
		out = append(out, collector)
	}
	return out, nil
}

func (s *recordingHubCollectorStore) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *recordingHubCollectorStore) DeleteHubCollector(id string) error {
	return fmt.Errorf("not implemented")
}

func (s *recordingHubCollectorStore) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	collector := s.statusByID[id]
	collector.ID = id
	collector.LastStatus = status
	collector.LastError = lastError
	collector.Enabled = true
	collected := collectedAt.UTC()
	collector.LastCollectedAt = &collected
	s.statusByID[id] = collector
	return nil
}

func TestCollectorRunGuardSingleFlight(t *testing.T) {
	sut := newTestAPIServer(t)

	if ok := sut.tryBeginCollectorRun("collector-a"); !ok {
		t.Fatalf("expected first tryBeginCollectorRun to acquire run guard")
	}
	if ok := sut.tryBeginCollectorRun("collector-a"); ok {
		t.Fatalf("expected second tryBeginCollectorRun to be rejected")
	}
	sut.finishCollectorRun("collector-a")
	if ok := sut.tryBeginCollectorRun("collector-a"); !ok {
		t.Fatalf("expected run guard to be released after finishCollectorRun")
	}
}

func TestExecuteDockerCollectorEmptyDiscoveryIsOK(t *testing.T) {
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	sut.connectorRegistry.Register(&fakeDockerDiscoverConnector{
		assets: nil,
	})

	collector := hubcollector.Collector{
		ID:              "collector-docker-empty",
		AssetID:         "docker-cluster-test",
		CollectorType:   hubcollector.CollectorTypeDocker,
		Enabled:         true,
		IntervalSeconds: 60,
	}

	sut.executeDockerCollector(context.Background(), collector)

	updated, ok, err := store.GetHubCollector(collector.ID)
	if err != nil {
		t.Fatalf("GetHubCollector() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected collector status update to be recorded")
	}
	if updated.LastStatus != "ok" {
		t.Fatalf("LastStatus = %q, want ok", updated.LastStatus)
	}
	if updated.LastError != "" {
		t.Fatalf("LastError = %q, want empty", updated.LastError)
	}

	clusterAsset, exists, assetErr := sut.assetStore.GetAsset(collector.AssetID)
	if assetErr != nil {
		t.Fatalf("GetAsset(%q) error = %v", collector.AssetID, assetErr)
	}
	if !exists {
		t.Fatalf("expected collector cluster asset heartbeat to be refreshed")
	}
	if clusterAsset.Metadata["discovered"] != "0" {
		t.Fatalf("cluster asset discovered metadata = %q, want 0", clusterAsset.Metadata["discovered"])
	}
}

func TestExecuteDockerCollectorAddsCanonicalResourceMetadata(t *testing.T) {
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	sut.connectorRegistry.Register(&fakeDockerDiscoverConnector{
		assets: []connectorsdk.Asset{
			{
				ID:     "docker-ct-agent-01-abc123",
				Type:   "docker-container",
				Name:   "nginx",
				Source: "docker",
				Metadata: map[string]string{
					"container_id": "abc123",
					"state":        "running",
					"status":       "Up 1h",
				},
			},
		},
	})

	collector := hubcollector.Collector{
		ID:              "collector-docker-canonical",
		AssetID:         "docker-cluster-canonical",
		CollectorType:   hubcollector.CollectorTypeDocker,
		Enabled:         true,
		IntervalSeconds: 60,
	}

	sut.executeDockerCollector(context.Background(), collector)

	asset, exists, err := sut.assetStore.GetAsset("docker-ct-agent-01-abc123")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	if !exists {
		t.Fatalf("expected discovered docker asset to be persisted")
	}
	if asset.Metadata["resource_class"] != "compute" {
		t.Fatalf("resource_class metadata = %q, want compute", asset.Metadata["resource_class"])
	}
	if asset.Metadata["resource_kind"] != "docker-container" {
		t.Fatalf("resource_kind metadata = %q, want docker-container", asset.Metadata["resource_kind"])
	}
	if asset.ResourceClass != "compute" {
		t.Fatalf("asset.ResourceClass = %q, want compute", asset.ResourceClass)
	}
	if asset.ResourceKind != "docker-container" {
		t.Fatalf("asset.ResourceKind = %q, want docker-container", asset.ResourceKind)
	}
}

func TestExecuteDockerCollectorPrunesStaleDiscoveredAssets(t *testing.T) {
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	sut.connectorRegistry.Register(&fakeDockerDiscoverConnector{
		assets: []connectorsdk.Asset{
			{
				ID:     "docker-ct-agent-01-keep123",
				Type:   "docker-container",
				Name:   "keep",
				Source: "docker",
				Metadata: map[string]string{
					"agent_id":     "agent-01",
					"container_id": "keep123",
					"state":        "running",
					"status":       "Up 1h",
				},
			},
		},
	})

	collector := hubcollector.Collector{
		ID:              "collector-docker-prune",
		AssetID:         "docker-cluster-prune",
		CollectorType:   hubcollector.CollectorTypeDocker,
		Enabled:         true,
		IntervalSeconds: 60,
	}

	for _, req := range []assets.HeartbeatRequest{
		{
			AssetID: "docker-ct-agent-01-keep123",
			Type:    "docker-container",
			Name:    "keep",
			Source:  "docker",
			Status:  "online",
			Metadata: map[string]string{
				"agent_id":     "agent-01",
				"container_id": "keep123",
				"state":        "running",
				"status":       "Up 1h",
			},
		},
		{
			AssetID: "docker-ct-agent-01-stale999",
			Type:    "docker-container",
			Name:    "stale",
			Source:  "docker",
			Status:  "online",
			Metadata: map[string]string{
				"agent_id":     "agent-01",
				"container_id": "stale999",
				"state":        "running",
				"status":       "Up 1h",
			},
		},
		{
			AssetID: "docker-cluster-prune",
			Type:    "connector-cluster",
			Name:    "",
			Source:  "docker",
			Status:  "online",
			Metadata: map[string]string{
				"collector_id":   collector.ID,
				"connector_type": "docker",
				"discovered":     "2",
			},
		},
	} {
		if _, err := sut.processHeartbeatRequest(req); err != nil {
			t.Fatalf("seed heartbeat %s: %v", req.AssetID, err)
		}
	}

	sut.executeDockerCollector(context.Background(), collector)

	if _, exists, err := sut.assetStore.GetAsset("docker-ct-agent-01-keep123"); err != nil {
		t.Fatalf("GetAsset(keep) error = %v", err)
	} else if !exists {
		t.Fatalf("expected keep asset to remain after prune")
	}

	if _, exists, err := sut.assetStore.GetAsset("docker-ct-agent-01-stale999"); err != nil {
		t.Fatalf("GetAsset(stale) error = %v", err)
	} else if exists {
		t.Fatalf("expected stale docker asset to be deleted")
	}

	clusterAsset, exists, err := sut.assetStore.GetAsset("docker-cluster-prune")
	if err != nil {
		t.Fatalf("GetAsset(cluster) error = %v", err)
	}
	if !exists {
		t.Fatalf("expected collector cluster asset to remain")
	}
	if clusterAsset.Type != "connector-cluster" {
		t.Fatalf("cluster asset type = %q, want connector-cluster", clusterAsset.Type)
	}
}

func TestExecuteCollectorSingleFlightSkipsDuplicateConcurrentRuns(t *testing.T) {
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	blocking := &blockingDockerConnector{release: make(chan struct{})}
	sut.connectorRegistry.Register(blocking)

	collector := hubcollector.Collector{
		ID:              "collector-docker-singleflight",
		AssetID:         "docker-cluster-singleflight",
		CollectorType:   hubcollector.CollectorTypeDocker,
		Enabled:         true,
		IntervalSeconds: 60,
	}

	var wg sync.WaitGroup
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			defer wg.Done()
			sut.executeCollector(context.Background(), collector)
		}()
	}

	time.Sleep(200 * time.Millisecond)
	if calls := blocking.calls.Load(); calls != 1 {
		t.Fatalf("blocking connector Discover() calls = %d, want 1", calls)
	}
	close(blocking.release)
	wg.Wait()

	if calls := blocking.calls.Load(); calls != 1 {
		t.Fatalf("blocking connector Discover() calls after completion = %d, want 1", calls)
	}
}

func TestWithCanonicalResourceMetadataCrossSourceAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		source       string
		assetType    string
		metadata     map[string]string
		wantKind     string
		wantClass    string
		requireField string
	}{
		{
			name:      "pbs datastore kind",
			source:    "pbs",
			assetType: "storage-pool",
			metadata: map[string]string{
				"store": "backup-store",
			},
			wantKind:     "datastore",
			wantClass:    "storage",
			requireField: "store",
		},
		{
			name:      "truenas controller kind",
			source:    "truenas",
			assetType: "nas",
			metadata: map[string]string{
				"hostname": "OmegaNAS",
			},
			wantKind:     "storage-controller",
			wantClass:    "storage",
			requireField: "hostname",
		},
		{
			name:      "portainer container stays container",
			source:    "portainer",
			assetType: "container",
			metadata: map[string]string{
				"container_id": "abc123",
			},
			wantKind:     "container",
			wantClass:    "compute",
			requireField: "container_id",
		},
		{
			name:      "proxmox vm stays vm",
			source:    "proxmox",
			assetType: "vm",
			metadata: map[string]string{
				"vmid": "100",
			},
			wantKind:     "vm",
			wantClass:    "compute",
			requireField: "vmid",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resourceKind, metadata := withCanonicalResourceMetadata(tc.source, tc.assetType, tc.metadata)
			if resourceKind != tc.wantKind {
				t.Fatalf("resourceKind = %q, want %q", resourceKind, tc.wantKind)
			}
			if metadata["resource_kind"] != tc.wantKind {
				t.Fatalf("metadata.resource_kind = %q, want %q", metadata["resource_kind"], tc.wantKind)
			}
			if metadata["resource_class"] != tc.wantClass {
				t.Fatalf("metadata.resource_class = %q, want %q", metadata["resource_class"], tc.wantClass)
			}
			if metadata[tc.requireField] == "" {
				t.Fatalf("expected %q metadata key to be preserved", tc.requireField)
			}
		})
	}
}
