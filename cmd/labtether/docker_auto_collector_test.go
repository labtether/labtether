package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubcollector"
)

type autoDockerCollectorStoreSpy struct {
	collectors  []hubcollector.Collector
	listErr     error
	createErr   error
	createCalls []hubcollector.CreateCollectorRequest
}

func (s *autoDockerCollectorStoreSpy) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	interval := req.IntervalSeconds
	if interval <= 0 {
		interval = 60
	}

	copiedReq := hubcollector.CreateCollectorRequest{
		AssetID:         req.AssetID,
		CollectorType:   req.CollectorType,
		Config:          cloneAnyMap(req.Config),
		IntervalSeconds: interval,
	}
	if req.Enabled != nil {
		enabledCopy := *req.Enabled
		copiedReq.Enabled = &enabledCopy
	}
	s.createCalls = append(s.createCalls, copiedReq)

	if s.createErr != nil {
		return hubcollector.Collector{}, s.createErr
	}

	created := hubcollector.Collector{
		ID:              fmt.Sprintf("hc-auto-%d", len(s.createCalls)),
		AssetID:         req.AssetID,
		CollectorType:   req.CollectorType,
		Config:          cloneAnyMap(req.Config),
		Enabled:         enabled,
		IntervalSeconds: interval,
	}
	s.collectors = append(s.collectors, created)
	return created, nil
}

func (s *autoDockerCollectorStoreSpy) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	for _, collector := range s.collectors {
		if collector.ID == id {
			return collector, true, nil
		}
	}
	return hubcollector.Collector{}, false, nil
}

func (s *autoDockerCollectorStoreSpy) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	result := make([]hubcollector.Collector, 0, len(s.collectors))
	for _, collector := range s.collectors {
		if enabledOnly && !collector.Enabled {
			continue
		}
		result = append(result, collector)
	}
	return result, nil
}

func (s *autoDockerCollectorStoreSpy) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *autoDockerCollectorStoreSpy) DeleteHubCollector(id string) error {
	return fmt.Errorf("not implemented")
}

func (s *autoDockerCollectorStoreSpy) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	return nil
}

func TestAutoProvisionDockerCollectorIfNeededCreatesCollector(t *testing.T) {
	sut := newTestAPIServer(t)
	store := &autoDockerCollectorStoreSpy{}
	sut.hubCollectorStore = store
	sut.connectorRegistry = nil // Skip immediate collector run in this unit test.

	sut.autoProvisionDockerCollectorIfNeeded("containervm-deltaserver", []agentmgr.ConnectorInfo{
		{Type: "docker", Endpoint: "unix:///var/run/docker.sock", Reachable: false},
	})

	if len(store.createCalls) != 1 {
		t.Fatalf("expected exactly one collector create call, got %d", len(store.createCalls))
	}
	createReq := store.createCalls[0]
	if createReq.AssetID != "docker-cluster-containervm-deltaserver" {
		t.Fatalf("collector asset_id = %q, want docker-cluster-containervm-deltaserver", createReq.AssetID)
	}
	if createReq.AssetID == "containervm-deltaserver" {
		t.Fatalf("collector asset_id should not collide with agent asset id")
	}
	if createReq.CollectorType != hubcollector.CollectorTypeDocker {
		t.Fatalf("collector_type = %q, want %q", createReq.CollectorType, hubcollector.CollectorTypeDocker)
	}
	if createReq.Enabled == nil || !*createReq.Enabled {
		t.Fatalf("expected created collector to be enabled")
	}
	if createReq.IntervalSeconds != autoDockerCollectorIntervalSeconds {
		t.Fatalf("interval_seconds = %d, want %d", createReq.IntervalSeconds, autoDockerCollectorIntervalSeconds)
	}

	clusterAsset, ok, err := sut.assetStore.GetAsset("docker-cluster-containervm-deltaserver")
	if err != nil {
		t.Fatalf("failed to read auto-provisioned cluster asset: %v", err)
	}
	if !ok {
		t.Fatalf("expected auto-provisioned docker cluster asset to exist")
	}
	if clusterAsset.Source != "docker" {
		t.Fatalf("cluster asset source = %q, want docker", clusterAsset.Source)
	}
	if clusterAsset.Type != "connector-cluster" {
		t.Fatalf("cluster asset type = %q, want connector-cluster", clusterAsset.Type)
	}
}

