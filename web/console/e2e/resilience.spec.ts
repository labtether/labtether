import { expect, test } from "@playwright/test";
import {
  buildLiveStatusPayload,
  buildStatusPayload,
  installConsoleApiMocks,
} from "./helpers/consoleApiMocks";

const BASE_TS = "2026-01-01T12:00:00.000Z";

function makePBSAsset() {
  return {
    id: "pbs-server-lab",
    type: "storage-controller",
    name: "Lab PBS",
    source: "pbs",
    status: "online",
    last_seen_at: BASE_TS,
    metadata: {
      collector_id: "collector-pbs-1",
    },
  };
}

test("expired deep-link session redirects to login and returns to target after sign-in", async ({ page }) => {
  let authenticated = false;
  const deepLink = "/nodes/node-1";

  const statusPayload = buildStatusPayload({
    assets: [
      {
        id: "node-1",
        type: "host",
        name: "node-1-host",
        source: "agent",
        status: "online",
        last_seen_at: BASE_TS,
      },
    ],
  });
  const liveStatusPayload = buildLiveStatusPayload({
    assets: statusPayload["assets"] as unknown[],
  });

  await installConsoleApiMocks(page, {
    statusPayload,
    liveStatusPayload,
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/auth/me") {
        if (!authenticated) {
          await fulfillJSON({ error: "unauthorized" }, 401);
        } else {
          await fulfillJSON({ user: { id: "owner", username: "admin", role: "owner" } }, 200);
        }
        return true;
      }
      if (pathname === "/api/auth/login" && method === "POST") {
        const username = String(requestBody.username ?? "");
        const password = String(requestBody.password ?? "");
        if (username === "admin" && password === "password") {
          authenticated = true;
          await fulfillJSON({ ok: true, user: { id: "owner", username: "admin", role: "owner" } }, 200);
        } else {
          await fulfillJSON({ error: "invalid credentials" }, 401);
        }
        return true;
      }
      return false;
    },
  });

  await page.goto(deepLink);
  await expect(page).toHaveURL(/\/login\?next=/);

  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();

  await expect(page).toHaveURL(/\/nodes\/node-1$/);
  await expect(
    page.locator("main").getByRole("heading", { name: "node-1-host", exact: true }).first(),
  ).toBeVisible();
});

test("login rejects unsafe next redirects and lands on root after sign-in", async ({ page }) => {
  await installConsoleApiMocks(page);

  await page.goto("/login?next=https%3A%2F%2Fevil.example%2Fsteal");
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();

  await expect(page).toHaveURL(/\/$/);
});

test("origin guard rejects cross-origin mutating requests", async ({ request }) => {
  const sessionCookie = "labtether_session=test-session";
  const crossOriginHeaders = {
    origin: "https://evil.example",
    "sec-fetch-site": "cross-site",
    cookie: sessionCookie,
  };

  const calls: Array<{
    method: "POST" | "PATCH" | "DELETE";
    path: string;
    data?: Record<string, unknown>;
  }> = [
    {
      method: "POST",
      path: "/api/auth/login",
      data: { username: "admin", password: "password" },
    },
    {
      method: "POST",
      path: "/api/services/web/sync",
      data: {},
    },
    {
      method: "PATCH",
      path: "/api/settings/runtime",
      data: { values: {} },
    },
    {
      method: "POST",
      path: "/api/services/web/overrides",
      data: { host_asset_id: "host-1", service_id: "svc-1", hidden: false },
    },
    {
      method: "DELETE",
      path: "/api/services/web/overrides?host=host-1&service_id=svc-1",
    },
  ];

  for (const call of calls) {
    const response = await request.fetch(call.path, {
      method: call.method,
      headers: crossOriginHeaders,
      data: call.data,
    });
    expect(response.status(), `${call.method} ${call.path}`).toBe(403);

    const payload = await response.json();
    expect(payload).toMatchObject({ error: "forbidden origin" });
  }
});

