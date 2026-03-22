package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

const (
	defaultDockerDiscoveryInterval        = 30 * time.Second
	defaultDockerFullReconcileMinInterval = 5 * time.Minute
	defaultDockerFullReconcileMaxInterval = 30 * time.Minute
	dockerDiscoveryDebounce               = 1200 * time.Millisecond
	dockerDiscoveryImmediateDebounce      = 350 * time.Millisecond

	maxContainersPerHost = 500

	dockerStatsPollMinInterval = 5 * time.Second
	dockerStatsPollMaxInterval = 20 * time.Second
	dockerStatsMaxSamplesTick  = 24
	dockerStatsHotCPUThreshold = 25.0
	dockerStatsHotMemThreshold = 70.0
	dockerStatsHotPIDThreshold = 150
)

type dockerDiscoveryTrigger struct {
	full      bool
	immediate bool
}

type dockerInventorySnapshot struct {
	Engine        agentmgr.DockerEngineInfo
	Containers    map[string]agentmgr.DockerContainerInfo
	Images        map[string]agentmgr.DockerImageInfo
	Networks      map[string]agentmgr.DockerNetworkInfo
	Volumes       map[string]agentmgr.DockerVolumeInfo
	ComposeStacks []agentmgr.DockerComposeStack
}

type dockerStatsSchedule struct {
	interval   time.Duration
	nextSample time.Time
	lastStats  agentmgr.DockerContainerStats
	hasSample  bool
}

// DockerCollector manages Docker discovery and stats collection on the agent side.
type DockerCollector struct {
	client     *dockerClient
	transport  Transport
	socketPath string
	assetID    string // agent's own asset ID, used as host_id
	interval   time.Duration

	fullReconcileInterval time.Duration
	discoveryTriggerCh    chan dockerDiscoveryTrigger
	statsTriggerCh        chan struct{}

	inventoryMu         sync.RWMutex
	inventory           dockerInventorySnapshot
	runningContainerIDs map[string]struct{}
	hasPublishedFull    bool

	statsMu            sync.Mutex
	statsSchedule      map[string]*dockerStatsSchedule
	lastRunningSetHash string
	lastStatsPublish   time.Time
}

