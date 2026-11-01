import { expect,test } from "@playwright/test";
import { mockConsoleBootstrap } from './helpers/consoleBootstrap';

// Promise: a device that stops answering shows on the Dashboard under Needs attention, and a healthy one does not.
test("dashboard lists devices that stop answering under Needs attention", async ({ page }) => {
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

  const needs = page.getByRole("region", { name: "Needs attention", exact: true });
  await expect(needs.getByText("Lab Edge Offline is offline", { exact: true })).toBeVisible();
  await expect(needs.getByText("Lab NAS Unresponsive is not responding", { exact: true })).toBeVisible();
  await expect(needs.getByText(/Lab Build Host/)).toHaveCount(0);
});
