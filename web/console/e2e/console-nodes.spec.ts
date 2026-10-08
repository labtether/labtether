import { expect,test } from "@playwright/test";
import { mockConsoleBootstrap } from './helpers/consoleBootstrap';

test("nodes page renders canonical kind labels when available", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const assets = [
    {
      id: "truenas-host-omega",
      name: "OmegaNAS",
      type: "nas",
      source: "truenas",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        hostname: "OmegaNAS"
      },
      resource_class: "storage",
      resource_kind: "storage-controller",
      attributes: {
        source: "truenas",
        hostname: "OmegaNAS"
      }
    }
  ];

  await page.route("**/api/status/live", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0
        },
        endpoints: [],
        assets,
        telemetryOverview: []
      })
    });
  });

  await page.route("**/api/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          connectorCount: 1,
          groupCount: 0,
          assetCount: assets.length,
          sessionCount: 0,
          auditCount: 0,
          processedJobs: 0,
          actionRunCount: 0,
          updateRunCount: 0,
          deadLetterCount: 0,
          staleAssetCount: 0
        },
        endpoints: [],
        connectors: [],
        groups: [],
        assets,
        telemetryOverview: [],
        recentLogs: [],
        logSources: [],
        groupReliability: [],
        actionRuns: [],
        updatePlans: [],
        updateRuns: [],
        deadLetters: [],
        deadLetterAnalytics: {
          window: "24h",
          bucket: "1h",
          total: 0,
          rate_per_hour: 0,
          rate_per_day: 0,
          trend: [],
          top_components: [],
          top_subjects: [],
          top_error_classes: []
        },
        sessions: [],
        recentCommands: [],
        recentAudit: []
      })
    });
  });

  await page.goto("/nodes", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  const row = page
    .locator("[role='link']")
    .filter({ has: page.getByText("OmegaNAS", { exact: true }) })
    .first();
  await expect(row).toContainText("Storage Controller");
});

test("node metrics tolerates telemetry series with null points", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const asset = {
    id: "docker-ct-1",
    name: "App Container",
    type: "docker-container",
    source: "docker",
    status: "online",
    platform: "linux",
    last_seen_at: now,
    metadata: {
      hostname: "app-container",
    },
  };

  const pageErrors: string[] = [];
  page.on("pageerror", (error) => {
    pageErrors.push(error.message);
  });

  await page.route("**/api/status/live", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: 1,
          staleAssetCount: 0,
        },
        endpoints: [],
        assets: [asset],
        telemetryOverview: [],
      }),
    });
  });

  await page.route("**/api/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          connectorCount: 0,
          groupCount: 0,
          assetCount: 1,
          sessionCount: 0,
          auditCount: 0,
          processedJobs: 0,
          actionRunCount: 0,
          updateRunCount: 0,
          deadLetterCount: 0,
          staleAssetCount: 0,
        },
        endpoints: [],
        connectors: [],
        groups: [],
        assets: [asset],
        telemetryOverview: [],
        recentLogs: [],
        logSources: [],
        groupReliability: [],
        actionRuns: [],
        updatePlans: [],
        updateRuns: [],
        deadLetters: [],
        deadLetterAnalytics: {
          window: "24h",
          bucket: "1h",
          total: 0,
          rate_per_hour: 0,
          rate_per_day: 0,
          trend: [],
          top_components: [],
          top_subjects: [],
          top_error_classes: [],
        },
        sessions: [],
        recentCommands: [],
        recentAudit: [],
      }),
    });
  });

  await page.route(/\/api\/metrics\/assets\/docker-ct-1(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        asset: {
          id: asset.id,
          name: asset.name,
          type: asset.type,
          source: asset.source,
          status: asset.status,
          platform: asset.platform,
          last_seen_at: asset.last_seen_at,
        },
        window: "1h",
        step: "1m",
        from: "2026-03-08T00:00:00.000Z",
        to: "2026-03-08T01:00:00.000Z",
        series: [
          {
            metric: "cpu_used_percent",
            unit: "percent",
            points: null,
            current: 62,
          },
          {
            metric: "memory_used_percent",
            unit: "percent",
            points: [
              { ts: 1_741_392_000, value: 41 },
              { ts: 1_741_392_060, value: 44 },
            ],
            current: 44,
          },
        ],
      }),
    });
  });

  await page.goto("/nodes/docker-ct-1?panel=monitoring", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "Metrics History", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "CPU Usage", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Memory Usage", exact: true })).toBeVisible();
  await expect(page.getByText("No data points in this window.", { exact: true })).toBeVisible();
  expect(pageErrors).toEqual([]);
});