func TestAutoProvisionDockerCollectorIfNeededSkipsWhenDockerCollectorExists(t *testing.T) {
	sut := newTestAPIServer(t)
	store := &autoDockerCollectorStoreSpy{
		collectors: []hubcollector.Collector{
			{
				ID:            "hc-existing",
				AssetID:       "docker-cluster-existing",
				CollectorType: hubcollector.CollectorTypeDocker,
				Enabled:       true,
			},
		},
	}
	sut.hubCollectorStore = store

	sut.autoProvisionDockerCollectorIfNeeded("agent-02", []agentmgr.ConnectorInfo{
		{Type: "docker", Endpoint: "unix:///var/run/docker.sock", Reachable: true},
	})

	if len(store.createCalls) != 0 {
		t.Fatalf("expected no collector create when docker collector already exists, got %d", len(store.createCalls))
	}
}

func TestProcessAgentHeartbeatAutoProvisionsDockerCollector(t *testing.T) {
	sut := newTestAPIServer(t)
	store := &autoDockerCollectorStoreSpy{}
	sut.hubCollectorStore = store
	sut.connectorRegistry = nil // Skip immediate collector run in this unit test.

	heartbeat := agentmgr.HeartbeatData{
		AssetID:  "linux-agent-01",
		Type:     "server",
		Name:     "linux-agent-01",
		Source:   "agent",
		Status:   "online",
		Metadata: map[string]string{"platform": "linux"},
		Connectors: []agentmgr.ConnectorInfo{
			{Type: "docker", Endpoint: "unix:///var/run/docker.sock", Reachable: true},
		},
	}
	payload, err := json.Marshal(heartbeat)
	if err != nil {
		t.Fatalf("marshal heartbeat: %v", err)
	}

	sut.processAgentHeartbeat(&agentmgr.AgentConn{AssetID: "linux-agent-01"}, agentmgr.Message{
		Type: agentmgr.MsgHeartbeat,
		Data: payload,
	})

	if len(store.createCalls) != 1 {
		t.Fatalf("expected heartbeat to auto-provision docker collector once, got %d", len(store.createCalls))
	}
}

func TestHeartbeatAdvertisesDockerConnector(t *testing.T) {
	if !heartbeatAdvertisesDockerConnector([]agentmgr.ConnectorInfo{{Type: " Docker "}}) {
		t.Fatalf("expected docker connector detection to be case-insensitive and trimmed")
	}
	if heartbeatAdvertisesDockerConnector([]agentmgr.ConnectorInfo{{Type: "proxmox"}}) {
		t.Fatalf("expected non-docker connectors to be ignored")
	}
}

func TestSelectDockerCollectorForDiscoveryKick(t *testing.T) {
	now := time.Date(2026, time.February, 27, 3, 30, 0, 0, time.UTC)

	t.Run("picks enabled docker collector when stale", func(t *testing.T) {
		last := now.Add(-2 * time.Minute)
		collector, ok := selectDockerCollectorForDiscoveryKick([]hubcollector.Collector{
			{
				ID:              "hc-docker-1",
				CollectorType:   hubcollector.CollectorTypeDocker,
				Enabled:         true,
				LastCollectedAt: &last,
			},
		}, now, 10*time.Second)
		if !ok {
			t.Fatalf("expected collector selection to succeed")
		}
		if collector.ID != "hc-docker-1" {
			t.Fatalf("selected collector id = %q, want hc-docker-1", collector.ID)
		}
	})

	t.Run("skips when docker collector ran too recently", func(t *testing.T) {
		last := now.Add(-5 * time.Second)
		if _, ok := selectDockerCollectorForDiscoveryKick([]hubcollector.Collector{
			{
				ID:              "hc-docker-2",
				CollectorType:   hubcollector.CollectorTypeDocker,
				Enabled:         true,
				LastCollectedAt: &last,
			},
		}, now, 10*time.Second); ok {
			t.Fatalf("expected selection to be skipped for recent run")
		}
	})

	t.Run("skips when only non-docker collectors exist", func(t *testing.T) {
		if _, ok := selectDockerCollectorForDiscoveryKick([]hubcollector.Collector{
			{ID: "hc-proxmox", CollectorType: hubcollector.CollectorTypeProxmox, Enabled: true},
		}, now, 10*time.Second); ok {
			t.Fatalf("expected no docker collector selection")
		}
	})
}