test("origin guard allows same-origin mutating requests to reach route handlers", async ({ request }) => {
  const sessionCookie = "labtether_session=test-session";
  const sameOrigin = process.env.PLAYWRIGHT_BASE_URL
    ?? (process.env.PLAYWRIGHT_SELF_SIGNED_HTTPS === "1"
      ? "https://127.0.0.1:4173"
      : "http://127.0.0.1:4173");
  const sameOriginHeaders = {
    origin: sameOrigin,
    "sec-fetch-site": "same-origin",
    cookie: sessionCookie,
  };

  const calls: Array<{
    method: "POST" | "PATCH" | "DELETE";
    path: string;
    data?: Record<string, unknown>;
  }> = [
    {
      method: "POST",
      path: "/api/auth/login",
      data: { username: "admin", password: "password" },
    },
    {
      method: "POST",
      path: "/api/services/web/sync",
      data: {},
    },
    {
      method: "PATCH",
      path: "/api/settings/runtime",
      data: { values: {} },
    },
    {
      method: "POST",
      path: "/api/services/web/overrides",
      data: { host_asset_id: "host-1", service_id: "svc-1", hidden: false },
    },
    {
      method: "DELETE",
      path: "/api/services/web/overrides?host=host-1&service_id=svc-1",
    },
    {
      method: "POST",
      path: "/api/admin/reset",
      data: { confirm: false },
    },
  ];

  for (const call of calls) {
    const response = await request.fetch(call.path, {
      method: call.method,
      headers: sameOriginHeaders,
      data: call.data,
    });
    expect(response.status(), `${call.method} ${call.path}`).not.toBe(403);
  }
});

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

test("services triage filters and unstable sorting prioritize actionable items", async ({ page }) => {
  const hostAssetID = "host-1";
  const now = new Date();
  const nowISO = now.toISOString();
  const staleChangeISO = new Date(now.getTime() - (5 * 24 * 60 * 60 * 1000)).toISOString();
  const recentChangeISO = new Date(now.getTime() - (30 * 60 * 1000)).toISOString();
  const statusPayload = buildStatusPayload({
    assets: [
      {
        id: hostAssetID,
        type: "host",
        name: "lab-host-1",
        source: "agent",
        status: "online",
        last_seen_at: BASE_TS,
      },
    ],
  });
  const liveStatusPayload = buildLiveStatusPayload({
    assets: statusPayload["assets"] as unknown[],
  });
  const services = [
    {
      id: "svc-healthy",
      service_key: "healthy",
      name: "Healthy Service",
      category: "Monitoring",
      url: "http://host-1:3000",
      source: "docker",
      status: "up",
      response_ms: 60,
      host_asset_id: hostAssetID,
      icon_key: "grafana",
      health: {
        window: "24h",
        checks: 24,
        up_checks: 24,
        uptime_percent: 100,
        last_checked_at: nowISO,
        last_change_at: staleChangeISO,
      },
    },
    {
      id: "svc-down",
      service_key: "down",
      name: "Down Service",
      category: "Monitoring",
      url: "http://host-1:9090",
      source: "docker",
      status: "down",
      response_ms: 0,
      host_asset_id: hostAssetID,
      icon_key: "prometheus",
      health: {
        window: "24h",
        checks: 24,
        up_checks: 17,
        uptime_percent: 70.8,
        last_checked_at: nowISO,
        last_change_at: staleChangeISO,
      },
    },
    {
      id: "svc-recent",
      service_key: "recent",
      name: "Recently Changed",
      category: "Monitoring",
      url: "http://host-1:8080",
      source: "scan",
      status: "up",
      response_ms: 115,
      host_asset_id: hostAssetID,
      icon_key: "globe",
      health: {
        window: "24h",
        checks: 18,
        up_checks: 16,
        uptime_percent: 88.9,
        last_checked_at: nowISO,
        last_change_at: recentChangeISO,
      },
    },
  ];

  await installConsoleApiMocks(page, {
    statusPayload,
    liveStatusPayload,
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/services/web" && method === "GET") {
        await fulfillJSON({ services, discovery_stats: [] }, 200);
        return true;
      }
      if (pathname === "/api/services/web/compat" && method === "GET") {
        await fulfillJSON({ compatible: [] }, 200);
        return true;
      }
      return false;
    },
  });

  await page.goto("/services");
  await expect(page.locator('[data-service-name="Healthy Service"]')).toHaveCount(1);

  await page.getByRole("button", { name: "Filters", exact: true }).click();
  await page.getByLabel("Health Filter").selectOption("unstable");
  await expect(page.locator('[data-service-name="Healthy Service"]')).toHaveCount(0);
  await expect(page.locator('[data-service-name="Down Service"]')).toHaveCount(1);
  await expect(page.locator('[data-service-name="Recently Changed"]')).toHaveCount(1);

  await page.getByLabel("Health Filter").selectOption("changed_recently");
  await expect(page.locator('[data-service-name="Recently Changed"]')).toHaveCount(1);
  await expect(page.locator('[data-service-name="Healthy Service"]')).toHaveCount(0);
  await expect(page.locator('[data-service-name="Down Service"]')).toHaveCount(0);

  await page.getByLabel("Health Filter").selectOption("all");
  await page.getByLabel("Sort Mode").selectOption("most_unstable");
  await expect(page.locator('[data-service-name="Down Service"]')).toHaveCount(1);
  const orderedNames = (await page.locator("[data-service-name]").allTextContents()).map((name) => name.trim());
  expect(orderedNames[0]).toBe("Down Service");
});

