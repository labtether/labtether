import { expect,test } from "@playwright/test";
import { mockConsoleBootstrap } from './helpers/consoleBootstrap';

test("node detail shows remote view for connected agent host", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const assets = [
    {
      id: "agent-host-1",
      name: "Lab Host",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: now,
      metadata: {
        hostname: "lab-host",
      },
    },
  ];

  await page.route("**/api/agents/connected", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ assets: ["agent-host-1"] }),
    });
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
          assetCount: assets.length,
          staleAssetCount: 0,
        },
        endpoints: [],
        assets,
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

  await page.goto("/nodes", { waitUntil: "domcontentloaded" });
  await page.locator("[role='link']").filter({ hasText: "Lab Host" }).first().click();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/agent-host-1$/);
  await expect(page.getByText("Lab Host", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: /Remote View/i }).first()).toBeVisible();
});
