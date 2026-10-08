import { expect,test } from "@playwright/test";
import { mockConsoleBootstrap } from './helpers/consoleBootstrap';

test("dashboard topology hero excludes service assets and keeps infrastructure", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const assets = [
    {
      id: "node-host-a",
      name: "Host A",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metadata: {}
    },
    {
      id: "svc-api-a",
      name: "svc-a",
      type: "service",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 45_000).toISOString(),
      metadata: {}
    }
  ];
  const telemetryOverview = [
    {
      asset_id: "node-host-a",
      name: "Host A",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metrics: { cpu_used_percent: 35, memory_used_percent: 40, disk_used_percent: 42 }
    },
    {
      asset_id: "svc-api-a",
      name: "svc-a",
      type: "service",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 45_000).toISOString(),
      metrics: { cpu_used_percent: 0, memory_used_percent: 0, disk_used_percent: 0 }
    }
  ];

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
  await expect(page.getByText("Host A", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("svc-a", { exact: true })).toHaveCount(0);
});

test("dashboard topology hero avoids long baseline edges in sparse infra layouts", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const assets = [
    {
      id: "infra-alpha",
      name: "A-Infra",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metadata: {}
    },
    {
      id: "infra-bravo",
      name: "B-Infra",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 40_000).toISOString(),
      metadata: {}
    },
    {
      id: "infra-charlie",
      name: "C-Infra",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 50_000).toISOString(),
      metadata: {}
    }
  ];

  const telemetryOverview = assets.map((asset, index) => ({
    asset_id: asset.id,
    name: asset.name,
    type: asset.type,
    source: asset.source,
    status: asset.status,
    platform: asset.platform,
    last_seen_at: asset.last_seen_at,
    metrics: {
      cpu_used_percent: 20 + index * 5,
      memory_used_percent: 30 + index * 5,
      disk_used_percent: 40 + index * 5
    }
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
  await expect(page.locator(".react-flow__edge-path")).toHaveCount(2);

  const hasLongHorizontalEdge = await page.evaluate(() => {
    const paths = Array.from(document.querySelectorAll<SVGPathElement>(".react-flow__edge-path"));
    return paths.some((path) => {
      const d = path.getAttribute("d");
      if (!d) return false;
      const numbers = d.match(/-?\d*\.?\d+/g)?.map((value) => Number(value)) ?? [];
      if (numbers.length < 4) return false;
      const x1 = numbers[0];
      const y1 = numbers[1];
      const x2 = numbers[numbers.length - 2];
      const y2 = numbers[numbers.length - 1];
      return Math.abs(y2 - y1) < 8 && Math.abs(x2 - x1) > 220;
    });
  });

  expect(hasLongHorizontalEdge).toBeFalsy();
});
