import { expect, test, type Locator, type Page } from "@playwright/test";
import { assignDeviceToGroup, BASE_TS, installGroupTopologyWorkflowMocks, mockTopologyData, type TopologyAsset, type TopologyDependency } from "./helpers/topologyMocks";

// Promise: the Map draws the hub's devices with their guests under their hosts, and selecting a device opens an
// Inspector that lists what the hub knows it is connected to (stored, discovered and inferred), worded from that
// device's side. Nothing on the Map is made up: a device the hub does not report never shows.

const asset = (id: string, name: string, type: string, source: string, metadata?: Record<string, string>): TopologyAsset =>
  ({ id, name, type, source, status: "online", platform: "linux", last_seen_at: BASE_TS, ...(metadata ? { metadata } : {}) });
const dep = (id: string, source: string, target: string, rel = "runs_on"): TopologyDependency =>
  ({ id, source_asset_id: source, target_asset_id: target, relationship_type: rel });

/** The tile of a device in the tree (its name, then its state). */
const node = (page: Page, name: string) => page.locator(".mp-n").getByRole("button", { name: new RegExp(`^${name},`) });
const guestRow = (page: Page, name: string) => page.locator(".mp-g", { hasText: name });
async function open(page: Page, name: string) {
  await node(page, name).click();
  const inspector = page.getByRole("complementary", { name, exact: true });
  await expect(inspector).toBeVisible();
  return inspector;
}
const connection = (inspector: Locator, other: string) => inspector.locator(".mp-lk", { hasText: other });