// NewDockerCollector creates a collector. socketPath is the Docker socket path.
// transport is used to send messages to the hub.
func NewDockerCollector(socketPath string, transport Transport, assetID string, interval time.Duration) *DockerCollector {
	if interval <= 0 {
		interval = defaultDockerDiscoveryInterval
	}
	return &DockerCollector{
		client:                NewDockerClient(socketPath),
		transport:             transport,
		socketPath:            socketPath,
		assetID:               assetID,
		interval:              interval,
		fullReconcileInterval: deriveDockerFullReconcileInterval(interval),
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

func deriveDockerFullReconcileInterval(base time.Duration) time.Duration {
	interval := base * 10
	if interval < defaultDockerFullReconcileMinInterval {
		interval = defaultDockerFullReconcileMinInterval
	}
	if interval > defaultDockerFullReconcileMaxInterval {
		interval = defaultDockerFullReconcileMaxInterval
	}
	return interval
}

func deriveDockerStatsPollInterval(base time.Duration) time.Duration {
	poll := base / 2
	if poll < dockerStatsPollMinInterval {
		poll = dockerStatsPollMinInterval
	}
	if poll > dockerStatsPollMaxInterval {
		poll = dockerStatsPollMaxInterval
	}
	return poll
}

// IsAvailable checks if the Docker socket exists and is accessible.
func (dc *DockerCollector) IsAvailable() bool {
	endpoint := strings.TrimSpace(dc.socketPath)
	if endpoint == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(endpoint), "unix://") {
		endpoint = strings.TrimPrefix(endpoint, "unix://")
	}
	if strings.HasPrefix(endpoint, "/") {
		if _, err := os.Stat(endpoint); err != nil {
			return false
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return dc.client.ping(ctx) == nil
}

// ResetPublishedState clears the published-full flag so the next discovery
// cycle forces a full republish. Call this after a WebSocket reconnect so the
// hub receives fresh Docker inventory immediately.
func (dc *DockerCollector) ResetPublishedState() {
	dc.inventoryMu.Lock()
	dc.hasPublishedFull = false
	dc.inventoryMu.Unlock()
}

func (dc *DockerCollector) transportConnected() bool {
	return dc.transport != nil && dc.transport.Connected()
}

func (dc *DockerCollector) queueDiscoveryTrigger(full, immediate bool) {
	if dc == nil || dc.discoveryTriggerCh == nil {
		return
	}
	trigger := dockerDiscoveryTrigger{full: full, immediate: immediate}
	select {
	case dc.discoveryTriggerCh <- trigger:
	default:
		if full {
			// Preserve full refresh requests under bursty event load.
			select {
			case <-dc.discoveryTriggerCh:
			default:
			}
			select {
			case dc.discoveryTriggerCh <- trigger:
			default:
			}
		}
	}
}

func (dc *DockerCollector) queueStatsTrigger() {
	if dc == nil || dc.statsTriggerCh == nil {
		return
	}
	select {
	case dc.statsTriggerCh <- struct{}{}:
	default:
	}
}

// Run starts periodic discovery, event streaming, and stats collection.
// Blocks until ctx is cancelled.
func (dc *DockerCollector) Run(ctx context.Context) {
	go dc.runEventLoop(ctx)
	go dc.runStats(ctx)
	dc.runDiscoveryManager(ctx)
}

func (dc *DockerCollector) runDiscoveryManager(ctx context.Context) {
	cheapTicker := time.NewTicker(dc.interval)
	defer cheapTicker.Stop()

	fullTicker := time.NewTicker(dc.fullReconcileInterval)
	defer fullTicker.Stop()

	var (
		debounceTimer    *time.Timer
		debounceC        <-chan time.Time
		pendingContainer bool
		pendingFull      bool
		forceFullPublish bool
	)

	armDebounce := func(delay time.Duration) {
		if debounceTimer == nil {
			debounceTimer = time.NewTimer(delay)
			debounceC = debounceTimer.C
			return
		}
		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
		debounceTimer.Reset(delay)
		debounceC = debounceTimer.C
	}

	pendingFull = true
	forceFullPublish = true
	armDebounce(0)

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				if !debounceTimer.Stop() {
					select {
					case <-debounceTimer.C:
					default:
					}
				}
			}
			return
		case <-cheapTicker.C:
			pendingContainer = true
			armDebounce(dockerDiscoveryDebounce)
		case <-fullTicker.C:
			pendingFull = true
			forceFullPublish = true // periodic snapshot keeps hub state fully reconciled
			armDebounce(dockerDiscoveryDebounce)
		case trigger := <-dc.discoveryTriggerCh:
			if trigger.full {
				pendingFull = true
			} else {
				pendingContainer = true
			}
			if trigger.immediate {
				armDebounce(dockerDiscoveryImmediateDebounce)
			} else {
				armDebounce(dockerDiscoveryDebounce)
			}
		case <-debounceC:
			debounceC = nil
			if pendingFull {
				pendingFull = false
				pendingContainer = false
				if _, err := dc.refreshAndPublishFull(ctx, forceFullPublish); err != nil {
					log.Printf("docker: full discovery refresh failed: %v", err)
				}
				forceFullPublish = false
				continue
			}
			if pendingContainer {
				pendingContainer = false
				if _, err := dc.refreshAndPublishContainerDelta(ctx); err != nil {
					log.Printf("docker: container delta refresh failed: %v", err)
				}
			}
		}
	}
}

func (dc *DockerCollector) refreshAndPublishFull(ctx context.Context, forcePublish bool) (bool, error) {
	if !dc.transportConnected() {
		return false, nil
	}

	discovery, snapshot, runningIDs, err := dc.collectFullDiscovery(ctx)
	if err != nil {
		return false, err
	}

	dc.inventoryMu.RLock()
	changed := !dockerInventorySnapshotsEqual(dc.inventory, snapshot)
	hasPublished := dc.hasPublishedFull
	dc.inventoryMu.RUnlock()

	if !changed && !forcePublish && hasPublished {
		dc.updateRunningContainerIDs(runningIDs)
		return false, nil
	}

	if err := dc.sendDockerMessage(agentmgr.MsgDockerDiscovery, discovery); err != nil {
		dc.updateRunningContainerIDs(runningIDs)
		return false, err
	}
	dc.replaceInventory(snapshot, runningIDs)
	dc.inventoryMu.Lock()
	dc.hasPublishedFull = true
	dc.inventoryMu.Unlock()
	return true, nil
}

