import { expect,type Page } from "@playwright/test";
import { childParentKey,hostParentKey,isInfraHost } from "../../app/console/taxonomy";
import { buildLiveStatusPayload,buildStatusPayload,installConsoleApiMocks } from "../helpers/consoleApiMocks";

export type TopologyAsset = {
  id: string;
  name: string;
  type: string;
  source: string;
  status: string;
  platform: string;
  last_seen_at: string;
  metadata?: Record<string, string>;
};

export type TopologyDependency = {
  id: string;
  source_asset_id: string;
  target_asset_id: string;
  relationship_type: string;
};

export type TopologyGroup = {
  id: string;
  name: string;
  slug: string;
  parent_group_id?: string;
  sort_order: number;
  created_at: string;
  updated_at: string;
};

export type MutableTopologyAsset = TopologyAsset & {
  group_id?: string;
};

export const BASE_TS = "2026-01-01T12:00:00.000Z";

export async function mockTopologyData(page: Page, assets: TopologyAsset[], dependencies: unknown[]) {
  const statusPayload = buildStatusPayload({ assets });
  const typedDependencies = deriveInferredDependencies(assets, dependencies.filter(isTopologyDependency));

  await installConsoleApiMocks(page, {
    statusPayload,
    liveStatusPayload: buildLiveStatusPayload({
      assets: statusPayload.assets as unknown[],
    }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/topology" && method === "GET") {
        await fulfillJSON(buildTopologyPayload([], assets, typedDependencies), 200);
        return true;
      }
      if (pathname === "/api/edges" && method === "GET") {
        await fulfillJSON({ edges: typedDependencies }, 200);
        return true;
      }
      return false;
    },
  });
}

export function deriveInferredDependencies(
  assets: TopologyAsset[],
  dependencies: TopologyDependency[],
): TopologyDependency[] {
  const inferred = [...dependencies];
  const seenEdges = new Set(dependencies.map((dependency) => `${dependency.source_asset_id}->${dependency.target_asset_id}`));
  const hostsByParentKey = new Map<string, TopologyAsset>();

  for (const asset of assets) {
    if (!isInfraHost(asset)) {
      continue;
    }
    const parentKey = hostParentKey(asset);
    if (!parentKey || hostsByParentKey.has(parentKey)) {
      continue;
    }
    hostsByParentKey.set(parentKey, asset);
  }

  for (const asset of assets) {
    if (isInfraHost(asset)) {
      continue;
    }
    const parentKey = childParentKey(asset);
    if (!parentKey) {
      continue;
    }
    const parent = hostsByParentKey.get(parentKey);
    if (!parent || parent.id === asset.id) {
      continue;
    }
    const edgeKey = `${asset.id}->${parent.id}`;
    if (seenEdges.has(edgeKey)) {
      continue;
    }
    seenEdges.add(edgeKey);
    inferred.push({
      id: `dep-inferred-${asset.id}-${parent.id}`,
      source_asset_id: asset.id,
      target_asset_id: parent.id,
      relationship_type: asset.source === "docker" && parent.source === "docker" ? "hosted_on" : "runs_on",
    });
  }

  return inferred;
}

export function cloneAsset(asset: MutableTopologyAsset): MutableTopologyAsset {
  return {
    ...asset,
    metadata: asset.metadata ? { ...asset.metadata } : undefined,
  };
}

export function cloneGroup(group: TopologyGroup): TopologyGroup {
  return { ...group };
}

export function isTopologyDependency(value: unknown): value is TopologyDependency {
  return typeof value === "object" && value !== null
    && typeof (value as TopologyDependency).id === "string"
    && typeof (value as TopologyDependency).source_asset_id === "string"
    && typeof (value as TopologyDependency).target_asset_id === "string"
    && typeof (value as TopologyDependency).relationship_type === "string";
}

export function normalizeRelationshipType(relationshipType: string): "runs_on" | "hosted_on" | null {
  const normalized = relationshipType.trim().toLowerCase();
  if (normalized === "runs_on") return "runs_on";
  if (normalized === "hosted_on" || normalized === "contains") return "hosted_on";
  return null;
}

