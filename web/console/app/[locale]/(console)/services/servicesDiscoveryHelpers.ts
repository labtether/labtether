import type { WebServiceDiscoveryHostStat } from "../../../hooks/useWebServices";

export type DiscoverySourceSummary = {
  key: string;
  label: string;
  enabledHosts: number;
  hostsReported: number;
  servicesFound: number;
  totalDurationMs: number;
};

export type DiscoveryOverview = {
  hostCount: number;
  latestCollectedAt: string;
  averageCycleDurationMs: number;
  averageDiscoveredServices: number;
  sources: DiscoverySourceSummary[];
  hostRows: Array<{
    hostAssetID: string;
    hostLabel: string;
    collectedAt: string;
    cycleDurationMs: number;
    totalServices: number;
  }>;
};

export const discoverySourceOrder = ["docker", "proxy", "local_scan", "lan_scan"] as const;

export function formatDiscoveryCollectedAt(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

export function discoverySourceLabel(source: string): string {
  switch (source) {
    case "docker":
      return "Docker";
    case "proxy":
      return "Proxy APIs";
    case "local_scan":
      return "Local Port Scan";
    case "lan_scan":
      return "LAN Scan";
    default:
      return source;
  }
}

export function buildDiscoveryOverview(
  discoveryStats: WebServiceDiscoveryHostStat[],
  assetNameMap: Map<string, string>
): DiscoveryOverview {
  const cycleHosts = discoveryStats.filter((entry) => entry.discovery != null);
  const sourceSummaries = new Map<string, DiscoverySourceSummary>();
  for (const key of discoverySourceOrder) {
    sourceSummaries.set(key, {
      key,
      label: discoverySourceLabel(key),
      enabledHosts: 0,
      hostsReported: 0,
      servicesFound: 0,
      totalDurationMs: 0,
    });
  }

  let latestCollectedAt = "";
  let totalCycleDuration = 0;
  let totalDiscoveredServices = 0;
  const hostRows: DiscoveryOverview["hostRows"] = [];

  for (const entry of cycleHosts) {
    const stats = entry.discovery;
    if (stats.collected_at > latestCollectedAt) {
      latestCollectedAt = stats.collected_at;
    }
    totalCycleDuration += Number.isFinite(stats.cycle_duration_ms) ? stats.cycle_duration_ms : 0;
    totalDiscoveredServices += Number.isFinite(stats.total_services) ? stats.total_services : 0;
    hostRows.push({
      hostAssetID: entry.host_asset_id,
      hostLabel: assetNameMap.get(entry.host_asset_id) ?? entry.host_asset_id,
      collectedAt: stats.collected_at,
      cycleDurationMs: stats.cycle_duration_ms,
      totalServices: stats.total_services,
    });

    const sourceStats = stats.sources ?? {};
    for (const key of discoverySourceOrder) {
      const summary = sourceSummaries.get(key);
      if (!summary) {
        continue;
      }
      const source = sourceStats[key];
      if (!source) {
        continue;
      }
      summary.hostsReported += 1;
      if (source.enabled) {
        summary.enabledHosts += 1;
      }
      summary.servicesFound += Number.isFinite(source.services_found) ? source.services_found : 0;
      summary.totalDurationMs += Number.isFinite(source.duration_ms) ? source.duration_ms : 0;
    }
  }

  hostRows.sort((left, right) => right.collectedAt.localeCompare(left.collectedAt));
  const sources = Array.from(sourceSummaries.values());
  const hostCount = cycleHosts.length;
  return {
    hostCount,
    latestCollectedAt,
    averageCycleDurationMs: hostCount > 0 ? Math.round(totalCycleDuration / hostCount) : 0,
    averageDiscoveredServices: hostCount > 0 ? Math.round(totalDiscoveredServices / hostCount) : 0,
    sources,
    hostRows,
  };
}
