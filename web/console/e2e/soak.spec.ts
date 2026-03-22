import { expect, test } from "@playwright/test";
import {
  buildLiveStatusPayload,
  buildStatusPayload,
  installConsoleApiMocks,
} from "./helpers/consoleApiMocks";

const BASE_TS = "2026-01-01T12:00:00.000Z";

test("nodes/logs soak keeps latency stable with bounded heap growth", async ({ page, browserName }) => {
  test.skip(browserName !== "chromium", "heap telemetry is chromium-only");

  const assetCount = 1800;
  const assets = Array.from({ length: assetCount }, (_, index) => ({
    id: `soak-node-${index}`,
    type: "host",
    name: `soak-node-${index}`,
    source: "agent",
    status: "online",
    last_seen_at: BASE_TS,
    metadata: {
      cpu_percent: String((index * 9) % 100),
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
      cpu_used_percent: (index * 9) % 100,
      memory_used_percent: (index * 7) % 100,
      disk_used_percent: (index * 5) % 100,
    },
  }));
  const allLogEvents = Array.from({ length: 8000 }, (_, index) => ({
    id: `soak-log-${index}`,
    source: "agent",
    level: index % 80 === 0 ? "error" : "info",
    message: `soak-event-${index}`,
    timestamp: BASE_TS,
  }));

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets,
      telemetryOverview,
      logSources: [{ source: "agent", count: allLogEvents.length, last_seen_at: BASE_TS }],
      recentLogs: allLogEvents.slice(0, 200),
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
        await fulfillJSON({ events: events.slice(0, 400) }, 200);
        return true;
      }
      return false;
    },
  });

  const heapStart = await page.evaluate(() => {
    const perf = performance as Performance & { memory?: { usedJSHeapSize?: number } };
    return perf.memory?.usedJSHeapSize ?? 0;
  });

  const navDurations: number[] = [];
  for (let i = 0; i < 8; i++) {
    const isNodes = i % 2 === 0;
    const started = Date.now();
    await page.goto(isNodes ? "/nodes" : "/logs");
    if (isNodes) {
      await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();
      await page.getByPlaceholder("Search devices...").fill("soak-node-1799");
      await expect(page.locator("[role='link']").filter({ hasText: "soak-node-1799" }).first()).toBeVisible({ timeout: 3000 });
    } else {
      await expect(page.getByRole("heading", { name: "Logs", level: 1, exact: true })).toBeVisible();
      await page.getByPlaceholder("Search message text (error, timeout, zfs...)").fill("soak-event-7999");
      await expect(page.getByText("soak-event-7999")).toBeVisible({ timeout: 3000 });
    }
    navDurations.push(Date.now() - started);
    await page.waitForTimeout(200);
  }

  const heapEnd = await page.evaluate(() => {
    const perf = performance as Performance & { memory?: { usedJSHeapSize?: number } };
    return perf.memory?.usedJSHeapSize ?? 0;
  });

  const maxNav = Math.max(...navDurations);
  expect(maxNav).toBeLessThan(7000);
  const slowNavigations = navDurations.filter((duration) => duration > 5500).length;
  expect(slowNavigations).toBeLessThanOrEqual(1);

  if (heapStart > 0 && heapEnd > 0) {
    expect(heapEnd - heapStart).toBeLessThan(140 * 1024 * 1024);
  }
});
