import { expect,test } from "@playwright/test";
import { mockConsoleBootstrap } from './helpers/consoleBootstrap';

test("nodes page shows populated Home Assistant detail panels and keeps entity grouping collector-scoped", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const assets = [
    {
      id: "ha-hub-main",
      name: "Home Assistant Main",
      type: "connector-cluster",
      source: "homeassistant",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        connector_type: "homeassistant",
        collector_id: "collector-ha-main",
        collector_base_url: "http://ha-main.local:8123",
        discovered: "2",
      },
    },
    {
      id: "ha-entity-light-kitchen",
      name: "Kitchen Light",
      type: "ha-entity",
      source: "homeassistant",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        collector_id: "collector-ha-main",
        entity_id: "light.kitchen",
        domain: "light",
        state: "on",
        friendly_name: "Kitchen Light",
        supported_features: "1",
        last_changed: "2026-03-09T08:00:00Z",
        last_updated: "2026-03-09T08:05:00Z",
      },
    },
    {
      id: "ha-entity-sensor-office-temp",
      name: "Office Temp",
      type: "ha-entity",
      source: "homeassistant",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        collector_id: "collector-ha-main",
        entity_id: "sensor.office_temp",
        domain: "sensor",
        state: "23",
        unit_of_measurement: "C",
        device_class: "temperature",
        state_class: "measurement",
        entity_category: "diagnostic",
        last_changed: "2026-03-09T08:10:00Z",
        last_updated: "2026-03-09T08:12:00Z",
      },
    },
    {
      id: "ha-hub-secondary",
      name: "Home Assistant Secondary",
      type: "connector-cluster",
      source: "homeassistant",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        connector_type: "homeassistant",
        collector_id: "collector-ha-secondary",
        collector_base_url: "http://ha-secondary.local:8123",
        discovered: "1",
      },
    },
    {
      id: "ha-entity-switch-garden",
      name: "Garden Switch",
      type: "ha-entity",
      source: "homeassistant",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        collector_id: "collector-ha-secondary",
        entity_id: "switch.garden",
        domain: "switch",
        state: "off",
      },
    },
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
          connectorCount: 1,
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
  const homeAssistantCard = page
    .locator("[role='link']")
    .filter({ hasText: "Home Assistant Main" })
    .first();

  await expect(homeAssistantCard).toHaveCount(1);
  await expect(page.locator("[role='link']").filter({ hasText: "Kitchen Light" })).toHaveCount(0);

  await Promise.all([
    page.waitForURL(/(?:\/[a-z]{2})?\/nodes\/ha-hub-main$/, { timeout: 10_000 }),
    homeAssistantCard.click(),
  ]);

  const homeAssistantPanel = page.getByRole("button", { name: /Home Assistant/i }).first();
  const servicesPanel = page.getByRole("button", { name: /Services/i }).first();
  await expect(homeAssistantPanel).toBeVisible();
  await expect(servicesPanel).toContainText("Services");
  await expect(servicesPanel).toContainText("2 items");
  await expect(page.getByRole("button", { name: /Monitoring/i })).toHaveCount(0);

  await homeAssistantPanel.click();
  await expect(page.getByRole("heading", { name: "Home Assistant Hub", exact: true })).toBeVisible();
  await expect(page.getByText("http://ha-main.local:8123", { exact: true })).toBeVisible();
  await expect(page.getByText("Kitchen Light", { exact: true })).toBeVisible();
  await expect(page.getByText("Office Temp", { exact: true })).toBeVisible();
  await expect(page.getByText("Garden Switch", { exact: true })).toHaveCount(0);

  await page.goto("/nodes/ha-hub-main?panel=services");
  await expect(page.getByRole("link", { name: "Kitchen Light" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Office Temp" })).toBeVisible();
  await expect(page.getByText("Garden Switch", { exact: true })).toHaveCount(0);

  await page.getByRole("link", { name: "Kitchen Light" }).click();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/ha-entity-light-kitchen$/);
  await expect(page.getByRole("button", { name: /Home Assistant/i }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: /Monitoring/i })).toHaveCount(0);

  await page.getByRole("button", { name: /Home Assistant/i }).first().click();
  await expect(page.getByRole("heading", { name: "Home Assistant Entity", exact: true })).toBeVisible();
  await expect(page.getByText("Current State", { exact: true })).toBeVisible();
  await expect(page.getByText("light.kitchen", { exact: true })).toBeVisible();
  await expect(page.getByText("Home Assistant Main", { exact: true })).toBeVisible();
});