func (dc *DockerCollector) refreshAndPublishContainerDelta(ctx context.Context) (bool, error) {
	if !dc.transportConnected() {
		return false, nil
	}

	dc.inventoryMu.RLock()
	hasPublished := dc.hasPublishedFull
	previousContainers := cloneDockerContainerInfoMap(dc.inventory.Containers)
	previousCompose := cloneComposeStacks(dc.inventory.ComposeStacks)
	dc.inventoryMu.RUnlock()

	if !hasPublished {
		return dc.refreshAndPublishFull(ctx, true)
	}

	rawContainers, err := dc.client.listContainers(ctx)
	if err != nil {
		return false, fmt.Errorf("containers: %w", err)
	}
	if len(rawContainers) > maxContainersPerHost {
		rawContainers = rawContainers[:maxContainersPerHost]
	}
	nextContainers, runningIDs := buildContainerInfoMap(rawContainers)
	nextCompose := inferComposeStacks(rawContainers)

	upserts, removals := diffContainerInfoMap(previousContainers, nextContainers)
	composeChanged := !composeStacksEqual(previousCompose, nextCompose)
	changeCount := len(upserts) + len(removals)
	if changeCount == 0 && !composeChanged {
		dc.updateRunningContainerIDs(runningIDs)
		return false, nil
	}
	if shouldFallbackToFull(changeCount, len(previousContainers), composeChanged) {
		return dc.refreshAndPublishFull(ctx, false)
	}

	delta := agentmgr.DockerDiscoveryDeltaData{
		HostID:             dc.assetID,
		UpsertContainers:   upserts,
		RemoveContainerIDs: removals,
	}
	if composeChanged {
		delta.ReplaceComposeStacks = true
		delta.ComposeStacks = cloneComposeStacks(nextCompose)
	}

	if err := dc.sendDockerMessage(agentmgr.MsgDockerDiscoveryDelta, delta); err != nil {
		dc.updateRunningContainerIDs(runningIDs)
		return false, err
	}

	dc.inventoryMu.Lock()
	dc.inventory.Containers = nextContainers
	dc.inventory.ComposeStacks = cloneComposeStacks(nextCompose)
	dc.runningContainerIDs = cloneStringSet(runningIDs)
	dc.inventoryMu.Unlock()
	return true, nil
}

func shouldFallbackToFull(changeCount, previousCount int, composeChanged bool) bool {
	if changeCount >= 200 {
		return true
	}
	if previousCount == 0 {
		return false
	}
	// When most containers churn in one cycle, a full snapshot is cheaper to apply.
	if changeCount*2 > previousCount {
		return true
	}
	if composeChanged && changeCount > 100 {
		return true
	}
	return false
}

