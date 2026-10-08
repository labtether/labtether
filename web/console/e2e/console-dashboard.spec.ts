import { expect,test } from "@playwright/test";
import { mockConsoleBootstrap } from './helpers/consoleBootstrap';

test("dashboard fleet focus surfaces issue-heavy devices", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const groups = [
    { id: "group-lab", name: "Lab", slug: "lab", sort_order: 0, created_at: "2026-01-01T00:00:00.000Z", updated_at: "2026-01-01T00:00:00.000Z" },
    { id: "group-garage", name: "Garage", slug: "garage", sort_order: 1, created_at: "2026-01-01T00:00:00.000Z", updated_at: "2026-01-01T00:00:00.000Z" }
  ];
  const telemetryOverview = [
    {
      asset_id: "node-offline",
      name: "Lab Edge Offline",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "offline",
      platform: "linux",
      last_seen_at: new Date(now - 20 * 60_000).toISOString(),
      metrics: { cpu_used_percent: 15, memory_used_percent: 45, disk_used_percent: 52 }
    },
    {
      asset_id: "node-unresponsive",
      name: "Lab NAS Unresponsive",
      type: "host",
      source: "truenas",
      group_id: "group-garage",
      status: "unresponsive",
      platform: "freebsd",
      last_seen_at: new Date(now - 4 * 60_000).toISOString(),
      metrics: { cpu_used_percent: 40, memory_used_percent: 50, disk_used_percent: 62 }
    },
    {
      asset_id: "node-highload",
      name: "Lab Build Host",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metrics: { cpu_used_percent: 96, memory_used_percent: 82, disk_used_percent: 78 }
    }
  ];
  const assets = [
    {
      id: "node-offline",
      name: "Lab Edge Offline",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "offline",
      platform: "linux",
      last_seen_at: new Date(now - 20 * 60_000).toISOString(),
      metadata: {}
    },
    {
      id: "node-unresponsive",
      name: "Lab NAS Unresponsive",
      type: "host",
      source: "truenas",
      group_id: "group-garage",
      status: "unresponsive",
      platform: "freebsd",
      last_seen_at: new Date(now - 4 * 60_000).toISOString(),
      metadata: {}
    },
    {
      id: "node-highload",
      name: "Lab Build Host",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metadata: {}
    }
  ];

  const fullStatusPayload = {
    timestamp: new Date(now).toISOString(),
    summary: {
      servicesUp: 5,
      servicesTotal: 5,
      connectorCount: 2,
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
  };

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date(now).toISOString(),
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
      body: JSON.stringify(fullStatusPayload)
    });
  });

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Dashboard", level: 1, exact: true })).toBeVisible();

  await expect(page.getByRole("heading", { name: "Fleet Focus", level: 2, exact: true })).toBeVisible();
  await expect(page.getByText("Lab Edge Offline", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Lab NAS Unresponsive", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Lab Build Host", { exact: true }).first()).toBeVisible();
});

test("dashboard fleet focus can expand from issues to full fleet", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const telemetryOverview = [
    {
      asset_id: "node-offline",
      name: "Lab Edge Offline",
      type: "host",
      source: "agent",
      status: "offline",
      platform: "linux",
      last_seen_at: new Date(now - 20 * 60_000).toISOString(),
      metrics: { cpu_used_percent: 15, memory_used_percent: 45, disk_used_percent: 52 }
    },
    {
      asset_id: "node-healthy",
      name: "Lab Quiet Host",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 45_000).toISOString(),
      metrics: { cpu_used_percent: 24, memory_used_percent: 31, disk_used_percent: 42 }
    }
  ];
  const assets = telemetryOverview.map((asset) => ({
    id: asset.asset_id,
    name: asset.name,
    type: asset.type,
    source: asset.source,
    status: asset.status,
    platform: asset.platform,
    last_seen_at: asset.last_seen_at,
    metadata: {}
  }));

  const fullStatusPayload = {
    timestamp: new Date(now).toISOString(),
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
      staleAssetCount: 1
    },
    endpoints: [],
    connectors: [],
    groups: [],
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
  };

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date(now).toISOString(),
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 1
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
      body: JSON.stringify(fullStatusPayload)
    });
  });

  await page.goto("/");

  const fleetCard = page.locator(".xl\\:col-span-2");

  await expect(fleetCard.locator("a[href$='/nodes/node-offline']")).toHaveCount(1);
  await expect(fleetCard.locator("a[href$='/nodes/node-healthy']")).toHaveCount(0);

  await fleetCard.getByRole("button", { name: "Show all fleet", exact: true }).click();
  await expect(fleetCard.locator("a[href$='/nodes/node-healthy']")).toHaveCount(1);
  await expect(fleetCard.getByRole("button", { name: "Show only issues", exact: true })).toBeVisible();
});

test("dashboard fleet focus shows all devices when no fleet issues exist", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const telemetryOverview = [
    {
      asset_id: "node-alpha",
      name: "Lab Alpha",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metrics: { cpu_used_percent: 32, memory_used_percent: 48, disk_used_percent: 41 }
    },
    {
      asset_id: "node-beta",
      name: "Lab Beta",
      type: "host",
      source: "agent",
      status: "up",
      platform: "linux",
      last_seen_at: new Date(now - 45_000).toISOString(),
      metrics: { cpu_used_percent: 28, memory_used_percent: 36, disk_used_percent: 39 }
    },
    {
      asset_id: "node-gamma",
      name: "Lab Gamma",
      type: "host",
      source: "agent",
      status: "healthy",
      platform: "linux",
      last_seen_at: new Date(now - 60_000).toISOString(),
      metrics: { cpu_used_percent: 44, memory_used_percent: 52, disk_used_percent: 47 }
    }
  ];
  const assets = telemetryOverview.map((asset) => ({
    id: asset.asset_id,
    name: asset.name,
    type: asset.type,
    source: asset.source,
    status: asset.status,
    platform: asset.platform,
    last_seen_at: asset.last_seen_at,
    metadata: {}
  }));

  const fullStatusPayload = {
    timestamp: new Date(now).toISOString(),
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
  };

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date(now).toISOString(),
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0
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
      body: JSON.stringify(fullStatusPayload)
    });
  });

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Fleet Focus", level: 2, exact: true })).toBeVisible();
  await expect(page.getByText("Lab Alpha", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Lab Beta", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Lab Gamma", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("No devices found. Add your first device to get started.", { exact: true })).toHaveCount(0);
});
