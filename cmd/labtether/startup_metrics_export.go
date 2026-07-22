package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectors/webservice"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/synthetic"
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/telemetry/bridge"
	"github.com/labtether/labtether/internal/telemetry/promexport"
	"golang.org/x/sync/singleflight"
)

// initMetricsExport wires the bridge registry and Prometheus scrape adapter
// into the server. It must be called before startRuntimeLoops so that the
// registry is available when Run is invoked.
//
// Every registered bridge below has a bounded production source. Proxmox is
// intentionally handled by its existing collector heartbeat telemetry, while
// PBS emits its richer datastore metrics in the existing PBS collector pass;
// neither connector has a second export-only polling loop.
func initMetricsExport(srv *apiServer, pgStore *persistence.PostgresStore) {
	reg := bridge.NewRegistry()
	srv.bridgeRegistry = reg

	// Docker stats bridge — collects per-container CPU, memory, network, block I/O.
	if srv.dockerCoordinator != nil {
		reg.Register(bridge.NewDockerStatsBridge(newDockerStatsAdapter(srv.dockerCoordinator)))
	}

	// Alert state bridge — counts firing instances and active rules.
	if srv.alertStore != nil && srv.alertInstanceStore != nil {
		metricsSnapshotStore, _ := srv.alertStore.(persistence.AlertMetricsSnapshotStore)
		if metricsSnapshotStore == nil {
			memoryRules, rulesOK := srv.alertStore.(*persistence.MemoryAlertStore)
			memoryInstances, instancesOK := srv.alertInstanceStore.(*persistence.MemoryAlertInstanceStore)
			if rulesOK && instancesOK {
				metricsSnapshotStore = persistence.NewMemoryAlertMetricsSnapshotStore(memoryRules, memoryInstances)
			}
		}
		if metricsSnapshotStore != nil {
			reg.Register(bridge.NewAlertStateBridge(&alertStateAdapter{
				metricsSnapshotStore: metricsSnapshotStore,
			}))
		}
	}

	// Agent presence bridge — per-agent connection state and heartbeat age.
	if srv.agentMgr != nil && srv.assetStore != nil {
		reg.Register(bridge.NewAgentPresenceBridge(&agentPresenceAdapter{
			agentMgr:   srv.agentMgr,
			assetStore: srv.assetStore,
		}))
	}

	// Agent resource bridges share the existing request/response channels used
	// by the operator APIs. Each sweep rotates through a fixed-size connected
	// asset batch with fixed concurrency and per-request timeouts. Process
	// collection is registered but remains dormant until the effective runtime
	// setting explicitly enables it.
	if srv.agentMgr != nil {
		agentSource := newAgentTelemetryAdapter(srv)
		reg.Register(bridge.NewProcessMetricsBridge(agentSource))
		reg.Register(bridge.NewNetworkInterfacesBridge(agentSource))
		reg.Register(bridge.NewDiskMountsBridge(agentSource))
	}

	if srv.webServiceCoordinator != nil {
		reg.Register(bridge.NewServiceHealthBridge(&serviceHealthMetricsAdapter{
			coordinator: srv.webServiceCoordinator,
		}))
	}

	if snapshotStore, ok := srv.syntheticStore.(persistence.SyntheticMetricSnapshotStore); ok {
		reg.Register(bridge.NewSyntheticChecksBridge(&syntheticMetricsAdapter{
			snapshotStore:     snapshotStore,
			lastResultByCheck: make(map[string]string),
		}))
	}

	// Site reliability bridge — per-group reliability scores. Prefer the
	// composite store so one bounded query replaces ListGroups plus one history
	// query per group.
	var reliabilitySnapshotStore persistence.ReliabilityMetricSnapshotStore
	if candidate, ok := srv.groupStore.(persistence.ReliabilityMetricSnapshotStore); ok {
		reliabilitySnapshotStore = candidate
	} else if pgStore != nil {
		reliabilitySnapshotStore = pgStore
	}
	if reliabilitySnapshotStore != nil {
		reg.Register(bridge.NewSiteReliabilityBridge(&siteReliabilityAdapter{
			snapshotStore: reliabilitySnapshotStore,
		}))
	}
}

// startMetricsExport starts the bridge collection loop. It is called from
// startRuntimeLoops after initMetricsExport, so the registry is already
// populated.
func startMetricsExport(ctx context.Context, srv *apiServer) {
	if srv.bridgeRegistry == nil {
		return
	}
	srv.bridgeRegistry.Run(ctx, func(ctx context.Context, samples []telemetry.MetricSample) error {
		if srv.telemetryStore == nil {
			return nil
		}
		flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return srv.telemetryStore.AppendSamples(flushCtx, samples)
	})
}

