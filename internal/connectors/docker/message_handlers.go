package docker

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// HandleDiscovery processes a docker.discovery message from an agent.
func (c *Coordinator) HandleDiscovery(agentID string, msg agentmgr.Message) {
	var data agentmgr.DockerDiscoveryData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("docker-coordinator: invalid discovery from %s: %v", agentID, err)
		return
	}

	c.mu.Lock()
	var existingStats map[string]ContainerStats
	if existing, ok := c.hosts[agentID]; ok {
		existingStats = existing.Stats
	}
	c.hosts[agentID] = dockerHostFromDiscovery(agentID, data, existingStats)
	c.mu.Unlock()
}

// HandleDiscoveryDelta processes a docker.discovery.delta message from an agent.
func (c *Coordinator) HandleDiscoveryDelta(agentID string, msg agentmgr.Message) {
	var data agentmgr.DockerDiscoveryDeltaData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("docker-coordinator: invalid discovery delta from %s: %v", agentID, err)
		return
	}

	c.mu.Lock()
	host, ok := c.hosts[agentID]
	if !ok || host == nil {
		host = &DockerHost{
			AgentID:  agentID,
			Stats:    make(map[string]ContainerStats),
			LastSeen: time.Now(),
		}
		c.hosts[agentID] = host
	}
	applyDiscoveryDelta(host, data)
	host.LastSeen = time.Now()
	c.mu.Unlock()
}

func dockerHostFromDiscovery(agentID string, data agentmgr.DockerDiscoveryData, existingStats map[string]ContainerStats) *DockerHost {
	host := &DockerHost{
		AgentID: agentID,
		Engine: EngineInfo{
			Version:    data.Engine.Version,
			APIVersion: data.Engine.APIVersion,
			OS:         data.Engine.OS,
			Arch:       data.Engine.Arch,
		},
		Stats:    make(map[string]ContainerStats),
		LastSeen: time.Now(),
	}

	for _, ct := range data.Containers {
		host.Containers = append(host.Containers, convertContainerInfo(ct))
	}
	sortContainerStates(host.Containers)

	for _, img := range data.Images {
		host.Images = append(host.Images, ImageState{
			ID:      img.ID,
			Tags:    append([]string(nil), img.Tags...),
			Size:    img.Size,
			Created: img.Created,
		})
	}
	sortImageStates(host.Images)

	for _, net := range data.Networks {
		host.Networks = append(host.Networks, NetworkState{
			ID:     net.ID,
			Name:   net.Name,
			Driver: net.Driver,
			Scope:  net.Scope,
		})
	}
	sortNetworkStates(host.Networks)

	for _, vol := range data.Volumes {
		host.Volumes = append(host.Volumes, VolumeState{
			Name:       vol.Name,
			Driver:     vol.Driver,
			Mountpoint: vol.Mountpoint,
		})
	}
	sortVolumeStates(host.Volumes)

	for _, stack := range data.ComposeStacks {
		host.ComposeStacks = append(host.ComposeStacks, convertComposeStack(stack))
	}
	sortComposeStackStates(host.ComposeStacks)

	for key, value := range existingStats {
		host.Stats[key] = value
	}

	return host
}

