import { expect,test } from "@playwright/test";
import { assignDeviceToGroup,BASE_TS,installGroupTopologyWorkflowMocks,mockTopologyData,switchToTreeView,TopologyDependency } from './helpers/topologyMocks';

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
