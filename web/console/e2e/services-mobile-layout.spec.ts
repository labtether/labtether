import { expect, test, type Route } from "@playwright/test";
import {
  buildLiveStatusPayload,
  buildStatusPayload,
  installConsoleApiMocks,
  type MockRouteContext,
} from "./helpers/consoleApiMocks";

const MOBILE_VIEWPORT = { width: 390, height: 844 } as const;

async function fulfillJSON(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

function servicesRouteMock(context: MockRouteContext): Promise<boolean> | boolean {
  const { pathname, route } = context;
  if (pathname === "/api/services/web") {
    return fulfillJSON(route, { services: [] }).then(() => true);
  }
  if (pathname === "/api/services/web/compat") {
    return fulfillJSON(route, { compatible: [] }).then(() => true);
  }
  return false;
}

test("services route keeps heading semantics and avoids mobile horizontal overflow", async ({ page }) => {
  const unmocked = new Set<string>();
  const consoleErrors: string[] = [];

  page.on("console", (msg) => {
    if (msg.type() === "error") {
      consoleErrors.push(msg.text());
    }
  });

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets: [], connectors: [], telemetryOverview: [] }),
    liveStatusPayload: buildLiveStatusPayload({ assets: [], telemetryOverview: [] }),
    customRoute: servicesRouteMock,
    unmocked,
  });

  await page.addInitScript(() => {
    localStorage.setItem("labtether.theme", "dark");
    localStorage.setItem("labtether.density", "minimal");
  });

  await page.setViewportSize(MOBILE_VIEWPORT);
  await page.goto("/services");
  await page.waitForTimeout(200);

  await expect(page.locator("h1", { hasText: "Services" })).toHaveCount(1);

  const overflowPx = await page.evaluate(() => {
    const root = document.documentElement;
    return Math.max(0, root.scrollWidth - window.innerWidth);
  });
  const offenders = await page.evaluate(() => {
    return Array.from(document.querySelectorAll<HTMLElement>("*"))
      .map((element) => {
        const rect = element.getBoundingClientRect();
        return {
          tag: element.tagName.toLowerCase(),
          className: element.className,
          right: Math.round(rect.right),
          width: Math.round(rect.width),
          text: (element.textContent ?? "").trim().slice(0, 40),
        };
      })
      .filter((entry) => entry.width > 0 && entry.right > window.innerWidth + 4)
      .sort((a, b) => b.right - a.right)
      .slice(0, 8);
  });

  expect(overflowPx, `/services overflow offenders: ${JSON.stringify(offenders)}`).toBeLessThanOrEqual(4);

  const filteredErrors = consoleErrors.filter((msg) => {
    const normalized = msg.toLowerCase();
    if (normalized.includes("failed to fetch")) return false;
    if (normalized.includes("websocket")) return false;
    return true;
  });

  expect(filteredErrors).toEqual([]);
  expect([...unmocked]).toEqual([]);
});