func (dc *DockerCollector) sendDockerMessage(messageType string, payload any) error {
	if dc.transport == nil {
		return fmt.Errorf("transport unavailable")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := dc.transport.Send(agentmgr.Message{Type: messageType, Data: raw}); err != nil {
		return err
	}
	return nil
}

func (dc *DockerCollector) replaceInventory(snapshot dockerInventorySnapshot, runningIDs map[string]struct{}) {
	dc.inventoryMu.Lock()
	dc.inventory = snapshot
	dc.runningContainerIDs = cloneStringSet(runningIDs)
	dc.inventoryMu.Unlock()
}

func (dc *DockerCollector) updateRunningContainerIDs(runningIDs map[string]struct{}) {
	dc.inventoryMu.Lock()
	dc.runningContainerIDs = cloneStringSet(runningIDs)
	dc.inventoryMu.Unlock()
}

func (dc *DockerCollector) currentRunningContainerIDs() []string {
	dc.inventoryMu.RLock()
	ids := make([]string, 0, len(dc.runningContainerIDs))
	for containerID := range dc.runningContainerIDs {
		ids = append(ids, containerID)
	}
	dc.inventoryMu.RUnlock()
	sort.Strings(ids)
	return ids
}

func (dc *DockerCollector) collectFullDiscovery(ctx context.Context) (agentmgr.DockerDiscoveryData, dockerInventorySnapshot, map[string]struct{}, error) {
	result := agentmgr.DockerDiscoveryData{HostID: dc.assetID}
	snapshot := dockerInventorySnapshot{
		Containers: make(map[string]agentmgr.DockerContainerInfo),
		Images:     make(map[string]agentmgr.DockerImageInfo),
		Networks:   make(map[string]agentmgr.DockerNetworkInfo),
		Volumes:    make(map[string]agentmgr.DockerVolumeInfo),
	}

	ver, err := dc.client.version(ctx)
	if err != nil {
		return result, snapshot, nil, fmt.Errorf("version: %w", err)
	}
	result.Engine = agentmgr.DockerEngineInfo{
		Version:    ver.Version,
		APIVersion: ver.APIVersion,
		OS:         ver.Os,
		Arch:       ver.Arch,
	}
	snapshot.Engine = result.Engine

	rawContainers, err := dc.client.listContainers(ctx)
	if err != nil {
		return result, snapshot, nil, fmt.Errorf("containers: %w", err)
	}
	if len(rawContainers) > maxContainersPerHost {
		rawContainers = rawContainers[:maxContainersPerHost]
	}
	var runningIDs map[string]struct{}
	snapshot.Containers, runningIDs = buildContainerInfoMap(rawContainers)
	result.Containers = containerInfoMapToSlice(snapshot.Containers)
	result.ComposeStacks = inferComposeStacks(rawContainers)
	snapshot.ComposeStacks = cloneComposeStacks(result.ComposeStacks)

	if images, err := dc.client.listImages(ctx); err == nil {
		snapshot.Images = buildImageInfoMap(images)
		result.Images = imageInfoMapToSlice(snapshot.Images)
	}
	if networks, err := dc.client.listNetworks(ctx); err == nil {
		snapshot.Networks = buildNetworkInfoMap(networks)
		result.Networks = networkInfoMapToSlice(snapshot.Networks)
	}
	if volumes, err := dc.client.listVolumes(ctx); err == nil {
		snapshot.Volumes = buildVolumeInfoMap(volumes)
		result.Volumes = volumeInfoMapToSlice(snapshot.Volumes)
	}

	return result, snapshot, runningIDs, nil
}

// runStats periodically collects per-container stats and sends them to the hub.
func (dc *DockerCollector) runStats(ctx context.Context) {
	ticker := time.NewTicker(deriveDockerStatsPollInterval(dc.interval))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dc.collectAndSendStats(ctx)
		case <-dc.statsTriggerCh:
			dc.collectAndSendStats(ctx)
		}
	}
}

