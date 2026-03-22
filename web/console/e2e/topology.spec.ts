import { expect, test, type Page } from "@playwright/test";
import { childParentKey, hostParentKey, isInfraHost } from "../app/console/taxonomy";
import { buildLiveStatusPayload, buildStatusPayload, installConsoleApiMocks } from "./helpers/consoleApiMocks";

type TopologyAsset = {
  id: string;
  name: string;
  type: string;
  source: string;
  status: string;
  platform: string;
  last_seen_at: string;
  metadata?: Record<string, string>;
};

type TopologyDependency = {
  id: string;
  source_asset_id: string;
  target_asset_id: string;
  relationship_type: string;
};

type TopologyGroup = {
  id: string;
  name: string;
  slug: string;
  parent_group_id?: string;
  sort_order: number;
  created_at: string;
  updated_at: string;
};

type MutableTopologyAsset = TopologyAsset & {
  group_id?: string;
};

const BASE_TS = "2026-01-01T12:00:00.000Z";

async function mockTopologyData(page: Page, assets: TopologyAsset[], dependencies: unknown[]) {
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

function deriveInferredDependencies(
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

function cloneAsset(asset: MutableTopologyAsset): MutableTopologyAsset {
  return {
    ...asset,
    metadata: asset.metadata ? { ...asset.metadata } : undefined,
  };
}

function cloneGroup(group: TopologyGroup): TopologyGroup {
  return { ...group };
}

function isTopologyDependency(value: unknown): value is TopologyDependency {
  return typeof value === "object" && value !== null
    && typeof (value as TopologyDependency).id === "string"
    && typeof (value as TopologyDependency).source_asset_id === "string"
    && typeof (value as TopologyDependency).target_asset_id === "string"
    && typeof (value as TopologyDependency).relationship_type === "string";
}

function normalizeRelationshipType(relationshipType: string): "runs_on" | "hosted_on" | null {
  const normalized = relationshipType.trim().toLowerCase();
  if (normalized === "runs_on") return "runs_on";
  if (normalized === "hosted_on" || normalized === "contains") return "hosted_on";
  return null;
}

function buildTopologyPayload(
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
      source_asset_id: dependency.source_asset_id,
      target_asset_id: dependency.target_asset_id,
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

function syncTopologyPayloads(
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

function cascadeGroupAssignment(
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

async function installGroupTopologyWorkflowMocks(
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

async function assignDeviceToGroup(page: Page, assetID: string, groupID: string) {
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

async function switchToTreeView(page: Page) {
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

test.describe("topology tree relationships", () => {
  test("keeps runs_on semantics aligned in tree view", async ({ page }) => {
    await mockTopologyData(
      page,
      [
        {
          id: "asset-host-1",
          name: "Agent Host",
          type: "host",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
        },
        {
          id: "asset-vm-1",
          name: "Guest VM",
          type: "vm",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
        },
      ],
      [
        {
          id: "dep-runs-on-1",
          source_asset_id: "asset-vm-1",
          target_asset_id: "asset-host-1",
          relationship_type: "runs_on",
        },
      ],
    );

    await page.goto("/topology", { waitUntil: "domcontentloaded" });
    await switchToTreeView(page);

    await page.getByText("Agent Host", { exact: true }).first().click();

    const inspectorPanel = page
      .locator("div")
      .filter({ has: page.getByRole("button", { name: "Close inspector" }) })
      .first();

    await expect(inspectorPanel).toBeVisible();
    await expect(inspectorPanel.getByText("Connections (1)", { exact: true })).toBeVisible();
    await expect(inspectorPanel.getByText("← Guest VM", { exact: true })).toBeVisible();
    await expect(inspectorPanel.getByText("runs on", { exact: true }).first()).toBeVisible();
  });

  test("keeps docker host/container hierarchy connected when filtering to Docker source in tree view", async ({ page }) => {
    await mockTopologyData(
      page,
      [
        {
          id: "agent-host-srv-1",
          name: "Agent Host",
          type: "host",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: {
            agent_id: "srv-1",
          },
        },
        {
          id: "docker-host-srv-1",
          name: "Docker Host",
          type: "container-host",
          source: "docker",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: {
            agent_id: "srv-1",
          },
        },
        {
          id: "docker-container-srv-1",
          name: "App Container",
          type: "docker-container",
          source: "docker",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: {
            agent_id: "srv-1",
          },
        },
      ],
      [],
    );

    await page.goto("/topology", { waitUntil: "domcontentloaded" });
    await switchToTreeView(page);
    await expect(page.getByText("No zones or assets.", { exact: true })).toHaveCount(0);

    await page.getByText("Docker Host", { exact: true }).first().click();

    const inspectorPanel = page
      .locator("div")
      .filter({ has: page.getByRole("button", { name: "Close inspector" }) })
      .first();

    await expect(inspectorPanel).toBeVisible();
    await expect(inspectorPanel.getByText("Connections (1)", { exact: true })).toBeVisible();
    await expect(inspectorPanel.getByText("← App Container", { exact: true })).toBeVisible();
    await expect(inspectorPanel.getByText("hosted on", { exact: true }).first()).toBeVisible();
  });

  test("infers docker host/container hierarchy from docker asset IDs when child metadata.agent_id is missing", async ({ page }) => {
    await mockTopologyData(
      page,
      [
        {
          id: "docker-host-containervm-deltaserver",
          name: "docker-containervm-deltaserver",
          type: "container-host",
          source: "docker",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: {
            agent_id: "containervm-deltaserver",
          },
        },
        {
          id: "docker-ct-containervm-deltaserver-abc123def456",
          name: "App Container",
          type: "docker-container",
          source: "docker",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: {
            container_id: "abc123def456",
          },
        },
      ],
      [],
    );

    await page.goto("/topology", { waitUntil: "domcontentloaded" });
    await switchToTreeView(page);
    await expect(page.getByText("No zones or assets.", { exact: true })).toHaveCount(0);

    await page.getByText("docker-containervm-deltaserver", { exact: true }).first().click();

    const inspectorPanel = page
      .locator("div")
      .filter({ has: page.getByRole("button", { name: "Close inspector" }) })
      .first();

    await expect(inspectorPanel).toBeVisible();
    await expect(inspectorPanel.getByText("Connections (1)", { exact: true })).toBeVisible();
    await expect(inspectorPanel.getByText("← App Container", { exact: true })).toBeVisible();
    await expect(inspectorPanel.getByText("hosted on", { exact: true }).first()).toBeVisible();
  });

  test("renders inferred containment on the canvas", async ({ page }) => {
    await mockTopologyData(
      page,
      [
        {
          id: "docker-host-srv-1",
          name: "Docker Host",
          type: "container-host",
          source: "docker",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: {
            agent_id: "srv-1",
          },
        },
        {
          id: "docker-vm-srv-1-a",
          name: "VM A",
          type: "vm",
          source: "docker",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: {
            agent_id: "srv-1",
          },
        },
        {
          id: "docker-vm-srv-1-b",
          name: "VM B",
          type: "vm",
          source: "docker",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: {
            agent_id: "srv-1",
          },
        },
      ],
      [],
    );

    await page.goto("/topology", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /Docker Host container-host docker no workloads/i })).toBeVisible();
    await expect(page.getByText("VM A", { exact: true })).toHaveCount(0);
    await expect(page.getByText("VM B", { exact: true })).toHaveCount(0);
  });

  test("keeps tree visible when graph lanes are hidden", async ({ page }) => {
    await mockTopologyData(
      page,
      [
        {
          id: "asset-host-2",
          name: "Node Host",
          type: "host",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
        },
        {
          id: "asset-vm-2",
          name: "Node VM",
          type: "vm",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
        },
      ],
      [
        {
          id: "dep-runs-on-2",
          source_asset_id: "asset-vm-2",
          target_asset_id: "asset-host-2",
          relationship_type: "runs_on",
        },
      ],
    );

    await page.goto("/topology", { waitUntil: "domcontentloaded" });
    await switchToTreeView(page);
    await expect(page.getByText("No zones or assets.", { exact: true })).toHaveCount(0);
    await expect(page.getByText("Node Host", { exact: true }).first()).toBeVisible();
  });

  test("shows dependency-linked service assets in topology tree view", async ({ page }) => {
    await mockTopologyData(
      page,
      [
        {
          id: "agent-host-3",
          name: "Lab Host",
          type: "host",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
        },
        {
          id: "agent-service-3",
          name: "Lab API",
          type: "service",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
        },
      ],
      [
        {
          id: "dep-runs-on-3",
          source_asset_id: "agent-service-3",
          target_asset_id: "agent-host-3",
          relationship_type: "runs_on",
        },
      ],
    );

    await page.goto("/topology", { waitUntil: "domcontentloaded" });
    await switchToTreeView(page);
    await page.getByText("Lab Host", { exact: true }).first().click();

    await expect(page.getByText("Lab API", { exact: true }).first()).toBeVisible();
  });

  test("ignores malformed dependency rows without breaking topology rendering", async ({ page }) => {
    await mockTopologyData(
      page,
      [
        {
          id: "agent-host-4",
          name: "Lab Host 4",
          type: "host",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
        },
        {
          id: "agent-vm-4",
          name: "Lab VM 4",
          type: "vm",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
        },
      ],
      [
        {
          id: "dep-valid-4",
          source_asset_id: "agent-vm-4",
          target_asset_id: "agent-host-4",
          relationship_type: "runs_on",
        },
        {
          id: "dep-bad-4",
          source_asset_id: "agent-vm-4",
          target_asset_id: "agent-host-4",
          relationship_type: null,
        } as unknown as TopologyDependency,
      ],
    );

    await page.goto("/topology", { waitUntil: "domcontentloaded" });
    await switchToTreeView(page);
    await expect(page.getByText("Lab Host 4", { exact: true }).first()).toBeVisible();
  });

  test("keeps created groups, assigned mixed-source devices, and topology lanes aligned", async ({ page }) => {
    await installGroupTopologyWorkflowMocks(page, {
      groups: [
        {
          id: "group-home",
          name: "Home",
          slug: "home",
          sort_order: 0,
          created_at: BASE_TS,
          updated_at: BASE_TS,
        },
      ],
      assets: [
        {
          id: "proxmox-node-1",
          name: "Proxmox Cluster",
          type: "hypervisor-node",
          source: "proxmox",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: { node: "pve01" },
        },
        {
          id: "proxmox-vm-201",
          name: "Kubernetes VM",
          type: "vm",
          source: "proxmox",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: { node: "pve01" },
        },
        {
          id: "docker-host-1",
          name: "Docker Runtime",
          type: "container-host",
          source: "docker",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: { agent_id: "srv-ops-1" },
        },
        {
          id: "docker-container-1",
          name: "API Container",
          type: "docker-container",
          source: "docker",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: { agent_id: "srv-ops-1", container_id: "abc123" },
        },
        {
          id: "truenas-controller-1",
          name: "TrueNAS Core",
          type: "nas",
          source: "truenas",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: { collector_id: "collector-truenas-1" },
        },
        {
          id: "truenas-dataset-1",
          name: "Media Dataset",
          type: "dataset",
          source: "truenas",
          status: "online",
          platform: "linux",
          last_seen_at: BASE_TS,
          metadata: { collector_id: "collector-truenas-1" },
        },
      ],
      dependencies: [
        {
          id: "dep-proxmox-vm",
          source_asset_id: "proxmox-vm-201",
          target_asset_id: "proxmox-node-1",
          relationship_type: "runs_on",
        },
        {
          id: "dep-docker-container",
          source_asset_id: "docker-container-1",
          target_asset_id: "docker-host-1",
          relationship_type: "hosted_on",
        },
        {
          id: "dep-truenas-dataset",
          source_asset_id: "truenas-controller-1",
          target_asset_id: "truenas-dataset-1",
          relationship_type: "contains",
        },
      ],
    });

    await page.goto("/groups", { waitUntil: "domcontentloaded" });
    await page.getByRole("button", { name: "New Group" }).click();
    await page.getByPlaceholder("Home Lab").fill("Operations");
    await page.getByPlaceholder("home-lab").fill("operations");
    await page.getByRole("button", { name: "Create Group", exact: true }).click();
    await expect(page.getByText("Operations", { exact: true })).toBeVisible();

    await assignDeviceToGroup(page, "proxmox-node-1", "group-operations");
    await assignDeviceToGroup(page, "docker-host-1", "group-operations");
    await assignDeviceToGroup(page, "truenas-controller-1", "group-operations");

    await page.goto("/topology", { waitUntil: "domcontentloaded" });
    await switchToTreeView(page);

    await expect(page.getByText("Operations", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("Proxmox Cluster", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("Docker Runtime", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("TrueNAS Core", { exact: true }).first()).toBeVisible();

    for (const parentAssetName of ["Proxmox Cluster", "Docker Runtime", "TrueNAS Core"]) {
      await page.getByText(parentAssetName, { exact: true }).first().click();
    }

    for (const assetName of ["Kubernetes VM", "API Container", "Media Dataset"]) {
      await page.getByText(assetName, { exact: true }).first().click();
      await expect(page.getByText(assetName, { exact: true }).first()).toBeVisible();
    }
  });
});
