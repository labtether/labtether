package main

import (
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubcollector"
)

// Package-level constants aliased from the collectors package so existing
// call-sites inside cmd/labtether compile unchanged.
const autoDockerCollectorIntervalSeconds = collectorspkg.AutoDockerCollectorIntervalSeconds
const dockerDiscoveryCollectorKickMinGap = collectorspkg.DockerDiscoveryCollectorKickMinGap

func (s *apiServer) autoProvisionDockerCollectorIfNeeded(agentAssetID string, connectors []agentmgr.ConnectorInfo) {
	if s.hubCollectorStore == nil {
		return
	}
	if !heartbeatAdvertisesDockerConnector(connectors) {
		return
	}

	collectors, err := s.hubCollectorStore.ListHubCollectors(500, false)
	if err != nil {
		log.Printf("agentws: failed to list hub collectors for docker auto-provision: %v", err)
		return
	}
	for _, collector := range collectors {
		if collector.CollectorType == hubcollector.CollectorTypeDocker {
			return
		}
	}

	collectorAssetID := autoDockerCollectorAssetID(agentAssetID)
	_, clusterMeta := withCanonicalResourceMetadata("docker", "connector-cluster", map[string]string{
		"connector_type": "docker",
		"provision_mode": "auto",
		"agent_id":       strings.TrimSpace(agentAssetID),
		"discovered":     "0",
	})
	if _, err := s.processHeartbeatRequest(assets.HeartbeatRequest{
		AssetID:  collectorAssetID,
		Type:     "connector-cluster",
		Name:     collectorAssetID,
		Source:   "docker",
		Status:   "online",
		Metadata: clusterMeta,
	}); err != nil {
		log.Printf("agentws: failed to create docker collector cluster asset %s: %v", collectorAssetID, err)
		return
	}

	enabled := true
	collector, err := s.hubCollectorStore.CreateHubCollector(hubcollector.CreateCollectorRequest{
		AssetID:       collectorAssetID,
		CollectorType: hubcollector.CollectorTypeDocker,
		Config: map[string]any{
			"provision_mode": "auto",
			"agent_asset_id": strings.TrimSpace(agentAssetID),
		},
		Enabled:         &enabled,
		IntervalSeconds: autoDockerCollectorIntervalSeconds,
	})
	if err != nil {
		if isHubCollectorAlreadyExistsError(err) {
			return
		}
		log.Printf("agentws: failed to auto-provision docker collector for %s: %v", strings.TrimSpace(agentAssetID), err)
		return
	}

	log.Printf("agentws: auto-provisioned docker collector id=%s asset_id=%s trigger_asset_id=%s",
		collector.ID, collector.AssetID, strings.TrimSpace(agentAssetID))

	if s.connectorRegistry == nil {
		return
	}
	if err := s.runHubCollectorNow(collector.ID); err != nil {
		log.Printf("agentws: failed to start auto-provisioned docker collector %s: %v", collector.ID, err)
	}
}

func (s *apiServer) triggerDockerCollectorRunForDiscovery() {
	if s.hubCollectorStore == nil {
		return
	}

	collectors, err := s.hubCollectorStore.ListHubCollectors(500, true)
	if err != nil {
		log.Printf("agentws: failed to list hub collectors for docker discovery trigger: %v", err)
		return
	}

	collector, ok := selectDockerCollectorForDiscoveryKick(collectors, time.Now().UTC(), dockerDiscoveryCollectorKickMinGap)
	if !ok {
		return
	}

	if err := s.runHubCollectorNow(collector.ID); err != nil {
		log.Printf("agentws: failed to trigger docker collector run on discovery for collector %s: %v", collector.ID, err)
	}
}

// --- Thin aliases to collectorspkg for backward compatibility with callers
// inside this package (including tests in package main). ---

func heartbeatAdvertisesDockerConnector(connectors []agentmgr.ConnectorInfo) bool {
	return collectorspkg.HeartbeatAdvertisesDockerConnector(connectors)
}

func autoDockerCollectorAssetID(agentAssetID string) string {
	return collectorspkg.AutoDockerCollectorAssetID(agentAssetID)
}

func selectDockerCollectorForDiscoveryKick(
	collectors []hubcollector.Collector,
	now time.Time,
	minGap time.Duration,
) (hubcollector.Collector, bool) {
	return collectorspkg.SelectDockerCollectorForDiscoveryKick(collectors, now, minGap)
}

func isHubCollectorAlreadyExistsError(err error) bool {
	return collectorspkg.IsHubCollectorAlreadyExistsError(err)
}