func (dc *DockerCollector) collectAndSendStats(ctx context.Context) {
	if !dc.transportConnected() {
		return
	}

	now := time.Now()
	runningIDs := dc.currentRunningContainerIDs()
	runningHash := strings.Join(runningIDs, ",")
	runningSet := make(map[string]struct{}, len(runningIDs))
	for _, containerID := range runningIDs {
		runningSet[containerID] = struct{}{}
	}

	dc.statsMu.Lock()
	for id := range dc.statsSchedule {
		if _, ok := runningSet[id]; !ok {
			delete(dc.statsSchedule, id)
		}
	}
	for _, id := range runningIDs {
		if _, ok := dc.statsSchedule[id]; !ok {
			base := dc.defaultStatsInterval()
			dc.statsSchedule[id] = &dockerStatsSchedule{interval: base, nextSample: now}
		}
	}
	var due []string
	for _, id := range runningIDs {
		schedule := dc.statsSchedule[id]
		if schedule == nil {
			continue
		}
		if !schedule.hasSample || !now.Before(schedule.nextSample) {
			due = append(due, id)
		}
	}
	if len(due) > dockerStatsMaxSamplesTick {
		due = due[:dockerStatsMaxSamplesTick]
	}
	dc.statsMu.Unlock()

	sampled := 0
	for _, containerID := range due {
		raw, err := dc.client.containerStats(ctx, containerID)
		now = time.Now()
		dc.statsMu.Lock()
		schedule := dc.statsSchedule[containerID]
		if schedule == nil {
			dc.statsMu.Unlock()
			continue
		}
		if err != nil {
			schedule.interval = dc.nextStatsInterval(schedule.interval, agentmgr.DockerContainerStats{}, true)
			schedule.nextSample = now.Add(schedule.interval)
			dc.statsMu.Unlock()
			continue
		}
		stats := calculateStats(containerID, raw)
		schedule.lastStats = stats
		schedule.hasSample = true
		schedule.interval = dc.nextStatsInterval(schedule.interval, stats, false)
		schedule.nextSample = now.Add(schedule.interval)
		dc.statsMu.Unlock()
		sampled++
	}

	dc.statsMu.Lock()
	payloadStats := make([]agentmgr.DockerContainerStats, 0, len(runningIDs))
	for _, containerID := range runningIDs {
		schedule := dc.statsSchedule[containerID]
		if schedule != nil && schedule.hasSample {
			payloadStats = append(payloadStats, schedule.lastStats)
		}
	}
	sort.Slice(payloadStats, func(i, j int) bool {
		return payloadStats[i].ID < payloadStats[j].ID
	})
	shouldSend := sampled > 0 || runningHash != dc.lastRunningSetHash || (len(runningIDs) > 0 && now.Sub(dc.lastStatsPublish) >= dc.interval)
	dc.statsMu.Unlock()

	if !shouldSend {
		return
	}

	payload := agentmgr.DockerStatsData{HostID: dc.assetID, Containers: payloadStats}
	if err := dc.sendDockerMessage(agentmgr.MsgDockerStats, payload); err != nil {
		log.Printf("docker: failed to send stats: %v", err)
		return
	}

	dc.statsMu.Lock()
	dc.lastRunningSetHash = runningHash
	dc.lastStatsPublish = time.Now()
	dc.statsMu.Unlock()
}

func (dc *DockerCollector) defaultStatsInterval() time.Duration {
	base := dc.interval
	if base <= 0 {
		base = defaultDockerDiscoveryInterval
	}
	if base < dockerStatsPollMinInterval {
		base = dockerStatsPollMinInterval
	}
	return base
}

func (dc *DockerCollector) minStatsInterval() time.Duration {
	interval := dc.interval / 2
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	if dc.interval > 0 && interval > dc.interval {
		interval = dc.interval
	}
	if interval < dockerStatsPollMinInterval {
		interval = dockerStatsPollMinInterval
	}
	return interval
}

func (dc *DockerCollector) maxStatsInterval() time.Duration {
	interval := dc.interval * 6
	if interval < 2*time.Minute {
		interval = 2 * time.Minute
	}
	if interval > 10*time.Minute {
		interval = 10 * time.Minute
	}
	return interval
}

func (dc *DockerCollector) nextStatsInterval(current time.Duration, stats agentmgr.DockerContainerStats, hadError bool) time.Duration {
	if current <= 0 {
		current = dc.defaultStatsInterval()
	}
	minInterval := dc.minStatsInterval()
	maxInterval := dc.maxStatsInterval()
	if hadError {
		next := current * 2
		if next > maxInterval {
			next = maxInterval
		}
		return next
	}
	isHot := stats.CPUPercent >= dockerStatsHotCPUThreshold || stats.MemoryPercent >= dockerStatsHotMemThreshold || stats.PIDs >= dockerStatsHotPIDThreshold
	if isHot {
		next := current / 2
		if next < minInterval {
			next = minInterval
		}
		return next
	}
	next := current + current/2
	if next > maxInterval {
		next = maxInterval
	}
	return next
}