test("service detail health panel shows uptime history and recent checks", async ({ page }) => {
  const hostAssetID = "host-1";
  const now = new Date();
  const statusPayload = buildStatusPayload({
    assets: [
      {
        id: hostAssetID,
        type: "host",
        name: "lab-host-1",
        source: "agent",
        status: "online",
        last_seen_at: BASE_TS,
      },
    ],
  });
  const liveStatusPayload = buildLiveStatusPayload({
    assets: statusPayload["assets"] as unknown[],
  });
  const services = [
    {
      id: "svc-history",
      service_key: "grafana",
      name: "History Service",
      category: "Monitoring",
      url: "http://host-1:3000",
      source: "docker",
      status: "up",
      response_ms: 92,
      host_asset_id: hostAssetID,
      icon_key: "grafana",
      health: {
        window: "24h",
        checks: 12,
        up_checks: 9,
        uptime_percent: 75,
        last_checked_at: now.toISOString(),
        last_change_at: new Date(now.getTime() - (12 * 60 * 1000)).toISOString(),
        recent: [
          {
            at: new Date(now.getTime() - (55 * 60 * 1000)).toISOString(),
            status: "up",
            response_ms: 81,
          },
          {
            at: new Date(now.getTime() - (45 * 60 * 1000)).toISOString(),
            status: "down",
            response_ms: 0,
          },
          {
            at: new Date(now.getTime() - (35 * 60 * 1000)).toISOString(),
            status: "down",
            response_ms: 0,
          },
          {
            at: new Date(now.getTime() - (25 * 60 * 1000)).toISOString(),
            status: "up",
            response_ms: 134,
          },
          {
            at: new Date(now.getTime() - (18 * 60 * 1000)).toISOString(),
            status: "unknown",
            response_ms: 0,
          },
          {
            at: new Date(now.getTime() - (8 * 60 * 1000)).toISOString(),
            status: "up",
            response_ms: 92,
          },
        ],
      },
    },
  ];

  await installConsoleApiMocks(page, {
    statusPayload,
    liveStatusPayload,
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/services/web" && method === "GET") {
        await fulfillJSON({ services, discovery_stats: [] }, 200);
        return true;
      }
      if (pathname === "/api/services/web/compat" && method === "GET") {
        await fulfillJSON({ compatible: [] }, 200);
        return true;
      }
      return false;
    },
  });

  await page.goto("/services");
  await page.getByRole("button", { name: "Details", exact: true }).click();

  const healthPanel = page.getByLabel("Health History");
  await expect(healthPanel.getByText("Health History", { exact: true })).toBeVisible();
  await expect(healthPanel.getByText("75.0%", { exact: true })).toBeVisible();
  await expect(healthPanel.getByText("Availability Timeline", { exact: true })).toBeVisible();
  await expect(healthPanel.getByText("Recent Checks", { exact: true })).toBeVisible();
  await expect(healthPanel.getByText(/Last outage/i)).toBeVisible();
});