// newPrometheusSnapshotSource builds the SnapshotSource backed by the real
// telemetry and asset stores. Called from handlePrometheusMetrics when the
// server is fully initialised.
func newPrometheusSnapshotSource(srv *apiServer) promexport.SnapshotSource {
	labeledStore, ok := srv.telemetryStore.(persistence.TelemetryLabeledSnapshotStore)
	if !ok || srv.assetStore == nil {
		return promexport.NoopSnapshotSource{}
	}
	hubMetricStore, _ := srv.telemetryStore.(persistence.TelemetryHubMetricStore)
	return &cachedSnapshotAdapter{
		inner: &prometheusSnapshotAdapter{
			telemetryStore: labeledStore,
			hubMetricStore: hubMetricStore,
			assetStore:     srv.assetStore,
		},
		cacheTTL: 5 * time.Second,
	}
}

// prometheusSnapshotAdapter fetches asset data directly from the stores. It is
// wrapped by cachedSnapshotAdapter to avoid the double ListAssets per scrape.
type prometheusSnapshotAdapter struct {
	telemetryStore persistence.TelemetryLabeledSnapshotStore
	hubMetricStore persistence.TelemetryHubMetricStore
	assetStore     persistence.AssetStore
}

const prometheusStoreSnapshotTimeout = 2 * time.Second

// scrape fetches the asset list once and returns both snapshots and metadata
// in a single pass, eliminating the double table scan that occurred when
// LatestSnapshots and AssetMetadata each called ListAssets independently.
func (a *prometheusSnapshotAdapter) scrape() (map[string][]promexport.LabeledMetric, map[string]promexport.AssetMeta) {
	assetList, err := a.assetStore.ListAssets()
	if err != nil {
		return nil, nil
	}
	if len(assetList) > telemetry.MaxPrometheusSnapshotAssets {
		return nil, nil
	}

	assetIDs := make([]string, 0, len(assetList))
	for _, asset := range assetList {
		assetIDs = append(assetIDs, asset.ID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), prometheusStoreSnapshotTimeout)
	defer cancel()
	snapMap, err := a.telemetryStore.LatestLabeledMetricSnapshots(
		ctx,
		assetIDs,
		time.Now().UTC(),
		telemetry.MaxPrometheusAssetMetricSeries,
	)
	if err != nil {
		return nil, nil
	}

	snapshots := make(map[string][]promexport.LabeledMetric, len(snapMap))
	for assetID, samples := range snapMap {
		labeled := make([]promexport.LabeledMetric, 0, len(samples))
		for _, sample := range samples {
			labeled = append(labeled, promexport.LabeledMetric{
				Metric:      sample.Metric,
				Value:       sample.Value,
				Labels:      sample.Labels,
				CollectedAt: sample.CollectedAt,
			})
		}
		snapshots[assetID] = labeled
	}

	metas := make(map[string]promexport.AssetMeta, len(assetList))
	for _, asset := range assetList {
		meta := promexport.AssetMeta{
			Name:     asset.Name,
			Type:     asset.Type,
			Platform: asset.Platform,
		}
		// Enrich Docker container metadata for PromQL joins.
		if asset.Type == "docker-container" {
			meta.DockerHost = asset.Metadata["agent_id"]
			meta.DockerImage = asset.Metadata["image"]
			meta.DockerStack = asset.Metadata["stack"]
		}
		metas[asset.ID] = meta
	}

	return snapshots, metas
}

func (a *prometheusSnapshotAdapter) hubScrape() map[string][]promexport.LabeledMetric {
	if a.hubMetricStore == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), prometheusStoreSnapshotTimeout)
	defer cancel()
	hubSnapshots, err := a.hubMetricStore.HubMetricSnapshots(ctx, time.Now().UTC(), telemetry.MaxHubMetricSnapshotSeries)
	if err != nil {
		return nil
	}
	out := make(map[string][]promexport.LabeledMetric, len(hubSnapshots))
	for scope, samples := range hubSnapshots {
		for _, sample := range samples {
			out[scope] = append(out[scope], promexport.LabeledMetric{
				Metric:      sample.Metric,
				Value:       sample.Value,
				Labels:      sample.Labels,
				CollectedAt: sample.CollectedAt,
			})
		}
	}
	return out
}

