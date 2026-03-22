package docker

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectorsdk"
)

// AgentCommander is an interface for sending messages to connected agents.
type AgentCommander interface {
	SendToAgent(assetID string, msg agentmgr.Message) error
}

// Coordinator manages Docker state from multiple agents and implements connectorsdk.Connector.
type Coordinator struct {
	mu       sync.RWMutex
	hosts    map[string]*DockerHost // agentAssetID -> latest state
	agentMgr AgentCommander

	// Pending action results for request-response correlation.
	pendingMu      sync.Mutex
	pendingResults map[string]chan agentmgr.DockerActionResultData
	pendingCompose map[string]chan agentmgr.DockerComposeResultData
}

// NewCoordinator creates a new Docker coordinator.
func NewCoordinator(agentMgr AgentCommander) *Coordinator {
	return &Coordinator{
		hosts:          make(map[string]*DockerHost),
		agentMgr:       agentMgr,
		pendingResults: make(map[string]chan agentmgr.DockerActionResultData),
		pendingCompose: make(map[string]chan agentmgr.DockerComposeResultData),
	}
}

// --- connectorsdk.Connector implementation ---

// ID returns the connector's canonical identifier.
func (c *Coordinator) ID() string { return "docker" }

// DisplayName returns the human-readable connector name.
func (c *Coordinator) DisplayName() string { return "Docker" }

// Capabilities reports what this connector supports.
func (c *Coordinator) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{
		DiscoverAssets: true,
		CollectMetrics: true,
		CollectEvents:  true,
		ExecuteActions: true,
	}
}

// TestConnection reports coordinator health based on how many agents are reporting.
func (c *Coordinator) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	c.mu.RLock()
	count := len(c.hosts)
	c.mu.RUnlock()
	if count == 0 {
		return connectorsdk.Health{
			Status:  "ok",
			Message: "docker coordinator running (no agents reporting yet)",
		}, nil
	}
	return connectorsdk.Health{
		Status:  "ok",
		Message: fmt.Sprintf("docker coordinator: %d agent(s) reporting", count),
	}, nil
}

// Discover returns all Docker assets aggregated from agent reports.
func (c *Coordinator) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var assets []connectorsdk.Asset

	for agentID, host := range c.hosts {
		// Docker host asset.
		hostAssetID := "docker-host-" + normalizeID(agentID)
		assets = append(assets, connectorsdk.Asset{
			ID:     hostAssetID,
			Type:   "container-host",
			Name:   fmt.Sprintf("docker-%s", normalizeID(agentID)),
			Source: "docker",
			Metadata: map[string]string{
				"agent_id":       agentID,
				"engine_version": host.Engine.Version,
				"engine_os":      host.Engine.OS,
				"engine_arch":    host.Engine.Arch,
				"containers":     fmt.Sprintf("%d", len(host.Containers)),
			},
		})

		// Container assets.
		for _, ct := range host.Containers {
			ctID := ct.ID
			if len(ctID) > 12 {
				ctID = ctID[:12]
			}
			assetID := fmt.Sprintf("docker-ct-%s-%s", normalizeID(agentID), ctID)

			meta := map[string]string{
				"container_id": ct.ID,
				"image":        ct.Image,
				"state":        ct.State,
				"status":       ct.Status,
				"agent_id":     agentID,
			}
			if ct.StackName != "" {
				meta["stack"] = ct.StackName
			}

			// Include stats if available.
			if stats, ok := host.Stats[ct.ID]; ok {
				meta["cpu_percent"] = fmt.Sprintf("%.1f", stats.CPUPercent)
				meta["memory_percent"] = fmt.Sprintf("%.1f", stats.MemoryPercent)
			}

			assets = append(assets, connectorsdk.Asset{
				ID:       assetID,
				Type:     "docker-container",
				Name:     ct.Name,
				Source:   "docker",
				Metadata: meta,
			})
		}

		// Compose stack assets.
		for _, stack := range host.ComposeStacks {
			stackID := fmt.Sprintf("docker-stack-%s-%s", normalizeID(agentID), normalizeID(stack.Name))
			assets = append(assets, connectorsdk.Asset{
				ID:     stackID,
				Type:   "compose-stack",
				Name:   stack.Name,
				Source: "docker",
				Metadata: map[string]string{
					"agent_id":    agentID,
					"status":      stack.Status,
					"config_file": stack.ConfigFile,
					"containers":  fmt.Sprintf("%d", len(stack.ContainerIDs)),
				},
			})
		}
	}

	return assets, nil
}