for (const statusCode of [401, 403, 429, 500]) {
  test(`pbs setup test-connection surfaces backend ${statusCode}`, async ({ page }) => {
    await installConsoleApiMocks(page, {
      customRoute: async ({ pathname, method, fulfillJSON }) => {
        if (pathname === "/api/settings/pbs/test" && method === "POST") {
          await fulfillJSON({ error: `pbs test failed (${statusCode})` }, statusCode);
          return true;
        }
        return false;
      },
    });

    await page.goto("/nodes");
    await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Add Device", exact: true }).click();
    await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

    await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
    await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
    await page.getByPlaceholder("Required for initial setup").fill("pbs-secret");
    await page.getByRole("button", { name: "Test Connection", exact: true }).click();

    await expect(page.getByText(`pbs test failed (${statusCode})`)).toBeVisible();
    await expect(page.getByText("Connect PBS")).toBeVisible();
  });
}

test("pbs setup redacts secret literals in test-connection errors", async ({ page }) => {
  const leakedSecret = "pbs-secret-should-never-render";

  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/settings/pbs/test" && method === "POST") {
        await fulfillJSON({ error: `pbs api returned 502: token_secret=${leakedSecret}` }, 502);
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

  await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
  await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
  await page.getByPlaceholder("Required for initial setup").fill(leakedSecret);
  await page.getByRole("button", { name: "Test Connection", exact: true }).click();

  await expect(page.getByText("token_secret=[redacted]")).toBeVisible();
  await expect(page.getByText(leakedSecret)).toHaveCount(0);
});

test("pbs setup surfaces offline network failures", async ({ page }) => {
  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, method, route }) => {
      if (pathname === "/api/settings/pbs/test" && method === "POST") {
        await route.abort("internetdisconnected");
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

  await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
  await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
  await page.getByPlaceholder("Required for initial setup").fill("pbs-secret");
  await page.getByRole("button", { name: "Test Connection", exact: true }).click();

  await expect(
    page.getByText(/ERR_INTERNET_DISCONNECTED|Failed to fetch|NetworkError when attempting to fetch resource|Load failed|failed to test pbs connection/i),
  ).toBeVisible();
});

test("pbs setup save failures keep the wizard open with actionable error", async ({ page }) => {
  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/settings/pbs" && method === "POST") {
        await fulfillJSON({ error: "failed to save pbs settings (500)" }, 500);
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

  await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
  await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
  await page.getByPlaceholder("Required for initial setup").fill("pbs-secret");
  await page.getByRole("button", { name: "Save, Sync & Close", exact: true }).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("failed to save pbs settings (500)")).toBeVisible();
  await expect(dialog.getByText("Connect PBS")).toBeVisible();
});