// cachedSnapshotAdapter wraps prometheusSnapshotAdapter and caches the asset
// list for cacheTTL. Both LatestSnapshots and AssetMetadata use the same cached
// result from a single scrape(), so the asset list is fetched at most once per
// scrape interval rather than twice.
type cachedSnapshotAdapter struct {
	inner    *prometheusSnapshotAdapter
	cacheTTL time.Duration

	mu              sync.Mutex
	cachedSnapshots map[string][]promexport.LabeledMetric
	cachedMetas     map[string]promexport.AssetMeta
	cachedHub       map[string][]promexport.LabeledMetric
	cachedAt        time.Time
}

// refresh fetches a fresh scrape if the cache has expired, then updates the
// internal cache. Must be called with mu held.
func (c *cachedSnapshotAdapter) refresh() {
	now := time.Now()
	if !c.cachedAt.IsZero() && now.Sub(c.cachedAt) < c.cacheTTL {
		return
	}
	snaps, metas := c.inner.scrape()
	c.cachedSnapshots = snaps
	c.cachedMetas = metas
	c.cachedHub = c.inner.hubScrape()
	c.cachedAt = now
}

func (c *cachedSnapshotAdapter) LatestSnapshots() map[string][]promexport.LabeledMetric {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh()
	return c.cachedSnapshots
}

func (c *cachedSnapshotAdapter) AssetMetadata() map[string]promexport.AssetMeta {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh()
	return c.cachedMetas
}

func (c *cachedSnapshotAdapter) HubSnapshots() map[string][]promexport.LabeledMetric {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh()
	return c.cachedHub
}

const (
	agentTelemetryRequestTimeout = 3 * time.Second
	agentTelemetryConcurrency    = 8
	networkCounterMaxAge         = 10 * time.Minute
)

type processMetricsConfigResolver func() (enabled bool, topN int)

// agentTelemetryAdapter backs the process, interface, and mount bridges using
// the existing authenticated agent request channels. Each method has its own
// single-flight key and rotating fleet cursor so concurrent callers share work
// and fleets larger than one sweep cap are covered over subsequent intervals.
type agentTelemetryAdapter struct {
	agentMgr       *agentmgr.AgentManager
	processBridges *sync.Map
	networkBridges *sync.Map
	diskBridges    *sync.Map
	resolveProcess processMetricsConfigResolver
	requestTimeout time.Duration
	requestSem     chan struct{}

	flight singleflight.Group

	cursorMu sync.Mutex
	cursors  map[string]int

	networkMu       sync.Mutex
	networkCounters map[string]networkCounterSnapshot
}

type networkCounterSnapshot struct {
	rxBytes uint64
	txBytes uint64
	at      time.Time
}

func newAgentTelemetryAdapter(srv *apiServer) *agentTelemetryAdapter {
	adapter := &agentTelemetryAdapter{
		agentMgr:        srv.agentMgr,
		processBridges:  &srv.processBridges,
		networkBridges:  &srv.networkBridges,
		diskBridges:     &srv.diskBridges,
		requestTimeout:  agentTelemetryRequestTimeout,
		requestSem:      make(chan struct{}, agentTelemetryConcurrency),
		cursors:         make(map[string]int),
		networkCounters: make(map[string]networkCounterSnapshot),
	}
	adapter.resolveProcess = func() (bool, int) {
		if srv.runtimeStore == nil {
			return false, 0
		}
		values, _, err := shared.ResolveRuntimeSettingEffectiveValues(srv.runtimeStore, srv.secretsManager)
		if err != nil {
			return false, 0
		}
		enabled, err := strconv.ParseBool(strings.TrimSpace(values[runtimesettings.KeyProcessMetricsEnabled]))
		if err != nil || !enabled {
			return false, 0
		}
		topN, err := strconv.Atoi(strings.TrimSpace(values[runtimesettings.KeyProcessMetricsTopN]))
		if err != nil || topN < 1 {
			return false, 0
		}
		if topN > telemetry.MaxBridgeProcessesPerAsset {
			topN = telemetry.MaxBridgeProcessesPerAsset
		}
		return true, topN
	}
	return adapter
}

func (a *agentTelemetryAdapter) timeout() time.Duration {
	if a.requestTimeout <= 0 {
		return agentTelemetryRequestTimeout
	}
	return a.requestTimeout
}

