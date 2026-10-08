package main

import (
	"context"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/telemetry/bridge"
	"github.com/labtether/labtether/internal/telemetry/promexport"
	"sync"
	"time"
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