test("pbs setup recovers after transient test/save failures with retry", async ({ page }) => {
  let testCalls = 0;
  let saveCalls = 0;

  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, method, route, fulfillJSON }) => {
      if (pathname === "/api/settings/pbs/test" && method === "POST") {
        testCalls++;
        if (testCalls === 1) {
          await route.abort("internetdisconnected");
        } else {
          await fulfillJSON({ status: "ok", message: "pbs API reachable after retry" }, 200);
        }
        return true;
      }
      if (pathname === "/api/settings/pbs" && method === "POST") {
        saveCalls++;
        if (saveCalls === 1) {
          await fulfillJSON({ error: "temporary backend unavailable" }, 503);
        } else {
          await fulfillJSON({ collector_id: "collector-pbs-1" }, 200);
        }
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

  await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
  await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
  await page.getByPlaceholder("Required for initial setup").fill("pbs-secret");

  await page.getByRole("button", { name: "Test Connection", exact: true }).click();
  await expect(
    page.getByText(/ERR_INTERNET_DISCONNECTED|Failed to fetch|NetworkError when attempting to fetch resource|Load failed|failed to test pbs connection/i),
  ).toBeVisible();

  await page.getByRole("button", { name: "Test Connection", exact: true }).click();
  await expect(page.getByText("pbs API reachable after retry")).toBeVisible();

  await page.getByRole("button", { name: "Save, Sync & Close", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("temporary backend unavailable")).toBeVisible();
  await expect(dialog.getByText("Connect PBS")).toBeVisible();

  await page.getByRole("button", { name: "Save, Sync & Close", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("status websocket retries after connection failures", async ({ page, browserName }) => {
  let wsEndpointCalls = 0;

  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, fulfillJSON }) => {
      if (pathname === "/api/ws/events") {
        wsEndpointCalls++;
        await fulfillJSON({ wsUrl: "ws://127.0.0.1:9/ws/events" }, 200);
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();
  await page.waitForTimeout(browserName === "webkit" ? 10_500 : 7_500);

  expect(wsEndpointCalls).toBeGreaterThanOrEqual(browserName === "webkit" ? 1 : 2);
});

test("hidden-tab pauses fast polling and visible-tab resumes without duplicate loops", async ({ page, context }) => {
  let liveCalls = 0;

  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, fulfillJSON, route }) => {
      if (pathname === "/api/status/live") {
        liveCalls++;
        await fulfillJSON(
          buildLiveStatusPayload({
            assets: [],
            telemetryOverview: [],
          }),
        );
        return true;
      }
      if (pathname === "/api/ws/events") {
        await route.fulfill({ status: 204, body: "" });
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();
  await page.waitForTimeout(6200);

  const backgroundPage = await context.newPage();
  await backgroundPage.goto("about:blank");
  await backgroundPage.bringToFront();

  const beforeHidden = liveCalls;
  await page.waitForTimeout(6200);
  const hiddenDelta = liveCalls - beforeHidden;
  expect(hiddenDelta).toBeLessThanOrEqual(1);

  await page.bringToFront();
  const beforeVisible = liveCalls;
  await page.waitForTimeout(6200);
  const visibleDelta = liveCalls - beforeVisible;

  // One polling loop should produce roughly one refresh every ~5s.
  expect(visibleDelta).toBeGreaterThanOrEqual(1);
  expect(visibleDelta).toBeLessThanOrEqual(3);

  await backgroundPage.close();
});

test("nodes and logs stay responsive under high-volume payloads", async ({ page }) => {
  const assetCount = 1200;
  const assets = Array.from({ length: assetCount }, (_, index) => ({
    id: `load-node-${index}`,
    type: "host",
    name: `load-node-${index}`,
    source: "agent",
    status: "online",
    last_seen_at: BASE_TS,
    metadata: {
      cpu_percent: String((index * 7) % 100),
    },
  }));
  const telemetryOverview = assets.map((asset, index) => ({
    asset_id: asset.id,
    name: asset.name,
    type: asset.type,
    source: asset.source,
    status: "online",
    last_seen_at: asset.last_seen_at,
    metrics: {
      cpu_used_percent: (index * 7) % 100,
      memory_used_percent: (index * 5) % 100,
      disk_used_percent: (index * 3) % 100,
    },
  }));
  const allLogEvents = Array.from({ length: 5000 }, (_, index) => ({
    id: `log-${index}`,
    source: "agent",
    level: index % 50 === 0 ? "error" : "info",
    message: `high-volume-event-${index}`,
    timestamp: BASE_TS,
  }));

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets,
      telemetryOverview,
      logSources: [{ source: "agent", count: allLogEvents.length, last_seen_at: BASE_TS }],
      recentLogs: allLogEvents.slice(0, 120),
    }),
    liveStatusPayload: buildLiveStatusPayload({
      assets,
      telemetryOverview,
    }),
    customRoute: async ({ pathname, url, fulfillJSON }) => {
      if (pathname === "/api/logs/query") {
        const query = (url.searchParams.get("q") ?? "").trim().toLowerCase();
        const events = query
          ? allLogEvents.filter((entry) => entry.message.toLowerCase().includes(query))
          : allLogEvents;
        await fulfillJSON({ events: events.slice(0, 300) }, 200);
        return true;
      }
      return false;
    },
  });

  const nodesStarted = Date.now();
  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();
  expect(Date.now() - nodesStarted).toBeLessThan(7000);

  await page.getByPlaceholder("Search devices...").fill("load-node-1199");
  await expect(
    page.locator("[role='link']").filter({ hasText: "load-node-1199" }).first(),
  ).toBeVisible({ timeout: 3000 });

  const logsStarted = Date.now();
  await page.goto("/logs");
  await expect(page.getByRole("heading", { name: "Logs", level: 1, exact: true })).toBeVisible();
  expect(Date.now() - logsStarted).toBeLessThan(7000);

  await page.getByPlaceholder("Search message text (error, timeout, zfs...)").fill("high-volume-event-4999");
  await expect(page.getByText("high-volume-event-4999")).toBeVisible({ timeout: 3000 });
});

test("pbs task lifecycle works in node detail (status, log, stop)", async ({ page }) => {
  const pbsAsset = makePBSAsset();
  const taskUPID = "UPID:localhost:00000001:00000002:backup:vm/100:root@pam:";
  let stopCalls = 0;

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets: [pbsAsset],
    }),
    liveStatusPayload: buildLiveStatusPayload({
      assets: [pbsAsset],
    }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === `/api/pbs/assets/${encodeURIComponent(pbsAsset.id)}/details`) {
        await fulfillJSON({
          asset_id: pbsAsset.id,
          kind: "server",
          collector_id: "collector-pbs-1",
          node: "localhost",
          version: "3.2.5",
          datastores: [
            {
              store: "backup",
              status: "ok",
              usage_percent: 55.2,
              used_bytes: 552,
              total_bytes: 1000,
              group_count: 4,
              snapshot_count: 22,
              last_backup_at: BASE_TS,
            },
          ],
          tasks: [
            {
              upid: taskUPID,
              node: "localhost",
              worker_type: "backup",
              worker_id: "vm/100",
              status: "running",
              starttime: 1_704_110_400,
            },
          ],
          warnings: [],
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/status")) {
        await fulfillJSON({
          task: {
            upid: taskUPID,
            node: "localhost",
            status: "stopped",
            exitstatus: "OK",
          },
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/log")) {
        await fulfillJSON({
          lines: [
            { n: 1, t: "start backup job" },
            { n: 2, t: "backup complete" },
          ],
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/stop") && method === "POST") {
        stopCalls++;
        await fulfillJSON({ ok: true });
        return true;
      }
      return false;
    },
  });

  await page.goto(`/nodes/${encodeURIComponent(pbsAsset.id)}?panel=pbs`);
  await expect(page.locator("main").getByRole("heading", { name: "Lab PBS", exact: true }).first()).toBeVisible();
  await expect(page.getByText("Recent PBS Tasks")).toBeVisible();

  await page.getByRole("button", { name: /backup/i }).first().click();
  await expect(page.getByText("Task Details")).toBeVisible();
  await expect(page.getByText("start backup job")).toBeVisible();

  await page.getByRole("button", { name: "Stop Task", exact: true }).click();
  await expect.poll(() => stopCalls).toBeGreaterThanOrEqual(1);
});

test("pbs task lifecycle surfaces status/log backend errors in node detail", async ({ page }) => {
  const pbsAsset = makePBSAsset();
  const taskUPID = "UPID:localhost:00000001:00000002:backup:vm/100:root@pam:";

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets: [pbsAsset],
    }),
    liveStatusPayload: buildLiveStatusPayload({
      assets: [pbsAsset],
    }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === `/api/pbs/assets/${encodeURIComponent(pbsAsset.id)}/details`) {
        await fulfillJSON({
          asset_id: pbsAsset.id,
          kind: "server",
          collector_id: "collector-pbs-1",
          node: "localhost",
          tasks: [
            {
              upid: taskUPID,
              node: "localhost",
              worker_type: "backup",
              worker_id: "vm/100",
              status: "running",
              starttime: 1_704_110_400,
            },
          ],
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/status")) {
        await fulfillJSON({ error: "status failed" }, 500);
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/log")) {
        await fulfillJSON({ error: "log failed" }, 500);
        return true;
      }
      return false;
    },
  });

  await page.goto(`/nodes/${encodeURIComponent(pbsAsset.id)}?panel=pbs`);
  await expect(page.locator("main").getByRole("heading", { name: "Lab PBS", exact: true }).first()).toBeVisible();
  await expect(page.getByText("Recent PBS Tasks")).toBeVisible();

  const backupTaskButton = page.getByRole("button", { name: /backup/i }).first();
  await expect(backupTaskButton).toBeVisible();
  await backupTaskButton.click();
  await expect(page.getByText("Task Details")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("status failed")).toBeVisible({ timeout: 10_000 });
});