func (a *agentTelemetryAdapter) connectedAssetBatch(kind string) []string {
	if a == nil || a.agentMgr == nil {
		return nil
	}
	ids := a.agentMgr.ConnectedAssets()
	sort.Strings(ids)
	if len(ids) <= telemetry.MaxBridgeAgentAssets {
		return ids
	}
	a.cursorMu.Lock()
	start := a.cursors[kind] % len(ids)
	a.cursors[kind] = (start + telemetry.MaxBridgeAgentAssets) % len(ids)
	a.cursorMu.Unlock()
	out := make([]string, 0, telemetry.MaxBridgeAgentAssets)
	for offset := 0; offset < telemetry.MaxBridgeAgentAssets; offset++ {
		out = append(out, ids[(start+offset)%len(ids)])
	}
	return out
}

func collectAgentAssets[T any](assetIDs []string, sem chan struct{}, collect func(string) []T) []T {
	if len(assetIDs) == 0 {
		return nil
	}
	if cap(sem) == 0 {
		sem = make(chan struct{}, agentTelemetryConcurrency)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var out []T
	for _, assetID := range assetIDs {
		assetID := assetID
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			entries := func() (entries []T) {
				defer func() {
					<-sem
					if recover() != nil {
						entries = nil
					}
				}()
				return collect(assetID)
			}()
			if len(entries) == 0 {
				return
			}
			mu.Lock()
			out = append(out, entries...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

func (a *agentTelemetryAdapter) AllProcessMetrics() []bridge.ProcessMetricEntry {
	if a == nil || a.resolveProcess == nil {
		return nil
	}
	enabled, topN := a.resolveProcess()
	if !enabled || topN < 1 {
		return nil
	}
	value, _, _ := a.flight.Do("process", func() (any, error) {
		entries := collectAgentAssets(a.connectedAssetBatch("process"), a.requestSem, func(assetID string) []bridge.ProcessMetricEntry {
			return a.collectProcesses(assetID, topN)
		})
		return entries, nil
	})
	entries, _ := value.([]bridge.ProcessMetricEntry)
	return entries
}

func (a *agentTelemetryAdapter) collectProcesses(assetID string, topN int) []bridge.ProcessMetricEntry {
	conn, ok := a.agentMgr.Get(assetID)
	if !ok || conn == nil || a.processBridges == nil {
		return nil
	}
	requestID := generateRequestID()
	waiter := &processBridge{Ch: make(chan agentmgr.Message, 1), ExpectedAssetID: assetID}
	a.processBridges.Store(requestID, waiter)
	defer a.processBridges.Delete(requestID)
	payload, _ := json.Marshal(agentmgr.ProcessListData{RequestID: requestID, SortBy: "cpu", Limit: topN})
	if err := conn.Send(agentmgr.Message{Type: agentmgr.MsgProcessList, ID: requestID, Data: payload}); err != nil {
		return nil
	}
	timer := time.NewTimer(a.timeout())
	defer timer.Stop()
	var response agentmgr.ProcessListedData
	select {
	case msg := <-waiter.Ch:
		if json.Unmarshal(msg.Data, &response) != nil || strings.TrimSpace(response.Error) != "" {
			return nil
		}
	case <-timer.C:
		return nil
	}
	maxRawProcesses := telemetry.MaxBridgeProcessesPerAsset * 10
	if len(response.Processes) > maxRawProcesses {
		response.Processes = response.Processes[:maxRawProcesses]
	}
	sort.Slice(response.Processes, func(i, j int) bool {
		if response.Processes[i].CPUPct != response.Processes[j].CPUPct {
			return response.Processes[i].CPUPct > response.Processes[j].CPUPct
		}
		return response.Processes[i].PID < response.Processes[j].PID
	})
	out := make([]bridge.ProcessMetricEntry, 0, min(len(response.Processes), topN))
	seenPIDs := make(map[int]struct{}, topN)
	for _, process := range response.Processes {
		if len(out) >= topN {
			break
		}
		name, validName := boundedMetricLabel(process.Name, 256)
		if process.PID <= 0 || !validName || !finiteRange(process.CPUPct, 0, 10000) ||
			!finiteRange(process.MemPct, 0, 100) || process.MemRSS < 0 {
			continue
		}
		if _, duplicate := seenPIDs[process.PID]; duplicate {
			continue
		}
		seenPIDs[process.PID] = struct{}{}
		out = append(out, bridge.ProcessMetricEntry{
			AssetID: assetID, CPUPercent: process.CPUPct, MemPercent: process.MemPct, MemRSS: float64(process.MemRSS),
			Labels: map[string]string{"process_name": name, "process_pid": strconv.Itoa(process.PID)},
		})
	}
	return out
}

func (a *agentTelemetryAdapter) AllNetworkInterfaces() []bridge.NetworkInterfaceEntry {
	if a == nil {
		return nil
	}
	value, _, _ := a.flight.Do("network", func() (any, error) {
		entries := collectAgentAssets(a.connectedAssetBatch("network"), a.requestSem, a.collectNetworkInterfaces)
		return entries, nil
	})
	entries, _ := value.([]bridge.NetworkInterfaceEntry)
	return entries
}

func (a *agentTelemetryAdapter) collectNetworkInterfaces(assetID string) []bridge.NetworkInterfaceEntry {
	conn, ok := a.agentMgr.Get(assetID)
	if !ok || conn == nil || a.networkBridges == nil {
		return nil
	}
	requestID := generateRequestID()
	waiter := &networkBridge{Ch: make(chan agentmgr.Message, 1), ExpectedAssetID: assetID}
	a.networkBridges.Store(requestID, waiter)
	defer a.networkBridges.Delete(requestID)
	payload, _ := json.Marshal(agentmgr.NetworkListData{RequestID: requestID})
	if err := conn.Send(agentmgr.Message{Type: agentmgr.MsgNetworkList, ID: requestID, Data: payload}); err != nil {
		return nil
	}
	timer := time.NewTimer(a.timeout())
	defer timer.Stop()
	var response agentmgr.NetworkListedData
	select {
	case msg := <-waiter.Ch:
		if json.Unmarshal(msg.Data, &response) != nil || strings.TrimSpace(response.Error) != "" {
			return nil
		}
	case <-timer.C:
		return nil
	}
	maxRawInterfaces := telemetry.MaxBridgeInterfacesPerAsset * 10
	if len(response.Interfaces) > maxRawInterfaces {
		response.Interfaces = response.Interfaces[:maxRawInterfaces]
	}
	sort.Slice(response.Interfaces, func(i, j int) bool { return response.Interfaces[i].Name < response.Interfaces[j].Name })
	now := time.Now().UTC()
	out := make([]bridge.NetworkInterfaceEntry, 0, telemetry.MaxBridgeInterfacesPerAsset)
	a.networkMu.Lock()
	for key, previous := range a.networkCounters {
		if now.Sub(previous.at) > networkCounterMaxAge {
			delete(a.networkCounters, key)
		}
	}
	seenInterfaces := make(map[string]struct{}, telemetry.MaxBridgeInterfacesPerAsset)
	for _, iface := range response.Interfaces {
		name, validName := boundedMetricLabel(iface.Name, 128)
		if !validName {
			continue
		}
		if _, duplicate := seenInterfaces[name]; duplicate {
			continue
		}
		if len(seenInterfaces) >= telemetry.MaxBridgeInterfacesPerAsset {
			break
		}
		seenInterfaces[name] = struct{}{}
		key := assetID + "\x00" + name
		previous, hasPrevious := a.networkCounters[key]
		a.networkCounters[key] = networkCounterSnapshot{rxBytes: iface.RXBytes, txBytes: iface.TXBytes, at: now}
		if !hasPrevious || !now.After(previous.at) || iface.RXBytes < previous.rxBytes || iface.TXBytes < previous.txBytes {
			continue
		}
		seconds := now.Sub(previous.at).Seconds()
		rxRate := float64(iface.RXBytes-previous.rxBytes) / seconds
		txRate := float64(iface.TXBytes-previous.txBytes) / seconds
		if !finiteNonNegative(rxRate) || !finiteNonNegative(txRate) {
			continue
		}
		out = append(out, bridge.NetworkInterfaceEntry{
			AssetID: assetID, RXBytes: rxRate, TXBytes: txRate,
			RXPackets: float64(iface.RXPackets), TXPackets: float64(iface.TXPackets),
			Labels: map[string]string{"interface": name},
		})
	}
	a.networkMu.Unlock()
	return out
}

func (a *agentTelemetryAdapter) AllDiskMounts() []bridge.DiskMountEntry {
	if a == nil {
		return nil
	}
	value, _, _ := a.flight.Do("disk", func() (any, error) {
		entries := collectAgentAssets(a.connectedAssetBatch("disk"), a.requestSem, a.collectDiskMounts)
		return entries, nil
	})
	entries, _ := value.([]bridge.DiskMountEntry)
	return entries
}

func (a *agentTelemetryAdapter) collectDiskMounts(assetID string) []bridge.DiskMountEntry {
	conn, ok := a.agentMgr.Get(assetID)
	if !ok || conn == nil || a.diskBridges == nil {
		return nil
	}
	requestID := generateRequestID()
	waiter := &diskBridge{Ch: make(chan agentmgr.Message, 1), ExpectedAssetID: assetID}
	a.diskBridges.Store(requestID, waiter)
	defer a.diskBridges.Delete(requestID)
	payload, _ := json.Marshal(agentmgr.DiskListData{RequestID: requestID})
	if err := conn.Send(agentmgr.Message{Type: agentmgr.MsgDiskList, ID: requestID, Data: payload}); err != nil {
		return nil
	}
	timer := time.NewTimer(a.timeout())
	defer timer.Stop()
	var response agentmgr.DiskListedData
	select {
	case msg := <-waiter.Ch:
		if json.Unmarshal(msg.Data, &response) != nil || strings.TrimSpace(response.Error) != "" {
			return nil
		}
	case <-timer.C:
		return nil
	}
	maxRawMounts := telemetry.MaxBridgeMountsPerAsset * 10
	if len(response.Mounts) > maxRawMounts {
		response.Mounts = response.Mounts[:maxRawMounts]
	}
	sort.Slice(response.Mounts, func(i, j int) bool { return response.Mounts[i].MountPoint < response.Mounts[j].MountPoint })
	out := make([]bridge.DiskMountEntry, 0, telemetry.MaxBridgeMountsPerAsset)
	seenMounts := make(map[string]struct{}, telemetry.MaxBridgeMountsPerAsset)
	for _, mount := range response.Mounts {
		mountPoint, validMount := boundedMetricLabel(mount.MountPoint, 512)
		if !validMount || mount.Used > mount.Total || mount.Available > mount.Total ||
			mount.Used > mount.Total-mount.Available || !finiteRange(mount.UsePct, 0, 100) {
			continue
		}
		if _, duplicate := seenMounts[mountPoint]; duplicate {
			continue
		}
		if len(seenMounts) >= telemetry.MaxBridgeMountsPerAsset {
			break
		}
		seenMounts[mountPoint] = struct{}{}
		out = append(out, bridge.DiskMountEntry{
			AssetID: assetID, Total: float64(mount.Total), Used: float64(mount.Used),
			Available: float64(mount.Available), UsePct: mount.UsePct,
			Labels: map[string]string{"mount_point": mountPoint},
		})
	}
	return out
}

type serviceHealthMetricsAdapter struct {
	coordinator *webservice.Coordinator
}

func (a *serviceHealthMetricsAdapter) AllServiceHealth() []bridge.ServiceHealthEntry {
	if a == nil || a.coordinator == nil {
		return nil
	}
	services := a.coordinator.ListAll()
	if len(services) > telemetry.MaxBridgeServiceSeries {
		services = services[:telemetry.MaxBridgeServiceSeries]
	}
	a.coordinator.AttachHealthSummaries(services)
	out := make([]bridge.ServiceHealthEntry, 0, len(services))
	for _, service := range services {
		if strings.EqualFold(strings.TrimSpace(service.Metadata["hidden"]), "true") {
			continue
		}
		assetID, validAsset := boundedMetricLabel(service.HostAssetID, telemetry.MaxMetricIdentityBytes)
		serviceID, validID := boundedMetricLabel(service.ID, 256)
		serviceName, validName := boundedMetricLabel(service.Name, 256)
		if !validAsset || !validID || !validName || service.ResponseMs < 0 {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(service.Status))
		statusValue := 0.0
		switch status {
		case "up", "online", "healthy":
			statusValue = 1
		case "down", "offline", "unhealthy":
		default:
			continue
		}
		entry := bridge.ServiceHealthEntry{
			AssetID: assetID, Status: statusValue,
			HasResponse: service.ResponseMs > 0 || statusValue == 1,
			ResponseMs:  float64(service.ResponseMs),
			Labels:      map[string]string{"service_name": serviceName, "target": serviceID},
		}
		if service.Health != nil && finiteRange(service.Health.UptimePercent, 0, 100) {
			entry.HasUptime = true
			entry.UptimePercent = service.Health.UptimePercent
		}
		out = append(out, entry)
	}
	return out
}

type syntheticMetricsAdapter struct {
	snapshotStore     persistence.SyntheticMetricSnapshotStore
	mu                sync.Mutex
	lastResultByCheck map[string]string
}

func (a *syntheticMetricsAdapter) AllSyntheticCheckMetrics() []bridge.SyntheticCheckEntry {
	if a == nil || a.snapshotStore == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), prometheusStoreSnapshotTimeout)
	defer cancel()
	snapshots, err := a.snapshotStore.LatestSyntheticMetricSnapshots(ctx, telemetry.MaxBridgeSyntheticSeries)
	if err != nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	nextResults := make(map[string]string, len(snapshots))
	out := make([]bridge.SyntheticCheckEntry, 0, len(snapshots))
	for _, snapshot := range snapshots {
		checkID, validID := boundedMetricLabel(snapshot.CheckID, 256)
		checkName, validName := boundedMetricLabel(snapshot.CheckName, 256)
		checkType, validType := boundedMetricLabel(snapshot.CheckType, 64)
		resultID, validResult := boundedMetricLabel(snapshot.ResultID, 256)
		if !validID || !validName || !validType || !validResult || snapshot.CheckedAt.IsZero() {
			continue
		}
		nextResults[checkID] = resultID
		if a.lastResultByCheck[checkID] == resultID {
			continue
		}
		statusValue := 0.0
		switch synthetic.NormalizeResultStatus(snapshot.Status) {
		case synthetic.ResultStatusOK:
			statusValue = 1
		case synthetic.ResultStatusFail:
		case synthetic.ResultStatusTimeout:
			statusValue = 2
		default:
			continue
		}
		entry := bridge.SyntheticCheckEntry{
			Status: statusValue, CollectedAt: snapshot.CheckedAt,
			Labels: map[string]string{"check_id": checkID, "check_name": checkName, "check_type": checkType},
		}
		if snapshot.LatencyMS != nil && *snapshot.LatencyMS >= 0 {
			entry.HasLatency = true
			entry.LatencyMs = float64(*snapshot.LatencyMS)
		}
		out = append(out, entry)
	}
	a.lastResultByCheck = nextResults
	return out
}

func boundedMetricLabel(raw string, maxBytes int) (string, bool) {
	value := strings.TrimSpace(raw)
	return value, value != "" && len(value) <= maxBytes && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func finiteRange(value, min, max float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= min && value <= max
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

// ---- dockerStatsAdapter implements bridge.DockerStatsSource ----

type dockerStatsAdapter struct {
	coord *docker.Coordinator
}

func newDockerStatsAdapter(coord *docker.Coordinator) *dockerStatsAdapter {
	return &dockerStatsAdapter{coord: coord}
}

func (a *dockerStatsAdapter) AllContainerMetrics() []bridge.ContainerMetricEntry {
	if a == nil || a.coord == nil {
		return nil
	}
	hosts := a.coord.ListHosts()
	if len(hosts) == 0 {
		return nil
	}

	var out []bridge.ContainerMetricEntry
	for _, host := range hosts {
		normalizedAgentID := normalizeAgentID(host.AgentID)
		dockerHost := host.Engine.Hostname
		if dockerHost == "" {
			dockerHost = normalizedAgentID
		}

		for _, ct := range host.Containers {
			if len(out) >= telemetry.MaxBridgeDockerContainerSeries {
				return out
			}
			ctShort := ct.ID
			if len(ctShort) > 12 {
				ctShort = ctShort[:12]
			}
			assetID := fmt.Sprintf("docker-ct-%s-%s", normalizedAgentID, ctShort)

			stats, hasStats := host.Stats[ct.ID]
			if !hasStats {
				continue
			}

			labels := map[string]string{
				"docker_host":  dockerHost,
				"docker_image": ct.Image,
			}
			if ct.StackName != "" {
				labels["docker_stack"] = ct.StackName
			}

			out = append(out, bridge.ContainerMetricEntry{
				AssetID:    assetID,
				CPU:        stats.CPUPercent,
				Memory:     stats.MemoryPercent,
				NetRX:      float64(stats.NetRXBytes),
				NetTX:      float64(stats.NetTXBytes),
				BlockRead:  float64(stats.BlockReadBytes),
				BlockWrite: float64(stats.BlockWriteBytes),
				PIDs:       float64(stats.PIDs),
				Labels:     labels,
			})
		}
	}
	return out
}

// ---- alertStateAdapter implements bridge.AlertStateSource ----

type alertStateAdapter struct {
	metricsSnapshotStore persistence.AlertMetricsSnapshotStore
}

func (a *alertStateAdapter) AllAlertStateMetrics() []bridge.AlertStateEntry {
	entries, _ := a.AllAlertMetricsSnapshot()
	return entries
}

// AllAlertRuleEvalMetrics retains the optional legacy bridge interface. The
// bridge prefers AllAlertMetricsSnapshot so normal collection invokes the
// bounded persistence query only once.
func (a *alertStateAdapter) AllAlertRuleEvalMetrics() []bridge.AlertRuleEvalEntry {
	_, evaluations := a.AllAlertMetricsSnapshot()
	return evaluations
}

// AllAlertMetricsSnapshot implements bridge.AlertStateSnapshotSource. Exact
// active-rule and firing-instance counts share one bounded query with at most
// 500 latest per-rule evaluation series.
func (a *alertStateAdapter) AllAlertMetricsSnapshot() ([]bridge.AlertStateEntry, []bridge.AlertRuleEvalEntry) {
	if a == nil || a.metricsSnapshotStore == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), prometheusStoreSnapshotTimeout)
	defer cancel()
	snapshot, err := a.metricsSnapshotStore.AlertMetricsSnapshot(ctx, telemetry.MaxAlertRuleMetricSeries)
	if err != nil {
		return nil, nil
	}
	entries := []bridge.AlertStateEntry{{
		FiringCount: float64(snapshot.FiringInstanceCount),
		RulesCount:  float64(snapshot.ActiveRuleCount),
	}}
	evaluations := make([]bridge.AlertRuleEvalEntry, 0, len(snapshot.RuleEvaluations))
	for _, evaluation := range snapshot.RuleEvaluations {
		evaluations = append(evaluations, bridge.AlertRuleEvalEntry{
			RuleID:     evaluation.RuleID,
			RuleName:   evaluation.RuleName,
			DurationMS: float64(evaluation.DurationMS),
		})
	}
	return entries, evaluations
}

// ---- agentPresenceAdapter implements bridge.AgentPresenceSource ----

type agentPresenceAdapter struct {
	agentMgr   *agentmgr.AgentManager
	assetStore persistence.AssetStore
}

func (a *agentPresenceAdapter) AllAgentPresenceMetrics() []bridge.AgentPresenceEntry {
	if a == nil || a.agentMgr == nil || a.assetStore == nil {
		return nil
	}
	assets, err := a.assetStore.ListAssets()
	if err != nil {
		return nil
	}
	if len(assets) > telemetry.MaxBridgeAgentPresenceSeries {
		return nil
	}
	now := time.Now().UTC()
	var out []bridge.AgentPresenceEntry
	for _, asset := range assets {
		if asset.Source != "agent" {
			continue
		}
		if len(out) >= telemetry.MaxBridgeAgentPresenceSeries {
			break
		}
		connected := 0.0
		if a.agentMgr.IsConnected(asset.ID) {
			connected = 1.0
		}
		ageSec := now.Sub(asset.LastSeenAt).Seconds()
		if ageSec < 0 {
			ageSec = 0
		}
		out = append(out, bridge.AgentPresenceEntry{
			AssetID:             asset.ID,
			Connected:           connected,
			LastHeartbeatAgeSec: ageSec,
			Labels:              map[string]string{"asset_name": asset.Name, "platform": asset.Platform},
		})
	}
	return out
}

// ---- siteReliabilityAdapter implements bridge.SiteReliabilitySource ----

type siteReliabilityAdapter struct {
	snapshotStore persistence.ReliabilityMetricSnapshotStore
}

func (a *siteReliabilityAdapter) AllSiteReliabilityMetrics() []bridge.SiteReliabilityEntry {
	if a == nil || a.snapshotStore == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), prometheusStoreSnapshotTimeout)
	defer cancel()
	snapshots, err := a.snapshotStore.LatestReliabilityMetricSnapshots(ctx, time.Now().UTC(), telemetry.MaxSiteReliabilityMetricSeries)
	if err != nil {
		return nil
	}
	out := make([]bridge.SiteReliabilityEntry, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, bridge.SiteReliabilityEntry{
			Score:  float64(snapshot.Score),
			Labels: map[string]string{"site_id": snapshot.GroupID, "site_name": snapshot.GroupName},
		})
	}
	return out
}

// normalizeAgentID converts an agent asset ID to the normalized form used in
// docker asset IDs — lowercase, spaces and dots replaced with dashes. This
// must match the normalizeID function in the docker coordinator package.
func normalizeAgentID(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c == ' ' || c == '.' {
			c = '-'
		}
		out = append(out, c)
	}
	return string(out)
}