func applyDiscoveryDelta(host *DockerHost, data agentmgr.DockerDiscoveryDeltaData) {
	if host == nil {
		return
	}
	if data.Engine != nil {
		host.Engine = EngineInfo{
			Version:    data.Engine.Version,
			APIVersion: data.Engine.APIVersion,
			OS:         data.Engine.OS,
			Arch:       data.Engine.Arch,
		}
	}

	if len(data.UpsertContainers) > 0 || len(data.RemoveContainerIDs) > 0 {
		byID := make(map[string]ContainerState, len(host.Containers)+len(data.UpsertContainers))
		for _, container := range host.Containers {
			byID[container.ID] = container
		}
		for _, container := range data.UpsertContainers {
			byID[container.ID] = convertContainerInfo(container)
		}
		for _, containerID := range data.RemoveContainerIDs {
			delete(byID, containerID)
			delete(host.Stats, containerID)
		}
		host.Containers = host.Containers[:0]
		for _, container := range byID {
			host.Containers = append(host.Containers, container)
		}
		sortContainerStates(host.Containers)
	}

	if len(data.UpsertImages) > 0 || len(data.RemoveImageIDs) > 0 {
		byID := make(map[string]ImageState, len(host.Images)+len(data.UpsertImages))
		for _, image := range host.Images {
			byID[image.ID] = image
		}
		for _, image := range data.UpsertImages {
			byID[image.ID] = ImageState{
				ID:      image.ID,
				Tags:    append([]string(nil), image.Tags...),
				Size:    image.Size,
				Created: image.Created,
			}
		}
		for _, imageID := range data.RemoveImageIDs {
			delete(byID, imageID)
		}
		host.Images = host.Images[:0]
		for _, image := range byID {
			host.Images = append(host.Images, image)
		}
		sortImageStates(host.Images)
	}

	if len(data.UpsertNetworks) > 0 || len(data.RemoveNetworkIDs) > 0 {
		byID := make(map[string]NetworkState, len(host.Networks)+len(data.UpsertNetworks))
		for _, network := range host.Networks {
			byID[network.ID] = network
		}
		for _, network := range data.UpsertNetworks {
			byID[network.ID] = NetworkState{
				ID:     network.ID,
				Name:   network.Name,
				Driver: network.Driver,
				Scope:  network.Scope,
			}
		}
		for _, networkID := range data.RemoveNetworkIDs {
			delete(byID, networkID)
		}
		host.Networks = host.Networks[:0]
		for _, network := range byID {
			host.Networks = append(host.Networks, network)
		}
		sortNetworkStates(host.Networks)
	}

	if len(data.UpsertVolumes) > 0 || len(data.RemoveVolumeNames) > 0 {
		byName := make(map[string]VolumeState, len(host.Volumes)+len(data.UpsertVolumes))
		for _, volume := range host.Volumes {
			byName[volume.Name] = volume
		}
		for _, volume := range data.UpsertVolumes {
			byName[volume.Name] = VolumeState{
				Name:       volume.Name,
				Driver:     volume.Driver,
				Mountpoint: volume.Mountpoint,
			}
		}
		for _, volumeName := range data.RemoveVolumeNames {
			delete(byName, volumeName)
		}
		host.Volumes = host.Volumes[:0]
		for _, volume := range byName {
			host.Volumes = append(host.Volumes, volume)
		}
		sortVolumeStates(host.Volumes)
	}

	if data.ReplaceComposeStacks {
		host.ComposeStacks = host.ComposeStacks[:0]
		for _, stack := range data.ComposeStacks {
			host.ComposeStacks = append(host.ComposeStacks, convertComposeStack(stack))
		}
		sortComposeStackStates(host.ComposeStacks)
	}
}

func convertContainerInfo(ct agentmgr.DockerContainerInfo) ContainerState {
	container := ContainerState{
		ID:      ct.ID,
		Name:    ct.Name,
		Image:   ct.Image,
		State:   ct.State,
		Status:  ct.Status,
		Created: ct.Created,
		Labels:  cloneStringMap(ct.Labels),
	}
	if len(ct.Ports) > 0 {
		portStrs := make([]string, 0, len(ct.Ports))
		for _, port := range ct.Ports {
			portStrs = append(portStrs, fmt.Sprintf("%d->%d/%s", port.Host, port.Container, port.Protocol))
		}
		container.Ports = joinStrings(portStrs, ", ")
	}
	if project, ok := ct.Labels["com.docker.compose.project"]; ok {
		container.StackName = project
	}
	return container
}

func convertComposeStack(stack agentmgr.DockerComposeStack) ComposeStackState {
	return ComposeStackState{
		Name:         stack.Name,
		Status:       stack.Status,
		ConfigFile:   stack.ConfigFile,
		ContainerIDs: append([]string(nil), stack.Containers...),
	}
}

