import { expect,test } from "@playwright/test";
import {
buildLiveStatusPayload,
buildStatusPayload,
installConsoleApiMocks,
} from "./helpers/consoleApiMocks";
import { BASE_TS } from './helpers/resilienceMocks';

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
