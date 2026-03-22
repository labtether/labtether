import { assetFreshnessLabel } from "../../../console/formatters";
import type { Asset, TelemetryOverviewAsset } from "../../../console/models";
import {
  childParentKey,
  hostParentKey,
  isInfraChild,
  isInfraHost,
  isHiddenAsset,
  isHomeAssistantHubAsset,
  isHomeAssistantEntityAsset,
  isDeviceTier,
} from "../../../console/taxonomy";

export type Freshness = "online" | "unresponsive" | "offline" | "unknown";

export type FreshnessCounts = {
  total: number;
  online: number;
  unresponsive: number;
  offline: number;
  unknown: number;
  issues: number;
};

export type DensityMode = "compact" | "diagnostic";

export type GroupSection = {
  groupID: string;
  groupLabel: string;
  counts: FreshnessCounts;
  servers: Array<{ server: Asset; children: Asset[] }>;
  orphans: Asset[];
  regular: Asset[];
};

/* ---------- Device card types ---------- */

export type WorkloadSummary = {
  vms: number;
  containers: number;
  stacks: number;
  datastores: number;
  other: number;
};

export type HAHubSummary = {
  entityCount: number;
  unavailableCount: number;
  automationCount: number;
  automationsDisabled: number;
  /** Top domains sorted by count descending. */
  domains: Array<{ domain: string; count: number }>;
  /** Total number of unique domains (for "+N more" overflow). */
  totalDomains: number;
};

export type DeviceCardData = {
  asset: Asset;
  freshness: Freshness;
  cpu: number | null;
  mem: number | null;
  disk: number | null;
  workloads: WorkloadSummary;
  hostedOn: { id: string; name: string } | null;
  /** Merged Docker container-host — present when Docker runs on this agent host. */
  dockerHost: { id: string; name: string } | null;
  /** HA hub summary — only present for Home Assistant connector-cluster assets. */
  haHub: HAHubSummary | null;
};

export type DeviceGroupSection = {
  groupID: string;
  groupLabel: string;
  devices: DeviceCardData[];
  counts: FreshnessCounts;
};

/* ---------- Helpers ---------- */

export function assetFreshness(asset: Asset): Freshness {
  const freshness = assetFreshnessLabel(asset.last_seen_at);
  if (
    freshness === "online" ||
    freshness === "unresponsive" ||
    freshness === "offline"
  ) {
    return freshness;
  }
  return "unknown";
}

function freshnessRank(freshness: Freshness): number {
  if (freshness === "offline") return 0;
  if (freshness === "unresponsive") return 1;
  if (freshness === "unknown") return 2;
  return 3;
}

function freshnessCounts(assets: Asset[]): FreshnessCounts {
  let online = 0;
  let unresponsive = 0;
  let offline = 0;
  let unknown = 0;

  for (const asset of assets) {
    const freshness = assetFreshness(asset);
    if (freshness === "online") online++;
    else if (freshness === "unresponsive") unresponsive++;
    else if (freshness === "offline") offline++;
    else unknown++;
  }

  return {
    total: assets.length,
    online,
    unresponsive,
    offline,
    unknown,
    issues: unresponsive + offline + unknown,
  };
}

export function parsePercent(value: string | undefined): number | null {
  if (value == null) return null;
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return null;
  if (numeric < 0) return 0;
  if (numeric > 100) return 100;
  return numeric;
}

/* ---------- Device card sort ---------- */

const SOURCE_PRIORITY: Record<string, number> = {
  proxmox: 0,
  truenas: 1,
  docker: 2,
  portainer: 3,
  pbs: 4,
  homeassistant: 5,
  agent: 6,
};

function sourcePriority(source: string): number {
  return SOURCE_PRIORITY[source.toLowerCase()] ?? 99;
}

function deviceSortComparator(a: DeviceCardData, b: DeviceCardData): number {
  const fa = freshnessRank(a.freshness);
  const fb = freshnessRank(b.freshness);
  if (fa !== fb) return fa - fb;
  const sa = sourcePriority(a.asset.source);
  const sb = sourcePriority(b.asset.source);
  if (sa !== sb) return sa - sb;
  return a.asset.name.localeCompare(b.asset.name);
}

/* ---------- Device search ---------- */

