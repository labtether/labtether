package main

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectors/webservice"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/synthetic"
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/telemetry/bridge"
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

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