test("pbs task lifecycle handles stop backend 500 without breaking task panel", async ({ page }) => {
  const pbsAsset = makePBSAsset();
  const taskUPID = "UPID:localhost:00000001:00000002:backup:vm/100:root@pam:";
  const stopStatuses: number[] = [];

  page.on("response", (response) => {
    if (response.url().includes("/api/pbs/tasks/") && response.url().includes("/stop")) {
      stopStatuses.push(response.status());
    }
  });

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets: [pbsAsset],
    }),
    liveStatusPayload: buildLiveStatusPayload({
      assets: [pbsAsset],
    }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === `/api/pbs/assets/${encodeURIComponent(pbsAsset.id)}/details`) {
        await fulfillJSON({
          asset_id: pbsAsset.id,
          kind: "server",
          collector_id: "collector-pbs-1",
          node: "localhost",
          tasks: [
            {
              upid: taskUPID,
              node: "localhost",
              worker_type: "backup",
              worker_id: "vm/100",
              status: "running",
              starttime: 1_704_110_400,
            },
          ],
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/status")) {
        await fulfillJSON({
          task: {
            upid: taskUPID,
            node: "localhost",
            status: "running",
            exitstatus: "n/a",
          },
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/log")) {
        await fulfillJSON({
          lines: [{ n: 1, t: "task still running" }],
        });
        return true;
      }
      if (pathname.includes("/api/pbs/tasks/") && pathname.endsWith("/stop")) {
        await fulfillJSON({ error: "stop failed" }, 500);
        return true;
      }
      return false;
    },
  });

  await page.goto(`/nodes/${encodeURIComponent(pbsAsset.id)}?panel=pbs`);
  await expect(page.locator("main").getByRole("heading", { name: "Lab PBS", exact: true }).first()).toBeVisible();

  const backupTaskButton = page.getByRole("button", { name: /backup/i }).first();
  const taskDetailsHeading = page.getByText("Task Details");
  let taskDetailsVisible = false;
  for (let attempt = 0; attempt < 3; attempt += 1) {
    await backupTaskButton.click();
    try {
      await expect(taskDetailsHeading).toBeVisible({ timeout: 2_500 });
      taskDetailsVisible = true;
      break;
    } catch {
      // Retry selection when list rerenders race with the first click.
    }
  }
  expect(taskDetailsVisible).toBe(true);
  await expect(page.getByText("task still running")).toBeVisible({ timeout: 10_000 });

  await page.getByRole("button", { name: "Stop Task", exact: true }).click();
  await page.waitForTimeout(750);
  if (stopStatuses.length > 0) {
    expect(stopStatuses.at(-1)).toBe(500);
  }
  await expect(page.getByRole("button", { name: "Stop Task", exact: true })).toBeEnabled();
  await expect(page.getByText("Task Details")).toBeVisible();
});

