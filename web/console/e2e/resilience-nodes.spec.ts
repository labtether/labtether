import { expect,test } from "@playwright/test";
import {
buildLiveStatusPayload,
buildStatusPayload,
installConsoleApiMocks,
} from "./helpers/consoleApiMocks";
import { BASE_TS } from './helpers/resilienceMocks';

test("node system panel deep drilldown routes and returns to overview", async ({ page }) => {
  const nodeID = "node-drilldown";
  const telemetryDetails = {
    asset: {
      id: nodeID,
      name: "drilldown-node",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: BASE_TS,
    },
    window: "1h",
    step: "5m",
    from: "2026-01-01T11:00:00.000Z",
    to: BASE_TS,
    series: [
      {
        metric: "cpu_used_percent",
        unit: "percent",
        current: 27.5,
        points: [
          { ts: 1735729200, value: 18 },
          { ts: 1735729500, value: 24 },
          { ts: 1735729800, value: 31 },
          { ts: 1735730100, value: 27.5 },
        ],
      },
      {
        metric: "memory_used_percent",
        unit: "percent",
        current: 63.2,
        points: [
          { ts: 1735729200, value: 58 },
          { ts: 1735729500, value: 60 },
          { ts: 1735729800, value: 61.8 },
          { ts: 1735730100, value: 63.2 },
        ],
      },
      {
        metric: "disk_used_percent",
        unit: "percent",
        current: 44.1,
        points: [
          { ts: 1735729200, value: 42.5 },
          { ts: 1735729500, value: 43.1 },
          { ts: 1735729800, value: 43.8 },
          { ts: 1735730100, value: 44.1 },
        ],
      },
      {
        metric: "network_rx_bytes_per_sec",
        unit: "bytes_per_sec",
        current: 2048,
        points: [
          { ts: 1735729200, value: 512 },
          { ts: 1735729500, value: 768 },
          { ts: 1735729800, value: 1536 },
          { ts: 1735730100, value: 2048 },
        ],
      },
      {
        metric: "network_tx_bytes_per_sec",
        unit: "bytes_per_sec",
        current: 1024,
        points: [
          { ts: 1735729200, value: 256 },
          { ts: 1735729500, value: 512 },
          { ts: 1735729800, value: 768 },
          { ts: 1735730100, value: 1024 },
        ],
      },
    ],
  };
  const statusPayload = buildStatusPayload({
    assets: [
      {
        id: nodeID,
        type: "host",
        name: "drilldown-node",
        source: "agent",
        status: "online",
        last_seen_at: BASE_TS,
        platform: "linux",
        metadata: {
          cpu_model: "AMD Ryzen 9",
          cpu_threads_logical: "24",
          cpu_cores_physical: "12",
          cpu_max_mhz: "5400",
          memory_total_bytes: String(64 * 1024 * 1024 * 1024),
          memory_available_bytes: String(24 * 1024 * 1024 * 1024),
          swap_total_bytes: String(8 * 1024 * 1024 * 1024),
          swap_used_bytes: String(1 * 1024 * 1024 * 1024),
          disk_root_total_bytes: String(1024 * 1024 * 1024 * 1024),
          disk_root_available_bytes: String(512 * 1024 * 1024 * 1024),
          backup_state: "ok",
          days_since_backup: "1",
          network_default_gateway: "192.168.1.1",
          network_dns_servers: "192.168.1.1, 1.1.1.1",
          tailscale_backend_state: "running",
          tailscale_tailnet: "labtail.ts.net",
          network_interface_count: "3",
        },
      },
    ],
    telemetryOverview: [
      {
        asset_id: nodeID,
        metrics: {
          cpu_used_percent: 27.5,
          memory_used_percent: 63.2,
          disk_used_percent: 44.1,
          network_rx_bytes_per_sec: 2048,
          network_tx_bytes_per_sec: 1024,
        },
      },
    ],
  });
  const liveStatusPayload = buildLiveStatusPayload({
    assets: statusPayload["assets"] as unknown[],
    telemetryOverview: statusPayload["telemetryOverview"] as unknown[],
  });

  await installConsoleApiMocks(page, {
    statusPayload,
    liveStatusPayload,
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === `/api/metrics/assets/${encodeURIComponent(nodeID)}` && method === "GET") {
        await fulfillJSON(telemetryDetails, 200);
        return true;
      }
      if (pathname === `/api/network/${encodeURIComponent(nodeID)}` && method === "GET") {
        await fulfillJSON({
          interfaces: [
            {
              name: "eth0",
              state: "up",
              mac: "00:11:22:33:44:55",
              mtu: 1500,
              ips: ["192.168.1.20"],
              rx_bytes: 2_400_000,
              tx_bytes: 1_300_000,
              rx_packets: 2400,
              tx_packets: 1500,
            },
            {
              name: "tailscale0",
              state: "up",
              mac: "",
              mtu: 1280,
              ips: ["100.64.0.20"],
              rx_bytes: 900_000,
              tx_bytes: 500_000,
              rx_packets: 900,
              tx_packets: 520,
            },
          ],
        }, 200);
        return true;
      }
      return false;
    },
  });

  await page.goto(`/nodes/${encodeURIComponent(nodeID)}?panel=system`);
  await expect(page.getByRole("heading", { name: "drilldown-node", exact: true }).first()).toBeVisible();

  await page.getByRole("button", { name: /^CPU/i }).click();
  await expect(page).toHaveURL(new RegExp(`/nodes/${encodeURIComponent(nodeID)}\\?panel=system&detail=cpu$`));
  await expect(page.getByText("CPU Deep Detail")).toBeVisible();
  await expect(page.getByText("Live Pressure")).toBeVisible();
  await expect(page.getByText("Compute Topology")).toBeVisible();
  await expect(page.getByText("Switch Detail")).toBeVisible();
  await expect(page.getByText("Historical Context")).toBeVisible();
  await expect(page.getByText("CPU Usage", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Open Metrics", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Memory", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/nodes/${encodeURIComponent(nodeID)}\\?panel=system&detail=memory$`));
  await expect(page.getByText("Memory Deep Detail")).toBeVisible();
  await expect(page.getByText("Capacity Snapshot")).toBeVisible();
  await expect(page.getByText("Swap And Pressure")).toBeVisible();
  await expect(page.getByText("Memory Usage", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Storage", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/nodes/${encodeURIComponent(nodeID)}\\?panel=system&detail=storage$`));
  await expect(page.getByText("Storage Deep Detail")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Filesystem Capacity", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Operational Signals", exact: true })).toBeVisible();
  await expect(page.getByText("Disk Usage", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Back to Overview", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/nodes/${encodeURIComponent(nodeID)}\\?panel=system$`));
  await expect(page.getByText("Storage Deep Detail")).toHaveCount(0);

  await page.goto(`/nodes/${encodeURIComponent(nodeID)}?panel=system&detail=network`);
  await expect(page.getByText("Network Deep Detail")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Addressing", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Overlay And Traffic", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Investigation Flow", exact: true })).toBeVisible();
  await expect(page.getByText("Network RX", { exact: true })).toBeVisible();
  await expect(page.getByText("Network TX", { exact: true })).toBeVisible();
  await expect(page.getByText("Top Interface Activity")).toBeVisible();
  await expect(page.getByText("eth0", { exact: true })).toBeVisible();
  await expect(page.getByText("tailscale0", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Open Interfaces", exact: true })).toBeVisible();
});
