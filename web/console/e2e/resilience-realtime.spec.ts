import { expect,test } from "@playwright/test";
import {
buildLiveStatusPayload,
buildStatusPayload,
installConsoleApiMocks,
} from "./helpers/consoleApiMocks";
import { BASE_TS } from './helpers/resilienceMocks';

test("status websocket retries after connection failures", async ({ page, browserName }) => {
  let wsEndpointCalls = 0;

  await page.addInitScript(() => {
    class FailingWebSocket {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSING = 2;
      static readonly CLOSED = 3;

      readonly url: string;
      readyState = FailingWebSocket.CONNECTING;
      onopen: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;
      onclose: ((event: CloseEvent) => void) | null = null;

      constructor(url: string | URL) {
        this.url = String(url);
        window.setTimeout(() => this.onerror?.(new Event("error")), 0);
      }

      close() {
        if (this.readyState === FailingWebSocket.CLOSED) return;
        this.readyState = FailingWebSocket.CLOSED;
        window.setTimeout(() => this.onclose?.(new CloseEvent("close")), 0);
      }

      send() {}
      addEventListener() {}
      removeEventListener() {}
      dispatchEvent() { return true; }
    }

    (window as unknown as { WebSocket: typeof WebSocket }).WebSocket =
      FailingWebSocket as unknown as typeof WebSocket;
  });

  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, fulfillJSON }) => {
      if (pathname === "/api/ws/events") {
        wsEndpointCalls++;
        await fulfillJSON({ wsUrl: "ws://127.0.0.1:9/ws/events" }, 200);
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();
  await expect.poll(
    () => wsEndpointCalls,
    { timeout: browserName === "webkit" ? 10_500 : 7_500 },
  ).toBeGreaterThanOrEqual(2);
});

test("hidden-tab pauses fast polling and visible-tab resumes without duplicate loops", async ({ page, context }) => {
  let liveCalls = 0;

  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, fulfillJSON, route }) => {
      if (pathname === "/api/status/live") {
        liveCalls++;
        await fulfillJSON(
          buildLiveStatusPayload({
            assets: [],
            telemetryOverview: [],
          }),
        );
        return true;
      }
      if (pathname === "/api/ws/events") {
        await route.fulfill({ status: 204, body: "" });
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();
  await page.waitForTimeout(6200);

  const backgroundPage = await context.newPage();
  await backgroundPage.goto("about:blank");
  await backgroundPage.bringToFront();

  const beforeHidden = liveCalls;
  await page.waitForTimeout(6200);
  const hiddenDelta = liveCalls - beforeHidden;
  expect(hiddenDelta).toBeLessThanOrEqual(1);

  await page.bringToFront();
  const beforeVisible = liveCalls;
  await page.waitForTimeout(6200);
  const visibleDelta = liveCalls - beforeVisible;

  // One polling loop should produce roughly one refresh every ~5s.
  expect(visibleDelta).toBeGreaterThanOrEqual(1);
  expect(visibleDelta).toBeLessThanOrEqual(3);

  await backgroundPage.close();
});

test("nodes and logs stay responsive under high-volume payloads", async ({ page }) => {
  const assetCount = 1200;
  const assets = Array.from({ length: assetCount }, (_, index) => ({
    id: `load-node-${index}`,
    type: "host",
    name: `load-node-${index}`,
    source: "agent",
    status: "online",
    last_seen_at: BASE_TS,
    metadata: {
      cpu_percent: String((index * 7) % 100),
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
      cpu_used_percent: (index * 7) % 100,
      memory_used_percent: (index * 5) % 100,
      disk_used_percent: (index * 3) % 100,
    },
  }));
  const allLogEvents = Array.from({ length: 5000 }, (_, index) => ({
    id: `log-${index}`,
    source: "agent",
    level: index % 50 === 0 ? "error" : "info",
    message: `high-volume-event-${index}`,
    timestamp: BASE_TS,
  }));

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets,
      telemetryOverview,
      logSources: [{ source: "agent", count: allLogEvents.length, last_seen_at: BASE_TS }],
      recentLogs: allLogEvents.slice(0, 120),
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
        await fulfillJSON({ events: events.slice(0, 300) }, 200);
        return true;
      }
      return false;
    },
  });

  const nodesStarted = Date.now();
  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();
  expect(Date.now() - nodesStarted).toBeLessThan(7000);

  await page.getByPlaceholder("Search devices...").fill("load-node-1199");
  await expect(
    page.locator("[role='link']").filter({ hasText: "load-node-1199" }).first(),
  ).toBeVisible({ timeout: 3000 });

  const logsStarted = Date.now();
  await page.goto("/logs");
  await expect(page.getByRole("heading", { name: "Logs", level: 1, exact: true })).toBeVisible();
  expect(Date.now() - logsStarted).toBeLessThan(7000);

  await page.getByPlaceholder("Search message text (error, timeout, zfs...)").fill("high-volume-event-4999");
  await expect(page.getByText("high-volume-event-4999")).toBeVisible({ timeout: 3000 });
});