test("terminal reconnect reuses prior session id after abnormal disconnect", async ({ page }) => {
  const terminalAsset = {
    id: "node-1",
    type: "host",
    name: "node-1-host",
    source: "agent",
    status: "online",
    last_seen_at: BASE_TS,
  };
  const workspaceTab = {
    id: "tab-1",
    name: "Default",
    layout: "single",
    panes: [{ targetNodeId: terminalAsset.id }],
    sort_order: 0,
  };

  let sessionCreateCalls = 0;
  const streamTicketSessionIds: string[] = [];

  await page.addInitScript(() => {
    let connectCount = 0;

    class MockTerminalWebSocket {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSING = 2;
      static CLOSED = 3;

      readyState = MockTerminalWebSocket.CONNECTING;
      binaryType: string = "arraybuffer";
      onopen: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;
      onclose: ((event: CloseEvent) => void) | null = null;

      constructor(_url: string) {
        connectCount += 1;
        const attempt = connectCount;

        setTimeout(() => {
          this.readyState = MockTerminalWebSocket.OPEN;
          this.onopen?.(new Event("open"));

          if (attempt === 1) {
            setTimeout(() => {
              this.readyState = MockTerminalWebSocket.CLOSED;
              this.onclose?.({
                code: 1011,
                reason: "upstream reset",
                wasClean: false,
              } as CloseEvent);
            }, 10);
          }
        }, 0);
      }

      send(_data: unknown) {}

      close(code = 1000, reason = "") {
        this.readyState = MockTerminalWebSocket.CLOSED;
        this.onclose?.({
          code,
          reason,
          wasClean: code === 1000,
        } as CloseEvent);
      }
    }

    (window as unknown as { WebSocket: unknown }).WebSocket = MockTerminalWebSocket;
  });

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets: [terminalAsset],
    }),
    liveStatusPayload: buildLiveStatusPayload({
      assets: [terminalAsset],
    }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({});
        return true;
      }
      if (pathname === "/api/terminal/snippets" && method === "GET") {
        await fulfillJSON({ snippets: [] });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "GET") {
        await fulfillJSON({ tabs: [workspaceTab] });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "POST") {
        await fulfillJSON({ tab: workspaceTab });
        return true;
      }
      if (pathname === `/api/terminal/workspace/tabs/${workspaceTab.id}` && method === "PUT") {
        await fulfillJSON({ tab: workspaceTab });
        return true;
      }
      if (pathname === "/api/terminal/session" && method === "POST") {
        sessionCreateCalls += 1;
        await fulfillJSON({ session: { id: "term-session-1" } });
        return true;
      }
      if (pathname === "/api/terminal/stream-ticket" && method === "POST") {
        streamTicketSessionIds.push(String(requestBody.sessionId ?? ""));
        await fulfillJSON({ wsUrl: "ws://terminal.mock/session" });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");
  await expect.poll(() => streamTicketSessionIds.length).toBe(1);
  await expect(page.getByText("Session disconnected", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Reconnect", exact: true }).click();

  await expect.poll(() => streamTicketSessionIds.length).toBe(2);
  expect(sessionCreateCalls).toBe(1);
  expect(streamTicketSessionIds).toEqual(["term-session-1", "term-session-1"]);
});
