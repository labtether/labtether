import { expect, test, type Route } from "@playwright/test";
import {
  buildLiveStatusPayload,
  buildStatusPayload,
  installConsoleApiMocks,
  type MockRouteContext,
} from "./helpers/consoleApiMocks";

const MOBILE_VIEWPORT = { width: 390, height: 844 } as const;

const CORE_ROUTES: Array<{ path: string; heading: string }> = [
  { path: "/", heading: "Dashboard" },
  { path: "/nodes", heading: "Devices" },
  { path: "/topology", heading: "Topology" },
  { path: "/services", heading: "Services" },
  { path: "/terminal", heading: "Terminal" },
  { path: "/files", heading: "Files" },
  { path: "/logs", heading: "Logs" },
  { path: "/alerts", heading: "Alerts" },
  { path: "/actions", heading: "Actions" },
  { path: "/groups", heading: "Groups" },
  { path: "/reliability", heading: "Health" },
  { path: "/settings", heading: "Settings" },
];

async function fulfillJSON(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

function coreRouteMock(context: MockRouteContext): Promise<boolean> | boolean {
  const { pathname, route } = context;

  if (pathname === "/api/assets") {
    return fulfillJSON(route, { assets: [] }).then(() => true);
  }

  if (pathname === "/api/dependencies") {
    return fulfillJSON(route, { dependencies: [] }).then(() => true);
  }

  if (pathname === "/api/dependencies/graph") {
    return fulfillJSON(route, { nodes: [], links: [] }).then(() => true);
  }

  if (pathname === "/api/services/web") {
    return fulfillJSON(route, { services: [] }).then(() => true);
  }

  if (pathname === "/api/files/workspaces") {
    return fulfillJSON(route, { workspaces: [] }).then(() => true);
  }

  if (/^\/api\/files\/workspace\/.+\/list$/.test(pathname)) {
    return fulfillJSON(route, { entries: [], cwd: "/" }).then(() => true);
  }

  if (/^\/api\/files\/workspace\/.+\/read$/.test(pathname)) {
    return fulfillJSON(route, { content: "", encoding: "utf-8" }).then(() => true);
  }

  if (pathname === "/api/actions/runs") {
    return fulfillJSON(route, { runs: [] }).then(() => true);
  }

  if (pathname === "/api/groups") {
    return fulfillJSON(route, { groups: [] }).then(() => true);
  }

  if (pathname === "/api/incidents") {
    return fulfillJSON(route, { incidents: [] }).then(() => true);
  }

  if (pathname.startsWith("/api/terminal/")) {
    return fulfillJSON(route, { ok: true, sessions: [] }).then(() => true);
  }

  if (pathname.startsWith("/api/v1/terminal/")) {
    return fulfillJSON(route, { ok: true, sessions: [] }).then(() => true);
  }

  return false;
}

test("core routes preserve heading semantics and mobile layout integrity", async ({ page }) => {
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
    customRoute: coreRouteMock,
    unmocked,
  });

  await page.addInitScript(() => {
    localStorage.setItem("labtether.theme", "dark");
    localStorage.setItem("labtether.density", "minimal");
  });

  await page.setViewportSize(MOBILE_VIEWPORT);

  for (const route of CORE_ROUTES) {
    await page.goto(route.path);
    await page.waitForTimeout(200);

    await expect(page.locator("h1", { hasText: route.heading })).toHaveCount(1);

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

    expect(
      overflowPx,
      `${route.path} overflow offenders: ${JSON.stringify(offenders)}`,
    ).toBeLessThanOrEqual(4);
  }

  const filteredErrors = consoleErrors.filter((msg) => {
    const normalized = msg.toLowerCase();
    if (normalized.includes("failed to fetch")) return false;
    if (normalized.includes("failed to load resource")) return false;
    if (normalized.includes("websocket")) return false;
    return true;
  });

  expect(filteredErrors).toEqual([]);
  expect([...unmocked]).toEqual([]);
});