// normalizeID makes a string safe for use in asset IDs (lowercase, spaces and dots become dashes).
func normalizeID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ".", "-")
	return s
}

// ClearAll removes all cached Docker hosts, stats, and pending results.
// Used after an admin data reset to ensure in-memory state matches the DB.
func (c *Coordinator) ClearAll() {
	c.mu.Lock()
	c.hosts = make(map[string]*DockerHost)
	c.mu.Unlock()

	c.pendingMu.Lock()
	c.pendingResults = make(map[string]chan agentmgr.DockerActionResultData)
	c.pendingCompose = make(map[string]chan agentmgr.DockerComposeResultData)
	c.pendingMu.Unlock()
}

// ListHosts returns a snapshot of all Docker hosts.
func (c *Coordinator) ListHosts() []*DockerHost {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*DockerHost, 0, len(c.hosts))
	for _, h := range c.hosts {
		hostCopy := copyDockerHost(h)
		result = append(result, &hostCopy)
	}
	return result
}

// GetHost returns a single Docker host by agent asset ID.
func (c *Coordinator) GetHost(agentID string) (*DockerHost, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	h, ok := c.hosts[agentID]
	if !ok || h == nil {
		return nil, false
	}
	hostCopy := copyDockerHost(h)
	return &hostCopy, true
}

// GetHostByNormalizedID looks up a host by its normalized ID (used in asset IDs).
func (c *Coordinator) GetHostByNormalizedID(normalizedID string) (*DockerHost, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for agentID, h := range c.hosts {
		if normalizeID(agentID) == normalizedID {
			hostCopy := copyDockerHost(h)
			return &hostCopy, true
		}
	}
	return nil, false
}

// FindContainer looks up a container across all hosts, returning host and container.
// containerAssetID must follow the docker-ct-{agentNorm}-{ctShort} format.
func (c *Coordinator) FindContainer(containerAssetID string) (*DockerHost, *ContainerState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	rest := strings.TrimPrefix(containerAssetID, "docker-ct-")
	if rest == containerAssetID {
		return nil, nil, false
	}
	for aID, host := range c.hosts {
		prefix := normalizeID(aID) + "-"
		if !strings.HasPrefix(rest, prefix) {
			continue
		}
		shortCT := strings.TrimPrefix(rest, prefix)
		for i := range host.Containers {
			ctID := host.Containers[i].ID
			if len(ctID) > 12 {
				ctID = ctID[:12]
			}
			if ctID == shortCT {
				hostCopy := copyDockerHost(host)
				ctCopy := host.Containers[i]
				return &hostCopy, &ctCopy, true
			}
		}
	}
	return nil, nil, false
}

// GetContainerStats returns stats for a container by its full Docker ID.
func (c *Coordinator) GetContainerStats(agentID, containerID string) (ContainerStats, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	h, ok := c.hosts[agentID]
	if !ok {
		return ContainerStats{}, false
	}
	s, ok := h.Stats[containerID]
	return s, ok
}

func copyDockerHost(src *DockerHost) DockerHost {
	if src == nil {
		return DockerHost{}
	}
	dst := *src
	dst.Containers = append([]ContainerState(nil), src.Containers...)
	for i := range dst.Containers {
		dst.Containers[i].Labels = cloneStringMap(dst.Containers[i].Labels)
	}
	dst.Images = append([]ImageState(nil), src.Images...)
	for i := range dst.Images {
		dst.Images[i].Tags = append([]string(nil), dst.Images[i].Tags...)
	}
	dst.Networks = append([]NetworkState(nil), src.Networks...)
	dst.Volumes = append([]VolumeState(nil), src.Volumes...)
	dst.ComposeStacks = append([]ComposeStackState(nil), src.ComposeStacks...)
	for i := range dst.ComposeStacks {
		dst.ComposeStacks[i].ContainerIDs = append([]string(nil), dst.ComposeStacks[i].ContainerIDs...)
	}
	dst.Stats = make(map[string]ContainerStats, len(src.Stats))
	for key, value := range src.Stats {
		dst.Stats[key] = value
	}
	return dst
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
