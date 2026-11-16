package testutil

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
)

// The collector runner fixtures below are shared by cmd/labtether's collector
// runner tests and internal/hubapi/collectors'. cmd aliases them.

// FakeDockerDiscoverConnector is a Docker connector whose Discover returns
// Assets and Err.
type FakeDockerDiscoverConnector struct {
	Assets []connectorsdk.Asset
	Err    error
}

func (f *FakeDockerDiscoverConnector) ID() string { return hubcollector.CollectorTypeDocker }

func (f *FakeDockerDiscoverConnector) DisplayName() string { return "Docker Test Connector" }

func (f *FakeDockerDiscoverConnector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{DiscoverAssets: true}
}

func (f *FakeDockerDiscoverConnector) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	return f.Assets, f.Err
}

func (f *FakeDockerDiscoverConnector) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	return connectorsdk.Health{Status: "ok", Message: "test connector"}, nil
}

func (f *FakeDockerDiscoverConnector) Actions() []connectorsdk.ActionDescriptor {
	return nil
}

func (f *FakeDockerDiscoverConnector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	return connectorsdk.ActionResult{Status: "succeeded", Message: "noop"}, nil
}

// RecordingHubCollectorStore records each collector's status updates by ID.
// Tests seed collectors by writing to StatusByID.
type RecordingHubCollectorStore struct {
	mu         sync.Mutex
	StatusByID map[string]hubcollector.Collector
}

func NewRecordingHubCollectorStore() *RecordingHubCollectorStore {
	return &RecordingHubCollectorStore{
		StatusByID: make(map[string]hubcollector.Collector),
	}
}

func (s *RecordingHubCollectorStore) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *RecordingHubCollectorStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	collector, ok := s.StatusByID[id]
	return collector, ok, nil
}

func (s *RecordingHubCollectorStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]hubcollector.Collector, 0, len(s.StatusByID))
	for _, collector := range s.StatusByID {
		if enabledOnly && !collector.Enabled {
			continue
		}
		out = append(out, collector)
	}
	return out, nil
}

func (s *RecordingHubCollectorStore) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *RecordingHubCollectorStore) DeleteHubCollector(id string) error {
	return fmt.Errorf("not implemented")
}

func (s *RecordingHubCollectorStore) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	collector := s.StatusByID[id]
	collector.ID = id
	collector.LastStatus = status
	collector.LastError = lastError
	collector.Enabled = true
	collected := collectedAt.UTC()
	collector.LastCollectedAt = &collected
	s.StatusByID[id] = collector
	return nil
}