func sortContainerStates(containers []ContainerState) {
	sort.Slice(containers, func(i, j int) bool {
		if containers[i].Name == containers[j].Name {
			return containers[i].ID < containers[j].ID
		}
		return containers[i].Name < containers[j].Name
	})
}

func sortImageStates(images []ImageState) {
	sort.Slice(images, func(i, j int) bool {
		return images[i].ID < images[j].ID
	})
}

func sortNetworkStates(networks []NetworkState) {
	sort.Slice(networks, func(i, j int) bool {
		if networks[i].Name == networks[j].Name {
			return networks[i].ID < networks[j].ID
		}
		return networks[i].Name < networks[j].Name
	})
}

func sortVolumeStates(volumes []VolumeState) {
	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].Name < volumes[j].Name
	})
}

func sortComposeStackStates(stacks []ComposeStackState) {
	sort.Slice(stacks, func(i, j int) bool {
		return stacks[i].Name < stacks[j].Name
	})
}

// HandleStats processes a docker.stats message from an agent.
func (c *Coordinator) HandleStats(agentID string, msg agentmgr.Message) {
	var data agentmgr.DockerStatsData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("docker-coordinator: invalid stats from %s: %v", agentID, err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	host, ok := c.hosts[agentID]
	if !ok {
		return // haven't received discovery yet; discard
	}
	host.LastSeen = time.Now()
	nextStats := make(map[string]ContainerStats, len(data.Containers))
	for _, cs := range data.Containers {
		nextStats[cs.ID] = ContainerStats{
			CPUPercent:      cs.CPUPercent,
			MemoryBytes:     cs.MemoryBytes,
			MemoryLimit:     cs.MemoryLimit,
			MemoryPercent:   cs.MemoryPercent,
			NetRXBytes:      cs.NetRXBytes,
			NetTXBytes:      cs.NetTXBytes,
			BlockReadBytes:  cs.BlockReadBytes,
			BlockWriteBytes: cs.BlockWriteBytes,
			PIDs:            cs.PIDs,
		}
	}
	host.Stats = nextStats
}

// HandleEvent processes a docker.events message from an agent.
// Container state changes are captured by the next discovery cycle; this
// handler only updates the LastSeen timestamp for now.
func (c *Coordinator) HandleEvent(agentID string, msg agentmgr.Message) {
	var data agentmgr.DockerEventData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("docker-coordinator: invalid event from %s: %v", agentID, err)
		return
	}

	c.mu.Lock()
	if host, ok := c.hosts[agentID]; ok {
		host.LastSeen = time.Now()
	}
	c.mu.Unlock()
}

// HandleActionResult processes a docker.action.result message from an agent.
// It delivers the result to any waiting ExecuteAction call via a pending channel.
func (c *Coordinator) HandleActionResult(agentID string, msg agentmgr.Message) {
	var data agentmgr.DockerActionResultData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("docker-coordinator: invalid action result from %s: %v", agentID, err)
		return
	}

	c.pendingMu.Lock()
	ch, ok := c.pendingResults[data.RequestID]
	if ok {
		delete(c.pendingResults, data.RequestID)
	}
	c.pendingMu.Unlock()

	if ok {
		ch <- data
	}
}

// HandleComposeResult processes a docker.compose.result message from an agent.
func (c *Coordinator) HandleComposeResult(agentID string, msg agentmgr.Message) {
	var data agentmgr.DockerComposeResultData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("docker-coordinator: invalid compose result from %s: %v", agentID, err)
		return
	}

	c.pendingMu.Lock()
	ch, ok := c.pendingCompose[data.RequestID]
	if ok {
		delete(c.pendingCompose, data.RequestID)
	}
	c.pendingMu.Unlock()

	if ok {
		ch <- data
	}
}

// RemoveHost removes a Docker host entry when its agent disconnects.
func (c *Coordinator) RemoveHost(agentID string) {
	c.mu.Lock()
	delete(c.hosts, agentID)
	c.mu.Unlock()
}

// joinStrings concatenates a slice of strings with a separator without importing strings.Join.
func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
