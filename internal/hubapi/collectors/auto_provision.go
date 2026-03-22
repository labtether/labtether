package collectors

// auto_provision.go — pure helper functions for Docker auto-collector
// provisioning logic. These functions have no dependency on apiServer state
// and are kept here so they can be tested in isolation and reused by any
// caller that imports the collectors package.

import (
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubcollector"
)

const (
	// AutoDockerCollectorIntervalSeconds is the default polling interval for
	// auto-provisioned Docker hub collectors.
	AutoDockerCollectorIntervalSeconds = 60

	// DockerDiscoveryCollectorKickMinGap is the minimum time that must have
	// elapsed since the last Docker collector run before a discovery kick is
	// allowed.
	DockerDiscoveryCollectorKickMinGap = 10 * time.Second
)

// HeartbeatAdvertisesDockerConnector returns true if any of the connector
// infos advertised in a heartbeat payload is a Docker connector.
func HeartbeatAdvertisesDockerConnector(connectors []agentmgr.ConnectorInfo) bool {
	for _, connector := range connectors {
		if strings.EqualFold(strings.TrimSpace(connector.Type), hubcollector.CollectorTypeDocker) {
			return true
		}
	}
	return false
}

// AutoDockerCollectorAssetID returns the stable asset ID for the Docker
// cluster asset auto-provisioned for the given agent.
func AutoDockerCollectorAssetID(agentAssetID string) string {
	normalized := NormalizeAssetKey(agentAssetID)
	if normalized == "" {
		return "docker-cluster-auto"
	}
	return "docker-cluster-" + normalized
}

// SelectDockerCollectorForDiscoveryKick returns the first enabled Docker
// collector that is eligible for an on-demand run based on the min gap since
// its last collection. When minGap is zero or negative the package default
// DockerDiscoveryCollectorKickMinGap is used.
func SelectDockerCollectorForDiscoveryKick(
	collectors []hubcollector.Collector,
	now time.Time,
	minGap time.Duration,
) (hubcollector.Collector, bool) {
	if minGap <= 0 {
		minGap = DockerDiscoveryCollectorKickMinGap
	}

	for _, collector := range collectors {
		if collector.CollectorType != hubcollector.CollectorTypeDocker || !collector.Enabled {
			continue
		}
		if collector.LastCollectedAt != nil {
			lastCollectedAt := collector.LastCollectedAt.UTC()
			if !lastCollectedAt.IsZero() && now.Sub(lastCollectedAt) < minGap {
				return hubcollector.Collector{}, false
			}
		}
		return collector, true
	}
	return hubcollector.Collector{}, false
}

// IsHubCollectorAlreadyExistsError returns true if the error indicates that a
// hub collector with the same identity already exists in the store.
func IsHubCollectorAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "already exists") ||
		strings.Contains(value, "duplicate") ||
		strings.Contains(value, "unique")
}
