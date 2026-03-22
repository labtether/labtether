import type { Asset, Group } from "../../../console/models";
import {
  childParentKey,
  hostParentKey,
  isInfraHost,
  isDeviceTier,
} from "../../../console/taxonomy";
import type { AssetDependency } from "./useAssetDependencies";

export type WorkloadSummary = {
  vms: number;
  containers: number;
  services: number;
  stacks: number;
  pools: number;
  datasets: number;
  other: number;
};

export type HierarchyEntry = {
  host: Asset;
  deviceChildren: Asset[];
  workloadChildren: Asset[];
  workloads: WorkloadSummary;
};

export type HierarchyMap = {
  entries: HierarchyEntry[];
  orphans: Asset[];
  parentOf: Map<string, string>;
  childIDs: Set<string>;
};

const EMPTY_WORKLOADS: WorkloadSummary = {
  vms: 0,
  containers: 0,
  services: 0,
  stacks: 0,
  pools: 0,
  datasets: 0,
  other: 0,
};

function classifyChild(type: string): keyof WorkloadSummary {
  const normalized = type.trim().toLowerCase();
  if (normalized === "vm") return "vms";
  if (normalized === "container" || normalized === "docker-container" || normalized === "pod")
    return "containers";
  if (normalized === "service" || normalized === "ha-entity") return "services";
  if (normalized === "stack" || normalized === "compose-stack" || normalized === "deployment")
    return "stacks";
  if (normalized === "storage-pool" || normalized === "datastore") return "pools";
  if (normalized === "dataset" || normalized === "disk" || normalized === "share-smb" || normalized === "share-nfs" || normalized === "snapshot")
    return "datasets";
  return "other";
}

/**
 * Build a hierarchy map from assets, dependency edges, and stored groups.
 *
 * Priority order for parent-child resolution:
 * 0. "contains" dependency edges — stored infrastructure containment from groups
 * 1. Explicit dependency edges (runs_on, hosted_on)
 * 2. Metadata-inferred matching via taxonomy keys when no explicit edge exists
 *
 * Steps:
 * 1. Identify infrastructure hosts
 * 2. Merge Docker container-hosts with their agent hosts
 * 3. Build parent-key lookup from hosts
 * 4. Apply "contains" edges from stored group hierarchy
 * 5. Apply runs_on/hosted_on edges
 * 6. Fall back to taxonomy key inference for unlinked assets
 * 7. Collect orphans (assets that are neither hosts nor matched children)
 */