test.describe("map", () => {
  test("shows a VM on its host's spine and the runs-on link in the host's Inspector", async ({ page }) => {
    await mockTopologyData(page, [asset("asset-host-1", "Agent Host", "host", "agent"), asset("asset-vm-1", "Guest VM", "vm", "agent")],
      [dep("dep-runs-on-1", "asset-vm-1", "asset-host-1")]);
    await page.goto("/topology", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: "Map", level: 1 })).toBeVisible();
    await expect(guestRow(page, "Guest VM")).toBeVisible();
    await expect(page.locator(".mp-n", { hasText: "Guest VM" })).toHaveCount(0);

    const inspector = await open(page, "Agent Host");
    await expect(connection(inspector, "Guest VM")).toContainText("Runs");
  });

  test("lists a Docker host's container in its Inspector when the host is not merged into an agent", async ({ page }) => {
    await mockTopologyData(page, [
      asset("agent-host-srv-1", "Agent Host", "host", "agent", { agent_id: "srv-1" }),
      asset("docker-host-srv-1", "Docker Host", "container-host", "docker", { agent_id: "srv-1" }),
      asset("docker-container-srv-1", "App Container", "docker-container", "docker", { agent_id: "srv-1" }),
    ], []);
    await page.goto("/topology", { waitUntil: "domcontentloaded" });

    const inspector = await open(page, "Docker Host");
    await expect(connection(inspector, "App Container")).toContainText("Hosts");
  });

  test("infers a Docker host's container from the asset IDs when the container has no agent_id", async ({ page }) => {
    await mockTopologyData(page, [
      asset("docker-host-dockerhost1-northserver", "docker-dockerhost1-northserver", "container-host", "docker", { agent_id: "dockerhost1-northserver" }),
      asset("docker-ct-dockerhost1-northserver-abc123def456", "App Container", "docker-container", "docker", { container_id: "abc123def456" }),
    ], []);
    await page.goto("/topology", { waitUntil: "domcontentloaded" });

    const inspector = await open(page, "docker-dockerhost1-northserver");
    await expect(connection(inspector, "App Container")).toContainText("Hosts");
  });

  test("draws guests the hub infers from metadata under their host, not as their own tiles", async ({ page }) => {
    await mockTopologyData(page, [
      asset("docker-host-srv-1", "Docker Host", "container-host", "docker", { agent_id: "srv-1" }),
      asset("docker-vm-srv-1-a", "VM A", "vm", "docker", { agent_id: "srv-1" }),
      asset("docker-vm-srv-1-b", "VM B", "vm", "docker", { agent_id: "srv-1" }),
    ], []);
    await page.goto("/topology", { waitUntil: "domcontentloaded" });

    await expect(node(page, "Docker Host")).toBeVisible();
    await expect(guestRow(page, "VM A")).toBeVisible();
    await expect(guestRow(page, "VM B")).toBeVisible();
    await expect(page.locator(".mp-n", { hasText: "VM A" })).toHaveCount(0);
  });

  test("shows the same tree as a list from the page menu", async ({ page }) => {
    await mockTopologyData(page, [asset("asset-host-2", "Node Host", "host", "agent"), asset("asset-vm-2", "Node VM", "vm", "agent")],
      [dep("dep-runs-on-2", "asset-vm-2", "asset-host-2")]);
    await page.goto("/topology", { waitUntil: "domcontentloaded" });

    await page.getByRole("button", { name: "Map actions" }).click();
    await page.getByRole("menuitem", { name: "Show as list" }).click();
    const list = page.getByRole("navigation", { name: "Map as a list" });
    await expect(list.getByRole("link", { name: /Node Host/ })).toBeVisible();
    await expect(list.getByRole("link", { name: /Node VM/ })).toBeVisible();
  });

  test("lists a service that runs on a host in the host's Inspector", async ({ page }) => {
    await mockTopologyData(page, [asset("agent-host-3", "Lab Host", "host", "agent"), asset("agent-service-3", "Lab API", "service", "agent")],
      [dep("dep-runs-on-3", "agent-service-3", "agent-host-3")]);
    await page.goto("/topology", { waitUntil: "domcontentloaded" });

    const inspector = await open(page, "Lab Host");
    await expect(connection(inspector, "Lab API")).toContainText("Runs");
  });

  test("ignores malformed relationship rows", async ({ page }) => {
    await mockTopologyData(page, [asset("agent-host-4", "Lab Host 4", "host", "agent"), asset("agent-vm-4", "Lab VM 4", "vm", "agent")], [
      dep("dep-valid-4", "agent-vm-4", "agent-host-4"),
      { id: "dep-bad-4", source_asset_id: "agent-vm-4", target_asset_id: "agent-host-4", relationship_type: null } as unknown as TopologyDependency,
    ]);
    await page.goto("/topology", { waitUntil: "domcontentloaded" });

    await expect(node(page, "Lab Host 4")).toBeVisible();
    await expect(guestRow(page, "Lab VM 4")).toBeVisible();
  });

  test("keeps groups as zones and mixed-source devices with their guests on the map", async ({ page }) => {
    await installGroupTopologyWorkflowMocks(page, {
      groups: [
        { id: "group-home", name: "Home", slug: "home", sort_order: 0, created_at: BASE_TS, updated_at: BASE_TS },
        { id: "group-operations", name: "Operations", slug: "operations", sort_order: 1, created_at: BASE_TS, updated_at: BASE_TS },
      ],
      assets: [
        asset("proxmox-node-1", "Proxmox Cluster", "hypervisor-node", "proxmox", { node: "pve01" }),
        asset("proxmox-vm-201", "Kubernetes VM", "vm", "proxmox", { node: "pve01" }),
        asset("docker-host-1", "Docker Runtime", "container-host", "docker", { agent_id: "srv-ops-1" }),
        asset("docker-container-1", "API Container", "docker-container", "docker", { agent_id: "srv-ops-1", container_id: "abc123" }),
        asset("truenas-controller-1", "TrueNAS Core", "nas", "truenas", { collector_id: "collector-truenas-1" }),
        asset("truenas-dataset-1", "Media Dataset", "dataset", "truenas", { collector_id: "collector-truenas-1" }),
      ],
      dependencies: [
        dep("dep-proxmox-vm", "proxmox-vm-201", "proxmox-node-1"),
        dep("dep-docker-container", "docker-container-1", "docker-host-1", "hosted_on"),
        dep("dep-truenas-dataset", "truenas-controller-1", "truenas-dataset-1", "contains"),
      ],
    });
    await page.goto("/topology", { waitUntil: "domcontentloaded" });
    for (const id of ["proxmox-node-1", "docker-host-1", "truenas-controller-1"]) await assignDeviceToGroup(page, id, "group-operations");
    await page.reload({ waitUntil: "domcontentloaded" });

    for (const name of ["Proxmox Cluster", "Docker Runtime", "TrueNAS Core"]) await expect(node(page, name)).toBeVisible();
    await expect(guestRow(page, "Kubernetes VM")).toBeVisible();
    await expect(connection(await open(page, "Docker Runtime"), "API Container")).toContainText("Hosts");
    // The hub reports two relationships for this pair (the stored contains edge and the one the collector ID
    // implies); each is its own row, listed once.
    const media = connection(await open(page, "TrueNAS Core"), "Media Dataset");
    await expect(media).toHaveCount(2);
    await expect(media.filter({ hasText: "Hosts" })).toHaveCount(1);
    await expect(media.filter({ hasText: "Runs" })).toHaveCount(1);

    await page.getByRole("button", { name: "Map actions" }).click();
    await page.getByRole("menuitem", { name: "Zones", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "Name of Operations" })).toHaveValue("Operations");
  });
});
