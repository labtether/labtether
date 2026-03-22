package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/telemetry/bridge"
	"github.com/labtether/labtether/internal/telemetry/promexport"
)

// initMetricsExport wires the bridge registry and Prometheus scrape adapter
// into the server. It must be called before startRuntimeLoops so that the
// registry is available when Run is invoked.
//
// Bridges are registered here as source adapters are available. The Docker
// stats bridge is the initial working example; others are wired incrementally
// once their coordinator iterators are in place.
func initMetricsExport(srv *apiServer, pgStore *persistence.PostgresStore) {
	reg := bridge.NewRegistry()
	srv.bridgeRegistry = reg

	// Docker stats bridge — collects per-container CPU, memory, network, block I/O.
	if srv.dockerCoordinator != nil {
		reg.Register(bridge.NewDockerStatsBridge(newDockerStatsAdapter(srv.dockerCoordinator)))
	}

	// Alert state bridge — counts firing instances and active rules.
	if srv.alertStore != nil && srv.alertInstanceStore != nil {
		reg.Register(bridge.NewAlertStateBridge(&alertStateAdapter{
			alertStore:         srv.alertStore,
			alertInstanceStore: srv.alertInstanceStore,
		}))
	}

	// Agent presence bridge — per-agent connection state and heartbeat age.
	if srv.agentMgr != nil && srv.assetStore != nil {
		reg.Register(bridge.NewAgentPresenceBridge(&agentPresenceAdapter{
			agentMgr:   srv.agentMgr,
			assetStore: srv.assetStore,
		}))
	}

	// Site reliability bridge — per-group reliability scores.
	if srv.groupStore != nil {
		reg.Register(bridge.NewSiteReliabilityBridge(&siteReliabilityAdapter{
			groupStore:       srv.groupStore,
			reliabilityStore: pgStore,
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
		return srv.telemetryStore.AppendSamples(ctx, samples)
	})
}

// newPrometheusSnapshotSource builds the SnapshotSource backed by the real
// telemetry and asset stores. Called from handlePrometheusMetrics when the
// server is fully initialised.
func newPrometheusSnapshotSource(srv *apiServer) promexport.SnapshotSource {
	dynStore, ok := srv.telemetryStore.(persistence.TelemetryDynamicStore)
	if !ok || srv.assetStore == nil {
		return promexport.NoopSnapshotSource{}
	}
	return &cachedSnapshotAdapter{
		inner: &prometheusSnapshotAdapter{
			telemetryStore: dynStore,
			assetStore:     srv.assetStore,
		},
		cacheTTL: 5 * time.Second,
	}
}

// prometheusSnapshotAdapter fetches asset data directly from the stores. It is
// wrapped by cachedSnapshotAdapter to avoid the double ListAssets per scrape.
type prometheusSnapshotAdapter struct {
	telemetryStore persistence.TelemetryDynamicStore
	assetStore     persistence.AssetStore
}

// scrape fetches the asset list once and returns both snapshots and metadata
// in a single pass, eliminating the double table scan that occurred when
// LatestSnapshots and AssetMetadata each called ListAssets independently.
func (a *prometheusSnapshotAdapter) scrape() (map[string][]promexport.LabeledMetric, map[string]promexport.AssetMeta) {
	assetList, err := a.assetStore.ListAssets()
	if err != nil || len(assetList) == 0 {
		return nil, nil
	}

	assetIDs := make([]string, 0, len(assetList))
	for _, asset := range assetList {
		assetIDs = append(assetIDs, asset.ID)
	}

	snapMap, err := a.telemetryStore.DynamicSnapshotMany(assetIDs, time.Now().UTC())
	if err != nil {
		snapMap = nil
	}

	snapshots := make(map[string][]promexport.LabeledMetric, len(snapMap))
	for assetID, snap := range snapMap {
		if len(snap.Metrics) == 0 {
			continue
		}
		labeled := make([]promexport.LabeledMetric, 0, len(snap.Metrics))
		for metric, value := range snap.Metrics {
			labeled = append(labeled, promexport.LabeledMetric{
				Metric: metric,
				Value:  value,
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

// ---- dockerStatsAdapter implements bridge.DockerStatsSource ----

type dockerStatsAdapter struct {
	coord *docker.Coordinator
}

func newDockerStatsAdapter(coord *docker.Coordinator) *dockerStatsAdapter {
	return &dockerStatsAdapter{coord: coord}
}

func (a *dockerStatsAdapter) AllContainerMetrics() []bridge.ContainerMetricEntry {
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
	alertStore         persistence.AlertStore
	alertInstanceStore persistence.AlertInstanceStore
}

func (a *alertStateAdapter) AllAlertStateMetrics() []bridge.AlertStateEntry {
	rules, err := a.alertStore.ListAlertRules(persistence.AlertRuleFilter{Status: "active", Limit: 10000})
	if err != nil {
		rules = nil
	}
	instances, err := a.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{Status: "firing", Limit: 10000})
	if err != nil {
		instances = nil
	}
	return []bridge.AlertStateEntry{{
		FiringCount: float64(len(instances)),
		RulesCount:  float64(len(rules)),
	}}
}

// AllAlertRuleEvalMetrics implements bridge.AlertRuleEvalSource — returns the
// latest evaluation duration for each active alert rule.
func (a *alertStateAdapter) AllAlertRuleEvalMetrics() []bridge.AlertRuleEvalEntry {
	rules, err := a.alertStore.ListAlertRules(persistence.AlertRuleFilter{Status: "active", Limit: 10000})
	if err != nil {
		return nil
	}
	var out []bridge.AlertRuleEvalEntry
	for _, rule := range rules {
		evals, err := a.alertStore.ListAlertEvaluations(rule.ID, 1)
		if err != nil || len(evals) == 0 {
			continue
		}
		out = append(out, bridge.AlertRuleEvalEntry{
			RuleName:   rule.Name,
			DurationMS: float64(evals[0].DurationMS),
		})
	}
	return out
}

// ---- agentPresenceAdapter implements bridge.AgentPresenceSource ----

type agentPresenceAdapter struct {
	agentMgr   *agentmgr.AgentManager
	assetStore persistence.AssetStore
}

func (a *agentPresenceAdapter) AllAgentPresenceMetrics() []bridge.AgentPresenceEntry {
	assets, err := a.assetStore.ListAssets()
	if err != nil {
		return nil
	}
	now := time.Now().UTC()
	var out []bridge.AgentPresenceEntry
	for _, asset := range assets {
		if asset.Source != "agent" {
			continue
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
	groupStore         persistence.GroupStore
	reliabilityStore   persistence.ReliabilityHistoryStore
}

func (a *siteReliabilityAdapter) AllSiteReliabilityMetrics() []bridge.SiteReliabilityEntry {
	if a.reliabilityStore == nil {
		return nil
	}
	groups, err := a.groupStore.ListGroups()
	if err != nil {
		return nil
	}
	var out []bridge.SiteReliabilityEntry
	for _, g := range groups {
		records, err := a.reliabilityStore.ListReliabilityHistory(g.ID, 1)
		if err != nil || len(records) == 0 {
			continue
		}
		out = append(out, bridge.SiteReliabilityEntry{
			Score:  float64(records[0].Score),
			Labels: map[string]string{"site_id": g.ID, "site_name": g.Name},
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