export function buildHierarchy(
  assets: Asset[],
  dependencies: AssetDependency[],
  groups?: Group[],
): HierarchyMap {
  const assetByID = new Map<string, Asset>();
  for (const a of assets) assetByID.set(a.id, a);

  // Build group lookup for group-based containment context
  const _groupByID = new Map<string, Group>();
  for (const g of groups ?? []) _groupByID.set(g.id, g);

  // Step 1: Identify infra hosts and non-hosts
  const infraHosts: Asset[] = [];
  const nonHosts: Asset[] = [];
  for (const a of assets) {
    if (isInfraHost(a)) infraHosts.push(a);
    else nonHosts.push(a);
  }

  // Step 2: Merge Docker container-hosts with agent hosts
  const mergedContainerHostIDs = new Set<string>();
  const agentForHost = new Map<string, string>();

  for (const host of infraHosts) {
    if (
      (host.type === "container-host" || host.source === "docker") &&
      host.metadata?.agent_id
    ) {
      const agentAsset = assets.find(
        (a) =>
          a.source === "agent" &&
          a.id !== host.id &&
          (a.id === host.metadata!.agent_id ||
            a.metadata?.agent_id === host.metadata!.agent_id),
      );
      if (agentAsset && isInfraHost(agentAsset)) {
        mergedContainerHostIDs.add(host.id);
        agentForHost.set(host.id, agentAsset.id);
      }
    }
  }

  // Step 3: Build parent-key lookup from non-merged hosts
  const activeHosts = infraHosts.filter((h) => !mergedContainerHostIDs.has(h.id));
  const hostByParentKey = new Map<string, Asset>();
  for (const host of activeHosts) {
    const pk = hostParentKey(host);
    if (pk) hostByParentKey.set(pk, host);
  }
  // Redirect merged container-host parent keys to agent hosts
  for (const [containerHostID, agentID] of agentForHost) {
    const containerHost = assetByID.get(containerHostID);
    if (containerHost) {
      const pk = hostParentKey(containerHost);
      if (pk) {
        const agentHost = assetByID.get(agentID);
        if (agentHost) hostByParentKey.set(pk, agentHost);
      }
    }
  }

  // Step 4 + 5: Match children to parents
  const parentOf = new Map<string, string>();
  const childIDs = new Set<string>();
  const deviceChildrenMap = new Map<string, Asset[]>();
  const workloadChildrenMap = new Map<string, Asset[]>();
  const workloadMap = new Map<string, WorkloadSummary>();

  for (const host of activeHosts) {
    deviceChildrenMap.set(host.id, []);
    workloadChildrenMap.set(host.id, []);
    workloadMap.set(host.id, { ...EMPTY_WORKLOADS });
  }

  /** Helper: assign child to parent in the hierarchy maps. */
  function assignChild(child: Asset, parentID: string): void {
    if (parentOf.has(child.id)) return;
    // Ensure parent has child-tracking maps (may be a non-infra-host parent via contains)
    if (!deviceChildrenMap.has(parentID)) {
      deviceChildrenMap.set(parentID, []);
      workloadChildrenMap.set(parentID, []);
      workloadMap.set(parentID, { ...EMPTY_WORKLOADS });
    }

    parentOf.set(child.id, parentID);
    childIDs.add(child.id);

    if (isDeviceTier(child)) {
      deviceChildrenMap.get(parentID)!.push(child);
    } else {
      workloadChildrenMap.get(parentID)!.push(child);
      const bucket = classifyChild(child.type);
      workloadMap.get(parentID)![bucket]++;
    }
  }

  // Filter out suggested/dismissed edges — they should not participate in
  // hierarchy resolution. If an edge has no origin field (old data), treat
  // it as accepted.
  const acceptedDependencies = dependencies.filter((dep) => {
    if (!dep.origin) return true;
    return dep.origin !== "suggested" && dep.origin !== "dismissed";
  });

  // First: use "contains" dependency edges from stored group hierarchy.
  // These represent explicit infrastructure containment (parent contains child).
  for (const dep of acceptedDependencies) {
    const relationshipType = typeof dep.relationship_type === "string"
      ? dep.relationship_type.trim().toLowerCase()
      : "";
    if (relationshipType !== "contains") continue;
    // "contains" edges: source_asset_id is the parent (container), target is the child
    const parent = assetByID.get(dep.source_asset_id);
    const child = assetByID.get(dep.target_asset_id);
    if (!parent || !child) continue;
    if (parent.id === child.id) continue;
    // Ensure parent is tracked as an active host for hierarchy entries
    if (!activeHosts.some((h) => h.id === parent.id)) {
      activeHosts.push(parent);
    }
    assignChild(child, parent.id);
  }

  // Second: use explicit dependency edges (runs_on, hosted_on)
  for (const dep of acceptedDependencies) {
    const relationshipType = typeof dep.relationship_type === "string"
      ? dep.relationship_type.trim().toLowerCase()
      : "";
    if (relationshipType !== "runs_on" && relationshipType !== "hosted_on") continue;
    const child = assetByID.get(dep.source_asset_id);
    const parent = assetByID.get(dep.target_asset_id);
    if (!child || !parent) continue;
    if (!deviceChildrenMap.has(parent.id)) continue;
    assignChild(child, parent.id);
  }

  // Third: inferred matching via taxonomy keys when no explicit edge exists.
  for (const asset of nonHosts) {
    if (parentOf.has(asset.id)) continue;
    if (mergedContainerHostIDs.has(asset.id)) continue;

    const cpk = childParentKey(asset);
    if (!cpk) continue;
    const parentHost = hostByParentKey.get(cpk);
    if (!parentHost) continue;

    assignChild(asset, parentHost.id);
  }

  // Step 6: Build entries
  const activeHostSet = new Set(activeHosts.map((h) => h.id));
  const entries = activeHosts
    .filter((host) => activeHostSet.has(host.id))
    .map((host) => ({
      host,
      deviceChildren: deviceChildrenMap.get(host.id) ?? [],
      workloadChildren: workloadChildrenMap.get(host.id) ?? [],
      workloads: workloadMap.get(host.id) ?? { ...EMPTY_WORKLOADS },
    }));

  // Deduplicate entries (a host may have been added twice if it was both
  // an infra host and a "contains" parent)
  const seenHostIDs = new Set<string>();
  const dedupedEntries = entries.filter((entry) => {
    if (seenHostIDs.has(entry.host.id)) return false;
    seenHostIDs.add(entry.host.id);
    return true;
  });

  // Step 7: Collect orphans
  const allHostIDs = new Set(activeHosts.map((h) => h.id));
  const orphans = assets.filter(
    (a) =>
      !childIDs.has(a.id) &&
      !allHostIDs.has(a.id) &&
      !mergedContainerHostIDs.has(a.id),
  );

  return { entries: dedupedEntries, orphans, parentOf, childIDs };
}

/**
 * Format a WorkloadSummary into a human-readable string like "3 VMs · 12 containers · 2 stacks".
 */
export function formatWorkloadSummary(w: WorkloadSummary): string {
  const parts: string[] = [];
  if (w.vms > 0) parts.push(`${w.vms} VM${w.vms !== 1 ? "s" : ""}`);
  if (w.containers > 0) parts.push(`${w.containers} container${w.containers !== 1 ? "s" : ""}`);
  if (w.services > 0) parts.push(`${w.services} service${w.services !== 1 ? "s" : ""}`);
  if (w.stacks > 0) parts.push(`${w.stacks} stack${w.stacks !== 1 ? "s" : ""}`);
  if (w.pools > 0) parts.push(`${w.pools} pool${w.pools !== 1 ? "s" : ""}`);
  if (w.datasets > 0) parts.push(`${w.datasets} dataset${w.datasets !== 1 ? "s" : ""}`);
  if (w.other > 0) parts.push(`${w.other} other`);
  return parts.join(" · ") || "no workloads";
}