test("nodes page search narrows visible device cards", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const groups = [
    { id: "group-lab", name: "Lab", slug: "lab", sort_order: 0, created_at: "2026-01-01T00:00:00.000Z", updated_at: "2026-01-01T00:00:00.000Z" },
    { id: "group-garage", name: "Garage", slug: "garage", sort_order: 1, created_at: "2026-01-01T00:00:00.000Z", updated_at: "2026-01-01T00:00:00.000Z" }
  ];
  const assets: Array<{
    id: string;
    name: string;
    type: string;
    source: string;
    group_id?: string;
    status: string;
    platform: string;
    last_seen_at: string;
    metadata: Record<string, string>;
  }> = [
    {
      id: "node-a",
      name: "Node Alpha",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metadata: {}
    },
    {
      id: "node-b",
      name: "Node Beta",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "unresponsive",
      platform: "linux",
      last_seen_at: new Date(now - 2 * 60_000).toISOString(),
      metadata: {}
    },
    {
      id: "node-c",
      name: "Node Gamma",
      type: "host",
      source: "agent",
      status: "offline",
      platform: "linux",
      last_seen_at: new Date(now - 15 * 60_000).toISOString(),
      metadata: {}
    }
  ];

  const telemetryOverview = assets.map((asset) => ({
    asset_id: asset.id,
    name: asset.name,
    type: asset.type,
    source: asset.source,
    group_id: asset.group_id,
    status: asset.status,
    platform: asset.platform,
    last_seen_at: asset.last_seen_at,
    metrics: {
      cpu_used_percent: asset.id === "node-a" ? 34 : asset.id === "node-b" ? 71 : 22,
      memory_used_percent: asset.id === "node-a" ? 48 : asset.id === "node-b" ? 76 : 58,
      disk_used_percent: asset.id === "node-a" ? 44 : asset.id === "node-b" ? 73 : 66
    }
  }));

  const fullStatusPayload = () => ({
    timestamp: new Date().toISOString(),
    summary: {
      servicesUp: 5,
      servicesTotal: 5,
      connectorCount: 1,
      groupCount: groups.length,
      assetCount: assets.length,
      sessionCount: 0,
      auditCount: 0,
      processedJobs: 0,
      actionRunCount: 0,
      updateRunCount: 0,
      deadLetterCount: 0,
      staleAssetCount: 2
    },
    endpoints: [],
    connectors: [],
    groups,
    assets,
    telemetryOverview,
    recentLogs: [],
    logSources: [],
    groupReliability: [],
    actionRuns: [],
    updatePlans: [],
    updateRuns: [],
    deadLetters: [],
    deadLetterAnalytics: {
      window: "24h",
      bucket: "1h",
      total: 0,
      rate_per_hour: 0,
      rate_per_day: 0,
      trend: [],
      top_components: [],
      top_subjects: [],
      top_error_classes: []
    },
    sessions: [],
    recentCommands: [],
    recentAudit: []
  });

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date().toISOString(),
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 2
        },
        endpoints: [],
        assets,
        telemetryOverview
      })
    });
  });

  await page.route(/\/api\/status(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fullStatusPayload())
    });
  });

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  const search = page.getByLabel("Search devices");
  await search.fill("Node Beta");
  await expect(page.locator("[role='link']").filter({ hasText: "Node Beta" })).toHaveCount(1);
  await expect(page.locator("[role='link']").filter({ hasText: "Node Alpha" })).toHaveCount(0);
  await expect(page.locator("[role='link']").filter({ hasText: "Node Gamma" })).toHaveCount(0);

  await search.fill("");
  await page.getByRole("button", { name: "Expand", exact: true }).click();
  await expect(page.locator("[role='link']").filter({ hasText: "Node Alpha" })).toHaveCount(1);
  await expect(page.locator("[role='link']").filter({ hasText: "Node Beta" })).toHaveCount(1);
  await expect(page.locator("[role='link']").filter({ hasText: "Node Gamma" })).toHaveCount(1);
});

test("device card navigation uses a single history entry per click", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const assets = [
    {
      id: "node-alpha",
      name: "Node Alpha",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: now,
      metadata: {
        hostname: "node-alpha",
      },
    },
    {
      id: "node-beta",
      name: "Node Beta",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: now,
      metadata: {
        hostname: "node-beta",
      },
    },
  ];

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0,
        },
        endpoints: [],
        assets,
        telemetryOverview: [],
      }),
    });
  });

  await page.route(/\/api\/status(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          connectorCount: 0,
          groupCount: 0,
          assetCount: assets.length,
          sessionCount: 0,
          auditCount: 0,
          processedJobs: 0,
          actionRunCount: 0,
          updateRunCount: 0,
          deadLetterCount: 0,
          staleAssetCount: 0,
        },
        endpoints: [],
        connectors: [],
        groups: [],
        assets,
        telemetryOverview: [],
        recentLogs: [],
        logSources: [],
        groupReliability: [],
        actionRuns: [],
        updatePlans: [],
        updateRuns: [],
        deadLetters: [],
        deadLetterAnalytics: {
          window: "24h",
          bucket: "1h",
          total: 0,
          rate_per_hour: 0,
          rate_per_day: 0,
          trend: [],
          top_components: [],
          top_subjects: [],
          top_error_classes: [],
        },
        sessions: [],
        recentCommands: [],
        recentAudit: [],
      }),
    });
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.locator("[role='link']").filter({ hasText: "Node Alpha" }).first().click();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/node-alpha$/);
  await expect(page.getByText("Node Alpha", { exact: true })).toBeVisible();

  await page.goBack();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes$/);
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.locator("[role='link']").filter({ hasText: "Node Beta" }).first().click();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/node-beta$/);
  await expect(page.getByText("Node Beta", { exact: true })).toBeVisible();

  await page.goBack();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes$/);
});