// calculateStats converts raw Docker stats into our ContainerStats format.
func calculateStats(containerID string, raw DockerStatsResponse) agentmgr.DockerContainerStats {
	// CPU percent: (delta_container / delta_system) * num_cpus * 100
	cpuPercent := 0.0
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage - raw.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(raw.CPUStats.SystemCPUUsage - raw.PreCPUStats.SystemCPUUsage)
	if sysDelta > 0 && cpuDelta > 0 {
		cpus := raw.CPUStats.OnlineCPUs
		if cpus == 0 {
			cpus = 1
		}
		cpuPercent = (cpuDelta / sysDelta) * float64(cpus) * 100.0
	}

	// Memory percent
	memPercent := 0.0
	if raw.MemoryStats.Limit > 0 {
		memPercent = float64(raw.MemoryStats.Usage) / float64(raw.MemoryStats.Limit) * 100.0
	}

	// Network I/O (sum across all interfaces)
	var netRX, netTX int64
	for _, iface := range raw.Networks {
		netRX += iface.RxBytes
		netTX += iface.TxBytes
	}

	// Block I/O
	var blockRead, blockWrite int64
	for _, entry := range raw.BlkioStats.IoServiceBytesRecursive {
		switch entry.Op {
		case "Read", "read":
			blockRead += entry.Value
		case "Write", "write":
			blockWrite += entry.Value
		}
	}

	return agentmgr.DockerContainerStats{
		ID:              containerID,
		CPUPercent:      cpuPercent,
		MemoryBytes:     raw.MemoryStats.Usage,
		MemoryLimit:     raw.MemoryStats.Limit,
		MemoryPercent:   memPercent,
		NetRXBytes:      netRX,
		NetTXBytes:      netTX,
		BlockReadBytes:  blockRead,
		BlockWriteBytes: blockWrite,
		PIDs:            raw.PidsStats.Current,
	}
}

func buildContainerInfoMap(rawContainers []DockerContainer) (map[string]agentmgr.DockerContainerInfo, map[string]struct{}) {
	containers := make(map[string]agentmgr.DockerContainerInfo, len(rawContainers))
	running := make(map[string]struct{})
	for _, container := range rawContainers {
		info := agentmgr.DockerContainerInfo{
			ID:      container.ID,
			Name:    ContainerName(container.Names),
			Image:   container.Image,
			State:   container.State,
			Status:  container.Status,
			Created: time.Unix(container.Created, 0).UTC().Format(time.RFC3339),
			Labels:  cloneStringMap(container.Labels),
		}
		if container.State == "running" {
			running[container.ID] = struct{}{}
		}

		for _, port := range container.Ports {
			if port.PublicPort <= 0 {
				continue
			}
			info.Ports = append(info.Ports, agentmgr.DockerPortMapping{
				Host:      port.PublicPort,
				Container: port.PrivatePort,
				Protocol:  strings.ToLower(port.Type),
			})
		}
		sort.Slice(info.Ports, func(i, j int) bool {
			if info.Ports[i].Host == info.Ports[j].Host {
				if info.Ports[i].Container == info.Ports[j].Container {
					return info.Ports[i].Protocol < info.Ports[j].Protocol
				}
				return info.Ports[i].Container < info.Ports[j].Container
			}
			return info.Ports[i].Host < info.Ports[j].Host
		})

		for networkName := range container.NetworkSettings.Networks {
			info.Networks = append(info.Networks, networkName)
		}
		sort.Strings(info.Networks)

		for _, mount := range container.Mounts {
			info.Mounts = append(info.Mounts, agentmgr.DockerMountInfo{
				Type:        mount.Type,
				Source:      mount.Source,
				Destination: mount.Destination,
			})
		}
		sort.Slice(info.Mounts, func(i, j int) bool {
			if info.Mounts[i].Destination == info.Mounts[j].Destination {
				if info.Mounts[i].Source == info.Mounts[j].Source {
					return info.Mounts[i].Type < info.Mounts[j].Type
				}
				return info.Mounts[i].Source < info.Mounts[j].Source
			}
			return info.Mounts[i].Destination < info.Mounts[j].Destination
		})

		containers[container.ID] = info
	}
	return containers, running
}