function includesDeviceQuery(asset: Asset, query: string): boolean {
  if (!query) return true;
  const terms: string[] = [
    asset.name, asset.type, asset.resource_kind ?? "",
    asset.source, asset.platform ?? "",
  ];
  terms.push(...(asset.tags ?? []));
  if (asset.metadata) {
    terms.push(...Object.values(asset.metadata));
  }
  return terms.join(" ").toLowerCase().includes(query);
}

/* ---------- Device card section builder ---------- */

export function buildDeviceCardSections(params: {
  assets: Asset[];
  telemetryOverview: TelemetryOverviewAsset[];
  query: string;
  groupLabelByID: ReadonlyMap<string, string>;
}): DeviceGroupSection[] {
  const { assets, telemetryOverview, query, groupLabelByID } = params;

  // 1. Telemetry lookup
  const telemetryByID = new Map<string, TelemetryOverviewAsset>();
  for (const t of telemetryOverview) {
    telemetryByID.set(t.asset_id, t);
  }

  // 1a. Pre-compute HA hub summaries from entity children.
  const haHubSummaries = new Map<string, HAHubSummary>();
  {
    const entitiesByCollector = new Map<string, Asset[]>();
    for (const asset of assets) {
      if (asset.source === "homeassistant" && asset.type === "ha-entity") {
        const cid = asset.metadata?.collector_id ?? "";
        const list = entitiesByCollector.get(cid) ?? [];
        list.push(asset);
        entitiesByCollector.set(cid, list);
      }
    }
    for (const asset of assets) {
      if (asset.source === "homeassistant" && asset.type === "connector-cluster") {
        const cid = asset.metadata?.collector_id ?? "";
        const children = entitiesByCollector.get(cid) ?? [];
        const domainCounts = new Map<string, number>();
        let unavailable = 0;
        let automationCount = 0;
        let automationsDisabled = 0;
        for (const child of children) {
          const domain = child.metadata?.domain ?? "unknown";
          domainCounts.set(domain, (domainCounts.get(domain) ?? 0) + 1);
          if (child.status === "offline") unavailable++;
          if (domain === "automation") {
            automationCount++;
            if (child.metadata?.state === "off") automationsDisabled++;
          }
        }
        const sortedDomains = [...domainCounts.entries()]
          .sort((a, b) => b[1] - a[1])
          .map(([domain, count]) => ({ domain, count }));
        haHubSummaries.set(asset.id, {
          entityCount: children.length,
          unavailableCount: unavailable,
          automationCount,
          automationsDisabled,
          domains: sortedDomains.slice(0, 5),
          totalDomains: sortedDomains.length,
        });
      }
    }
  }

  // 2. Separate device-tier from non-device in one pass to reduce allocations.
  const deviceAssets: Asset[] = [];
  const nonDeviceAssets: Asset[] = [];
  for (const asset of assets) {
    if (isHomeAssistantEntityAsset(asset)) continue; // entities only visible on hub detail page
    const includeAsHomeAssistantHub = isHomeAssistantHubAsset(asset);
    if (isHiddenAsset(asset) && !includeAsHomeAssistantHub) continue;
    if (includeAsHomeAssistantHub || isDeviceTier(asset)) {
      deviceAssets.push(asset);
      continue;
    }
    nonDeviceAssets.push(asset);
  }

  // 2a. Detect Docker container-hosts that should merge into their agent host card.
  //     When an agent runs on the same machine as Docker, both produce a device-tier
  //     asset. The Docker container-host carries metadata.agent_id matching the agent
  //     host's asset ID — suppress the container-host card and roll its workloads up.
  const agentHostByID = new Map<string, Asset>();
  for (const device of deviceAssets) {
    if (device.source === "agent") agentHostByID.set(device.id, device);
  }
  const mergedContainerHosts = new Set<string>();
  const dockerHostByAgentID = new Map<string, Asset>();
  for (const device of deviceAssets) {
    if (
      device.source === "docker" &&
      device.type === "container-host" &&
      device.metadata?.agent_id &&
      agentHostByID.has(device.metadata.agent_id)
    ) {
      mergedContainerHosts.add(device.id);
      dockerHostByAgentID.set(device.metadata.agent_id, device);
    }
  }

  // 3. Parent key lookup for infra hosts
  //    Merged container-hosts redirect their key to the agent host so workload
  //    counts roll up to the correct card.
  const deviceByParentKey = new Map<string, Asset>();
  for (const device of deviceAssets) {
    if (isInfraHost(device)) {
      const parentKey = hostParentKey(device);
      if (mergedContainerHosts.has(device.id) && device.metadata?.agent_id) {
        const agentHost = agentHostByID.get(device.metadata.agent_id);
        if (agentHost) {
          deviceByParentKey.set(parentKey, agentHost);
          continue;
        }
      }
      deviceByParentKey.set(parentKey, device);
    }
  }

  // 4. Count workloads per device (skip device-tier children — they get own cards)
  const workloadsByDeviceID = new Map<string, WorkloadSummary>();
  for (const child of nonDeviceAssets) {
    const parentKey = childParentKey(child);
    if (!parentKey) continue;
    const parent = deviceByParentKey.get(parentKey);
    if (!parent) continue;

    const summary = workloadsByDeviceID.get(parent.id) ?? { vms: 0, containers: 0, stacks: 0, datastores: 0, other: 0 };
    const type = child.type;
    if (type === "vm") summary.vms++;
    else if (type === "container" || type === "docker-container") summary.containers++;
    else if (type === "stack" || type === "compose-stack") summary.stacks++;
    else if (type === "storage-pool" || type === "datastore") summary.datastores++;
    else summary.other++;
    workloadsByDeviceID.set(parent.id, summary);
  }

  // 5. Build "hosted on" for device-tier assets that have a parent host
  const hostedOnByID = new Map<string, { id: string; name: string }>();
  for (const device of deviceAssets) {
    if (!isInfraChild(device) && device.type !== "vm") continue;
    const parentKey = childParentKey(device);
    if (!parentKey) continue;
    const parent = deviceByParentKey.get(parentKey);
    if (!parent || parent.id === device.id) continue;
    hostedOnByID.set(device.id, { id: parent.id, name: parent.name });
  }

  // 6. Build card data, apply search, compute metrics
  //    Skip merged container-hosts — their workloads are already on the agent card.
  const cardDataList: DeviceCardData[] = [];
  for (const device of deviceAssets) {
    if (mergedContainerHosts.has(device.id)) continue;
    if (!includesDeviceQuery(device, query)) continue;

    const freshness = assetFreshness(device);
    const tele = telemetryByID.get(device.id);

    const cpu = tele?.metrics.cpu_used_percent ?? parsePercent(device.metadata?.cpu_used_percent ?? device.metadata?.cpu_percent ?? device.metadata?.ha_cpu_percent);
    const mem = tele?.metrics.memory_used_percent ?? parsePercent(device.metadata?.memory_used_percent ?? device.metadata?.memory_percent ?? device.metadata?.ha_memory_used_percent);
    const disk = tele?.metrics.disk_used_percent ?? parsePercent(device.metadata?.disk_used_percent ?? device.metadata?.ha_disk_used_percent);

    const merged = dockerHostByAgentID.get(device.id);
    cardDataList.push({
      asset: device,
      freshness,
      cpu: typeof cpu === "number" ? cpu : null,
      mem: typeof mem === "number" ? mem : null,
      disk: typeof disk === "number" ? disk : null,
      workloads: workloadsByDeviceID.get(device.id) ?? { vms: 0, containers: 0, stacks: 0, datastores: 0, other: 0 },
      hostedOn: hostedOnByID.get(device.id) ?? null,
      dockerHost: merged ? { id: merged.id, name: merged.name } : null,
      haHub: haHubSummaries.get(device.id) ?? null,
    });
  }

  // 7. Group by group
  const byGroup = new Map<string, DeviceCardData[]>();
  for (const card of cardDataList) {
    const key = card.asset.group_id || "unassigned";
    const list = byGroup.get(key) ?? [];
    list.push(card);
    byGroup.set(key, list);
  }

  // 8. Build and sort sections
  const sections: DeviceGroupSection[] = [];
  for (const [groupID, devices] of byGroup) {
    devices.sort(deviceSortComparator);
    sections.push({
      groupID,
      groupLabel: groupID === "unassigned" ? "Unassigned" : (groupLabelByID.get(groupID) ?? groupID),
      devices,
      counts: freshnessCounts(devices.map(d => d.asset)),
    });
  }

  return sections.sort((a, b) => {
    if (a.groupID === "unassigned") return 1;
    if (b.groupID === "unassigned") return -1;
    const issueDiff = b.counts.issues - a.counts.issues;
    if (issueDiff !== 0) return issueDiff;
    return a.groupLabel.localeCompare(b.groupLabel);
  });
}