export function buildTopologyPayload(
  groups: TopologyGroup[],
  assets: MutableTopologyAsset[],
  dependencies: TopologyDependency[],
) {
  const effectiveGroups = groups.length > 0
    ? groups
    : [{
        id: "zone-unsorted",
        name: "Unsorted",
        slug: "unsorted",
        sort_order: 0,
        created_at: BASE_TS,
        updated_at: BASE_TS,
      }];

  const zones = effectiveGroups.map((group, index) => ({
    id: group.id,
    topology_id: "topology-e2e",
    parent_zone_id: group.parent_group_id ?? null,
    label: group.name,
    color: "blue",
    icon: "folder",
    position: { x: 40 + index * 80, y: 40 + index * 40 },
    size: { width: 320, height: 220 },
    collapsed: false,
    sort_order: group.sort_order,
  }));

  const zoneIndexByID = new Map(effectiveGroups.map((group, index) => [group.id, index]));
  const members = assets
    .filter((asset) => groups.length === 0 || (typeof asset.group_id === "string" && asset.group_id.length > 0))
    .map((asset, index) => ({
      zone_id: groups.length === 0 ? effectiveGroups[0].id : asset.group_id as string,
      asset_id: asset.id,
      position: {
        x: 40 + ((index % 3) * 120),
        y: 40 + ((zoneIndexByID.get(groups.length === 0 ? effectiveGroups[0].id : asset.group_id as string) ?? 0) * 24) + Math.floor(index / 3) * 80,
      },
      sort_order: index,
    }));

  const unsorted = assets
    .filter((asset) => groups.length > 0 && !asset.group_id)
    .map((asset) => asset.id);

  const connections = dependencies.flatMap((dependency) => {
    const relationship = normalizeRelationshipType(dependency.relationship_type);
    if (!relationship) {
      return [];
    }
    return [{
      id: dependency.id,
      // Stored `contains` edges are parent -> child; the topology display
      // contract uses child -> parent for its containment-like connections.
      source_asset_id: dependency.relationship_type.trim().toLowerCase() === "contains"
        ? dependency.target_asset_id
        : dependency.source_asset_id,
      target_asset_id: dependency.relationship_type.trim().toLowerCase() === "contains"
        ? dependency.source_asset_id
        : dependency.target_asset_id,
      relationship,
      user_defined: false,
      label: "",
      origin: "discovered" as const,
    }];
  });

  return {
    data: {
      id: "topology-e2e",
      name: "LabTether",
      zones,
      members,
      connections,
      unsorted,
      viewport: { x: 0, y: 0, zoom: 1 },
    },
  };
}

export function syncTopologyPayloads(
  statusPayload: Record<string, unknown>,
  liveStatusPayload: Record<string, unknown>,
  groups: TopologyGroup[],
  assets: MutableTopologyAsset[],
) {
  const nextGroups = groups.map(cloneGroup);
  const nextAssets = assets.map(cloneAsset);

  statusPayload.groups = nextGroups;
  statusPayload.assets = nextAssets;
  statusPayload.summary = {
    ...((statusPayload.summary as Record<string, unknown> | undefined) ?? {}),
    groupCount: nextGroups.length,
    assetCount: nextAssets.length,
  };

  liveStatusPayload.assets = nextAssets;
  liveStatusPayload.summary = {
    ...((liveStatusPayload.summary as Record<string, unknown> | undefined) ?? {}),
    assetCount: nextAssets.length,
  };
}

export function cascadeGroupAssignment(
  assets: MutableTopologyAsset[],
  dependencies: TopologyDependency[],
  assetID: string,
  nextGroupID: string | undefined,
) {
  const queue = [assetID];
  const visited = new Set<string>();

  while (queue.length > 0) {
    const currentAssetID = queue.shift() ?? "";
    if (!currentAssetID || visited.has(currentAssetID)) continue;
    visited.add(currentAssetID);

    const asset = assets.find((candidate) => candidate.id === currentAssetID);
    if (!asset) continue;
    if (nextGroupID) asset.group_id = nextGroupID;
    else delete asset.group_id;

    for (const dependency of dependencies) {
      const relationshipType = dependency.relationship_type.trim().toLowerCase();
      if (
        (relationshipType === "runs_on" || relationshipType === "hosted_on")
        && dependency.target_asset_id === currentAssetID
      ) {
        queue.push(dependency.source_asset_id);
      }
      if (relationshipType === "contains" && dependency.source_asset_id === currentAssetID) {
        queue.push(dependency.target_asset_id);
      }
    }
  }
}