func containerInfoMapToSlice(values map[string]agentmgr.DockerContainerInfo) []agentmgr.DockerContainerInfo {
	result := make([]agentmgr.DockerContainerInfo, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func buildImageInfoMap(values []DockerImage) map[string]agentmgr.DockerImageInfo {
	result := make(map[string]agentmgr.DockerImageInfo, len(values))
	for _, image := range values {
		tags := append([]string(nil), image.RepoTags...)
		sort.Strings(tags)
		result[image.ID] = agentmgr.DockerImageInfo{
			ID:      image.ID,
			Tags:    tags,
			Size:    image.Size,
			Created: time.Unix(image.Created, 0).UTC().Format(time.RFC3339),
		}
	}
	return result
}

func imageInfoMapToSlice(values map[string]agentmgr.DockerImageInfo) []agentmgr.DockerImageInfo {
	result := make([]agentmgr.DockerImageInfo, 0, len(values))
	for _, image := range values {
		result = append(result, image)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func buildNetworkInfoMap(values []DockerNetwork) map[string]agentmgr.DockerNetworkInfo {
	result := make(map[string]agentmgr.DockerNetworkInfo, len(values))
	for _, network := range values {
		result[network.ID] = agentmgr.DockerNetworkInfo{
			ID:     network.ID,
			Name:   network.Name,
			Driver: network.Driver,
			Scope:  network.Scope,
		}
	}
	return result
}

func networkInfoMapToSlice(values map[string]agentmgr.DockerNetworkInfo) []agentmgr.DockerNetworkInfo {
	result := make([]agentmgr.DockerNetworkInfo, 0, len(values))
	for _, network := range values {
		result = append(result, network)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func buildVolumeInfoMap(values []DockerVolume) map[string]agentmgr.DockerVolumeInfo {
	result := make(map[string]agentmgr.DockerVolumeInfo, len(values))
	for _, volume := range values {
		result[volume.Name] = agentmgr.DockerVolumeInfo{
			Name:       volume.Name,
			Driver:     volume.Driver,
			Mountpoint: volume.Mountpoint,
		}
	}
	return result
}

func volumeInfoMapToSlice(values map[string]agentmgr.DockerVolumeInfo) []agentmgr.DockerVolumeInfo {
	result := make([]agentmgr.DockerVolumeInfo, 0, len(values))
	for _, volume := range values {
		result = append(result, volume)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func dockerInventorySnapshotsEqual(a, b dockerInventorySnapshot) bool {
	if a.Engine != b.Engine {
		return false
	}
	if !DockerContainerInfoMapsEqual(a.Containers, b.Containers) {
		return false
	}
	if !DockerImageInfoMapsEqual(a.Images, b.Images) {
		return false
	}
	if !DockerNetworkInfoMapsEqual(a.Networks, b.Networks) {
		return false
	}
	if !DockerVolumeInfoMapsEqual(a.Volumes, b.Volumes) {
		return false
	}
	return composeStacksEqual(a.ComposeStacks, b.ComposeStacks)
}

func DockerContainerInfoMapsEqual(a, b map[string]agentmgr.DockerContainerInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for id, left := range a {
		right, ok := b[id]
		if !ok || !DockerContainerInfoEqual(left, right) {
			return false
		}
	}
	return true
}

func DockerContainerInfoEqual(a, b agentmgr.DockerContainerInfo) bool {
	if a.ID != b.ID || a.Name != b.Name || a.Image != b.Image || a.State != b.State || a.Status != b.Status || a.Created != b.Created {
		return false
	}
	if !DockerPortMappingsEqual(a.Ports, b.Ports) {
		return false
	}
	if !stringSlicesEqual(a.Networks, b.Networks) {
		return false
	}
	if !stringMapsEqual(a.Labels, b.Labels) {
		return false
	}
	if len(a.Mounts) != len(b.Mounts) {
		return false
	}
	for i := range a.Mounts {
		if a.Mounts[i] != b.Mounts[i] {
			return false
		}
	}
	return true
}

func DockerPortMappingsEqual(a, b []agentmgr.DockerPortMapping) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func diffContainerInfoMap(previous, next map[string]agentmgr.DockerContainerInfo) ([]agentmgr.DockerContainerInfo, []string) {
	upserts := make([]agentmgr.DockerContainerInfo, 0)
	removals := make([]string, 0)
	for id, nextContainer := range next {
		prevContainer, ok := previous[id]
		if !ok || !DockerContainerInfoEqual(prevContainer, nextContainer) {
			upserts = append(upserts, nextContainer)
		}
	}
	for id := range previous {
		if _, ok := next[id]; !ok {
			removals = append(removals, id)
		}
	}
	sort.Slice(upserts, func(i, j int) bool {
		if upserts[i].Name == upserts[j].Name {
			return upserts[i].ID < upserts[j].ID
		}
		return upserts[i].Name < upserts[j].Name
	})
	sort.Strings(removals)
	return upserts, removals
}

func DockerImageInfoMapsEqual(a, b map[string]agentmgr.DockerImageInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for id, left := range a {
		right, ok := b[id]
		if !ok || left.ID != right.ID || left.Size != right.Size || left.Created != right.Created || !stringSlicesEqual(left.Tags, right.Tags) {
			return false
		}
	}
	return true
}

func DockerNetworkInfoMapsEqual(a, b map[string]agentmgr.DockerNetworkInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for id, left := range a {
		right, ok := b[id]
		if !ok || left != right {
			return false
		}
	}
	return true
}

func DockerVolumeInfoMapsEqual(a, b map[string]agentmgr.DockerVolumeInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for name, left := range a {
		right, ok := b[name]
		if !ok || left != right {
			return false
		}
	}
	return true
}

func composeStacksEqual(a, b []agentmgr.DockerComposeStack) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	left := make(map[string]agentmgr.DockerComposeStack, len(a))
	for _, stack := range a {
		left[stack.Name] = stack
	}
	for _, stack := range b {
		other, ok := left[stack.Name]
		if !ok {
			return false
		}
		if other.Status != stack.Status || other.ConfigFile != stack.ConfigFile || !stringSlicesEqual(other.Containers, stack.Containers) {
			return false
		}
	}
	return true
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func cloneDockerContainerInfoMap(source map[string]agentmgr.DockerContainerInfo) map[string]agentmgr.DockerContainerInfo {
	if len(source) == 0 {
		return make(map[string]agentmgr.DockerContainerInfo)
	}
	result := make(map[string]agentmgr.DockerContainerInfo, len(source))
	for id, container := range source {
		copyContainer := container
		copyContainer.Labels = cloneStringMap(container.Labels)
		copyContainer.Ports = append([]agentmgr.DockerPortMapping(nil), container.Ports...)
		copyContainer.Networks = append([]string(nil), container.Networks...)
		copyContainer.Mounts = append([]agentmgr.DockerMountInfo(nil), container.Mounts...)
		result[id] = copyContainer
	}
	return result
}

func cloneComposeStacks(source []agentmgr.DockerComposeStack) []agentmgr.DockerComposeStack {
	if len(source) == 0 {
		return nil
	}
	result := make([]agentmgr.DockerComposeStack, len(source))
	for i, stack := range source {
		result[i] = stack
		result[i].Containers = append([]string(nil), stack.Containers...)
	}
	return result
}

func cloneStringSet(source map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(source))
	for key := range source {
		result[key] = struct{}{}
	}
	return result
}

// inferComposeStacks groups containers by com.docker.compose.project label.
func inferComposeStacks(containers []DockerContainer) []agentmgr.DockerComposeStack {
	type composeAccumulator struct {
		stack       *agentmgr.DockerComposeStack
		running     int
		containerID map[string]struct{}
	}
	stacks := make(map[string]*composeAccumulator)

	for _, c := range containers {
		project := c.Labels["com.docker.compose.project"]
		if project == "" {
			continue
		}
		acc, exists := stacks[project]
		if !exists {
			stack := &agentmgr.DockerComposeStack{Name: project, Status: "running(0)"}
			if dir := c.Labels["com.docker.compose.project.working_dir"]; dir != "" {
				stack.ConfigFile = dir + "/docker-compose.yml"
			}
			acc = &composeAccumulator{
				stack:       stack,
				containerID: make(map[string]struct{}),
			}
			stacks[project] = acc
		}
		name := ContainerName(c.Names)
		if name != "" {
			if _, seen := acc.containerID[name]; !seen {
				acc.stack.Containers = append(acc.stack.Containers, name)
				acc.containerID[name] = struct{}{}
			}
		}
		if c.State == "running" {
			acc.running++
		}
	}

	result := make([]agentmgr.DockerComposeStack, 0, len(stacks))
	for _, acc := range stacks {
		sort.Strings(acc.stack.Containers)
		acc.stack.Status = fmt.Sprintf("running(%d)", acc.running)
		result = append(result, *acc.stack)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}
