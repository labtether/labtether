package agentcore

import (
	"net"
	"os"
	"strings"
	"sync"
	"time"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
	"github.com/labtether/labtether/internal/agentmgr"
)

// connectorCache caches discovery results to avoid TCP probes on every heartbeat.
var connectorCache struct {
	mu      sync.Mutex
	results []agentmgr.ConnectorInfo
	at      time.Time
	key     string
}

const connectorCacheTTL = 60 * time.Second

var connectorDiscoveryConfig struct {
	mu         sync.RWMutex
	dockerMode string
	endpoint   string
}

func init() {
	setConnectorDiscoveryDockerConfig("auto", "/var/run/docker.sock")
}

func setConnectorDiscoveryDockerConfig(mode, endpoint string) {
	normalizedMode := strings.TrimSpace(strings.ToLower(mode))
	if normalizedMode == "" {
		normalizedMode = "auto"
	}
	normalizedEndpoint := strings.TrimSpace(endpoint)
	if normalizedEndpoint == "" {
		normalizedEndpoint = "/var/run/docker.sock"
	}

	connectorDiscoveryConfig.mu.Lock()
	connectorDiscoveryConfig.dockerMode = normalizedMode
	connectorDiscoveryConfig.endpoint = normalizedEndpoint
	connectorDiscoveryConfig.mu.Unlock()

	connectorCache.mu.Lock()
	connectorCache.at = time.Time{}
	connectorCache.results = nil
	connectorCache.key = ""
	connectorCache.mu.Unlock()
}

func currentConnectorDiscoveryDockerConfig() (mode, endpoint string) {
	connectorDiscoveryConfig.mu.RLock()
	defer connectorDiscoveryConfig.mu.RUnlock()
	return connectorDiscoveryConfig.dockerMode, connectorDiscoveryConfig.endpoint
}

// discoverConnectors returns locally available connector endpoints.
// Results are cached for 60s to avoid adding TCP probe latency to every heartbeat.
func discoverConnectors() []agentmgr.ConnectorInfo {
	dockerMode, dockerEndpoint := currentConnectorDiscoveryDockerConfig()
	cacheKey := dockerMode + "|" + dockerEndpoint

	connectorCache.mu.Lock()
	defer connectorCache.mu.Unlock()

	if connectorCache.key == cacheKey && time.Since(connectorCache.at) < connectorCacheTTL && connectorCache.results != nil {
		return connectorCache.results
	}

	var connectors []agentmgr.ConnectorInfo

	// Docker endpoint
	if dockerMode != "false" {
		endpoint := strings.TrimSpace(dockerEndpoint)
		if endpoint == "" {
			endpoint = "/var/run/docker.sock"
		}
		reachable := dockerEndpointReachable(endpoint)
		if reachable || dockerMode == "true" {
			connectors = append(connectors, agentmgr.ConnectorInfo{
				Type:      "docker",
				Endpoint:  connectorDockerEndpointForMetadata(endpoint),
				Reachable: reachable,
			})
		}
	}

	// Common API ports
	probes := []struct {
		connType string
		port     string
	}{
		{"proxmox", "8006"},
		{"pbs", "8007"},
		{"truenas", "443"},
		{"homeassistant", "8123"},
	}

	for _, p := range probes {
		if isPortOpen("127.0.0.1", p.port) {
			connectors = append(connectors, agentmgr.ConnectorInfo{
				Type:      p.connType,
				Endpoint:  "https://127.0.0.1:" + p.port,
				Reachable: true,
			})
		}
	}

	connectorCache.results = connectors
	connectorCache.at = time.Now()
	connectorCache.key = cacheKey
	return connectors
}

func connectorDockerEndpointForMetadata(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return "unix:///var/run/docker.sock"
	}
	if strings.HasPrefix(trimmed, "/") {
		return "unix://" + trimmed
	}
	return trimmed
}

func dockerEndpointReachable(endpoint string) bool {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return false
	}
	if stripped, ok := dockerpkg.TrimDockerUnixScheme(trimmed); ok {
		trimmed = stripped
	}
	if strings.HasPrefix(trimmed, "/") {
		if _, err := os.Stat(trimmed); err != nil {
			return false
		}
	}
	return dockerpkg.PingDockerEndpoint(strings.TrimSpace(endpoint), 4*time.Second)
}

func isPortOpen(host, port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
