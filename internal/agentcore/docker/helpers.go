package docker

import (
	"context"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// NewExecManager creates a DockerExecManager using this collector's Docker client.
func (dc *DockerCollector) NewExecManager() *DockerExecManager {
	return NewDockerExecManager(dc.client)
}

// NewLogManager creates a DockerLogManager using this collector's Docker client.
func (dc *DockerCollector) NewLogManager() *DockerLogManager {
	return NewDockerLogManager(dc.client)
}

// ListContainers lists Docker containers from the collector's client.
func (dc *DockerCollector) ListContainers(ctx context.Context) ([]DockerContainer, error) {
	if dc.client == nil {
		return nil, nil
	}
	return dc.client.listContainers(ctx)
}

// HasClient returns true if the collector has an active Docker client.
func (dc *DockerCollector) HasClient() bool {
	return dc.client != nil
}

// NewTestCollector creates a DockerCollector wired to a test HTTP endpoint.
// Intended for use in integration tests from parent packages.
func NewTestCollector(endpoint string, transport Transport, assetID string) *DockerCollector {
	client := NewDockerClient(endpoint)
	return &DockerCollector{
		client:                client,
		transport:             transport,
		socketPath:            endpoint,
		assetID:               assetID,
		interval:              defaultDockerDiscoveryInterval,
		fullReconcileInterval: deriveDockerFullReconcileInterval(defaultDockerDiscoveryInterval),
		discoveryTriggerCh:    make(chan dockerDiscoveryTrigger, 32),
		statsTriggerCh:        make(chan struct{}, 4),
		inventory: dockerInventorySnapshot{
			Containers: make(map[string]agentmgr.DockerContainerInfo),
			Images:     make(map[string]agentmgr.DockerImageInfo),
			Networks:   make(map[string]agentmgr.DockerNetworkInfo),
			Volumes:    make(map[string]agentmgr.DockerVolumeInfo),
		},
		runningContainerIDs: make(map[string]struct{}),
		statsSchedule:       make(map[string]*dockerStatsSchedule),
	}
}

// SetTestHTTPClient overrides the HTTP client on the collector's docker client.
// Used for testing with httptest.NewTLSServer.
func (dc *DockerCollector) SetTestHTTPClient(c interface {
	Do(*http.Request) (*http.Response, error)
}) {
	if hc, ok := c.(*http.Client); ok {
		dc.client.httpClient = hc
	}
}

// RefreshAndPublishFull is an exported wrapper for testing full discovery refresh.
func (dc *DockerCollector) RefreshAndPublishFull(ctx context.Context, forcePublish bool) (bool, error) {
	return dc.refreshAndPublishFull(ctx, forcePublish)
}

// RefreshAndPublishContainerDelta is an exported wrapper for testing delta refresh.
func (dc *DockerCollector) RefreshAndPublishContainerDelta(ctx context.Context) (bool, error) {
	return dc.refreshAndPublishContainerDelta(ctx)
}

// TestInventoryState returns snapshot info for test assertions.
func (dc *DockerCollector) TestInventoryState() (hasPublished bool, containerCount, imageCount int) {
	dc.inventoryMu.RLock()
	defer dc.inventoryMu.RUnlock()
	return dc.hasPublishedFull, len(dc.inventory.Containers), len(dc.inventory.Images)
}

// SetTestInventoryState sets internal state for test scenarios.
func (dc *DockerCollector) SetTestInventoryState(published bool, containers map[string]agentmgr.DockerContainerInfo, images map[string]agentmgr.DockerImageInfo, runningIDs map[string]struct{}) {
	dc.inventoryMu.Lock()
	dc.inventory.Containers = containers
	dc.inventory.Images = images
	dc.hasPublishedFull = published
	dc.runningContainerIDs = runningIDs
	dc.inventoryMu.Unlock()
}

// DiscoveryTriggerCount returns the number of pending discovery triggers.
func (dc *DockerCollector) DiscoveryTriggerCount() int {
	return len(dc.discoveryTriggerCh)
}

// RunEventLoop is an exported wrapper for testing the event loop.
func (dc *DockerCollector) RunEventLoop(ctx context.Context) {
	dc.runEventLoop(ctx)
}

// CurrentRunningContainerIDs returns the sorted list of running container IDs.
func (dc *DockerCollector) CurrentRunningContainerIDs() []string {
	return dc.currentRunningContainerIDs()
}

// CollectAndSendStats is an exported wrapper for testing stats collection.
func (dc *DockerCollector) CollectAndSendStats(ctx context.Context) {
	dc.collectAndSendStats(ctx)
}

// PingDockerEndpoint checks if a Docker endpoint is reachable.
func PingDockerEndpoint(endpoint string, timeout time.Duration) bool {
	client := NewDockerClient(endpoint)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return client.ping(ctx) == nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
