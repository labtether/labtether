import type { Asset, TelemetryOverviewAsset } from "../../../console/models";
import {
  childParentKey,
  hostParentKey,
  isDeviceTier,
  isHiddenAsset,
  isHomeAssistantHubAsset,
  isInfraChild,
  isInfraHost,
} from "../../../console/taxonomy";
import { freshnessRank } from "./deviceTreeModel";
import { assetFreshness, parsePercent, type DeviceCardData, type WorkloadSummary } from "./nodesPageUtils";

/**
 * Device rows from the raw status feed: device-tier assets with their metrics, the host each guest runs on and the
 * workloads each host carries, grouped by group id ("__ungrouped__" for none) and sorted problems first, then by name.
 * Shared by the Devices list and the group tree.
 */
export function buildDeviceCards(
  assets: Asset[],
  telemetryOverview: TelemetryOverviewAsset[],
): { cardsByGroupID: Map<string, DeviceCardData[]>; allCardIDs: string[] } {
  const telemetryByID = new Map<string, TelemetryOverviewAsset>();
  for (const t of telemetryOverview) {
    telemetryByID.set(t.asset_id, t);
  }

  const deviceAssets: Asset[] = [];
  const nonDeviceAssets: Asset[] = [];
  for (const asset of assets) {
    const includeAsHomeAssistantHub = isHomeAssistantHubAsset(asset);
    if (isHiddenAsset(asset) && !includeAsHomeAssistantHub) continue;
    if (includeAsHomeAssistantHub || isDeviceTier(asset)) {
      deviceAssets.push(asset);
    } else {
      nonDeviceAssets.push(asset);
    }
  }

  // Detect merged Docker container-hosts
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

  // Parent key lookup
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

  // Workload counts
  const workloadsByDeviceID = new Map<string, WorkloadSummary>();
  for (const child of nonDeviceAssets) {
    const pk = childParentKey(child);
    if (!pk) continue;
    const parent = deviceByParentKey.get(pk);
    if (!parent) continue;
    const summary = workloadsByDeviceID.get(parent.id) ?? { vms: 0, containers: 0, stacks: 0, datastores: 0, other: 0 };
    const type = child.type;
    if (type === "vm") summary.vms++;
    else if (type === "container" || type === "docker-container") summary.containers++;
    else if (type === "stack" || type === "compose-stack") summary.stacks++;
    else summary.other++;
    workloadsByDeviceID.set(parent.id, summary);
  }

  // Hosted-on
  const hostedOnByID = new Map<string, { id: string; name: string }>();
  for (const device of deviceAssets) {
    if (!isInfraChild(device) && device.type !== "vm") continue;
    const pk = childParentKey(device);
    if (!pk) continue;
    const parent = deviceByParentKey.get(pk);
    if (!parent || parent.id === device.id) continue;
    hostedOnByID.set(device.id, { id: parent.id, name: parent.name });
  }

  // Build cards, group by group_id
  const byGroup = new Map<string, DeviceCardData[]>();
  const ids: string[] = [];

  for (const device of deviceAssets) {
    if (mergedContainerHosts.has(device.id)) continue;

    const freshness = assetFreshness(device);
    const tele = telemetryByID.get(device.id);
    const cpu = tele?.metrics.cpu_used_percent ?? parsePercent(device.metadata?.cpu_used_percent ?? device.metadata?.cpu_percent);
    const mem = tele?.metrics.memory_used_percent ?? parsePercent(device.metadata?.memory_used_percent ?? device.metadata?.memory_percent);
    const disk = tele?.metrics.disk_used_percent ?? parsePercent(device.metadata?.disk_used_percent);
    const merged = dockerHostByAgentID.get(device.id);

    const card: DeviceCardData = {
      asset: device,
      freshness,
      cpu: typeof cpu === "number" ? cpu : null,
      mem: typeof mem === "number" ? mem : null,
      disk: typeof disk === "number" ? disk : null,
      workloads: workloadsByDeviceID.get(device.id) ?? { vms: 0, containers: 0, stacks: 0, datastores: 0, other: 0 },
      hostedOn: hostedOnByID.get(device.id) ?? null,
      dockerHost: merged ? { id: merged.id, name: merged.name } : null,
      haHub: null,
    };

    const key = device.group_id || "__ungrouped__";
    const list = byGroup.get(key) ?? [];
    list.push(card);
    byGroup.set(key, list);
    ids.push(device.id);
  }

  // Sort within each group
  for (const [, cards] of byGroup) {
    cards.sort((a, b) => {
      const fa = freshnessRank(a.freshness);
      const fb = freshnessRank(b.freshness);
      if (fa !== fb) return fa - fb;
      return a.asset.name.localeCompare(b.asset.name);
    });
  }

  return { cardsByGroupID: byGroup, allCardIDs: ids };
}