test("terminal reconnect reuses prior session id after abnormal disconnect", async ({ page }) => {
  const terminalAsset = {
    id: "node-1",
    type: "host",
    name: "node-1-host",
    source: "agent",
    status: "online",
    last_seen_at: BASE_TS,
  };
  const workspaceTab = {
    id: "tab-1",
    name: "Default",
    layout: "single",
    panes: [{ targetNodeId: terminalAsset.id }],
    sort_order: 0,
  };

  let sessionCreateCalls = 0;
  const streamTicketSessionIds: string[] = [];

  await page.addInitScript(() => {
    let connectCount = 0;

    class MockTerminalWebSocket {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSING = 2;
      static CLOSED = 3;

      readyState = MockTerminalWebSocket.CONNECTING;
      binaryType: string = "arraybuffer";
      onopen: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;
      onclose: ((event: CloseEvent) => void) | null = null;

      constructor(_url: string) {
        connectCount += 1;
        const attempt = connectCount;

        setTimeout(() => {
          this.readyState = MockTerminalWebSocket.OPEN;
          this.onopen?.(new Event("open"));

          if (attempt === 1) {
            setTimeout(() => {
              this.readyState = MockTerminalWebSocket.CLOSED;
              this.onclose?.({
                code: 1011,
                reason: "upstream reset",
                wasClean: false,
              } as CloseEvent);
            }, 10);
          }
        }, 0);
      }

      send(_data: unknown) {}

      close(code = 1000, reason = "") {
        this.readyState = MockTerminalWebSocket.CLOSED;
        this.onclose?.({
          code,
          reason,
          wasClean: code === 1000,
        } as CloseEvent);
      }
    }

    (window as unknown as { WebSocket: unknown }).WebSocket = MockTerminalWebSocket;
  });

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets: [terminalAsset],
    }),
    liveStatusPayload: buildLiveStatusPayload({
      assets: [terminalAsset],
    }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({});
        return true;
      }
      if (pathname === "/api/terminal/snippets" && method === "GET") {
        await fulfillJSON({ snippets: [] });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "GET") {
        await fulfillJSON({ tabs: [workspaceTab] });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "POST") {
        await fulfillJSON({ tab: workspaceTab });
        return true;
      }
      if (pathname === `/api/terminal/workspace/tabs/${workspaceTab.id}` && method === "PUT") {
        await fulfillJSON({ tab: workspaceTab });
        return true;
      }
      if (pathname === "/api/terminal/session" && method === "POST") {
        sessionCreateCalls += 1;
        await fulfillJSON({ session: { id: "term-session-1" } });
        return true;
      }
      if (pathname === "/api/terminal/stream-ticket" && method === "POST") {
        streamTicketSessionIds.push(String(requestBody.sessionId ?? ""));
        await fulfillJSON({ wsUrl: "ws://terminal.mock/session" });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");
  await expect.poll(() => streamTicketSessionIds.length).toBe(1);
  await expect(page.getByText("Session disconnected", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Reconnect", exact: true }).click();

  await expect.poll(() => streamTicketSessionIds.length).toBe(2);
  expect(sessionCreateCalls).toBe(1);
  expect(streamTicketSessionIds).toEqual(["term-session-1", "term-session-1"]);
});