export async function installGroupTopologyWorkflowMocks(
  page: Page,
  options: {
    groups: TopologyGroup[];
    assets: MutableTopologyAsset[];
    dependencies: TopologyDependency[];
  },
) {
  const groups = options.groups.map(cloneGroup);
  const assets = options.assets.map(cloneAsset);
  const dependencies = options.dependencies.map((dependency) => ({ ...dependency }));
  const statusPayload = buildStatusPayload({ groups, assets });
  const liveStatusPayload = buildLiveStatusPayload({ assets });

  syncTopologyPayloads(statusPayload, liveStatusPayload, groups, assets);

  await installConsoleApiMocks(page, {
    statusPayload,
    liveStatusPayload,
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      const effectiveDependencies = deriveInferredDependencies(assets, dependencies);

      if (pathname === "/api/topology" && method === "GET") {
        await fulfillJSON(buildTopologyPayload(groups, assets, effectiveDependencies), 200);
        return true;
      }

      if (pathname === "/api/edges" && method === "GET") {
        await fulfillJSON({ edges: effectiveDependencies }, 200);
        return true;
      }

      if (pathname === "/api/groups" && method === "POST") {
        const name = typeof requestBody.name === "string" ? requestBody.name.trim() : "";
        const slug = typeof requestBody.slug === "string" ? requestBody.slug.trim() : "";
        const parentGroupID = typeof requestBody.parent_group_id === "string"
          ? requestBody.parent_group_id.trim()
          : "";
        const nextGroup: TopologyGroup = {
          id: `group-${slug || "new"}`,
          name: name || "New Group",
          slug: slug || "new-group",
          parent_group_id: parentGroupID || undefined,
          sort_order: groups.length,
          created_at: BASE_TS,
          updated_at: BASE_TS,
        };
        groups.push(nextGroup);
        syncTopologyPayloads(statusPayload, liveStatusPayload, groups, assets);
        await fulfillJSON({ group: cloneGroup(nextGroup) }, 201);
        return true;
      }

      if (/^\/api\/assets\/[^/]+$/.test(pathname) && method === "PATCH") {
        const assetID = decodeURIComponent(pathname.split("/").pop() ?? "");
        const asset = assets.find((candidate) => candidate.id === assetID);
        if (!asset) {
          await fulfillJSON({ error: `asset ${assetID} not found` }, 404);
          return true;
        }

        if (typeof requestBody.name === "string" && requestBody.name.trim()) {
          asset.name = requestBody.name.trim();
        }

        if ("group_id" in requestBody) {
          const requestedGroupID = typeof requestBody.group_id === "string"
            ? requestBody.group_id.trim()
            : "";
          cascadeGroupAssignment(assets, dependencies, assetID, requestedGroupID || undefined);
        }

        syncTopologyPayloads(statusPayload, liveStatusPayload, groups, assets);
        await fulfillJSON({ asset: cloneAsset(asset) }, 200);
        return true;
      }

      return false;
    },
  });
}

export async function assignDeviceToGroup(page: Page, assetID: string, groupID: string) {
  const response = await page.evaluate(async ({ assetID: nextAssetID, groupID: nextGroupID }) => {
    const result = await fetch(`/api/assets/${encodeURIComponent(nextAssetID)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ group_id: nextGroupID }),
    });

    return {
      ok: result.ok,
      status: result.status,
      body: await result.text(),
    };
  }, { assetID, groupID });

  expect(response.ok, response.body || `failed to assign ${assetID} to ${groupID} (${response.status})`).toBeTruthy();
}

export async function switchToTreeView(page: Page) {
  await expect(page.getByText("Loading topology canvas...", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Switch to tree view", exact: true }).click();
  const unsortedButton = page
    .locator("main")
    .getByRole("button")
    .filter({ hasText: /^[▶▼]\s*▪\s*Unsorted\b/ })
    .first();
  if (await unsortedButton.isVisible({ timeout: 5000 }).catch(() => false)) {
    await unsortedButton.click();
  }
}
