import { expect, test, type Page } from "@playwright/test";
import {
  buildLiveStatusPayload,
  buildStatusPayload,
  installConsoleApiMocks,
} from "./helpers/consoleApiMocks";

const BASE_TS = "2026-01-01T12:00:00.000Z";

function makeTerminalAsset(id: string, name: string) {
  return {
    id,
    type: "host",
    name,
    source: "agent",
    status: "online",
    last_seen_at: BASE_TS,
  };
}

async function installTerminalWebSocketMock(page: Page) {
  await page.addInitScript(() => {
    type WsEvent = { url: string; data: string };
    const wsEvents: WsEvent[] = [];
    const sockets: MockTerminalWebSocket[] = [];

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
      url: string;

      constructor(url: string) {
        this.url = url;
        sockets.push(this);
        setTimeout(() => {
          this.readyState = MockTerminalWebSocket.OPEN;
          this.onopen?.(new Event("open"));
        }, 0);
      }

      send(data: unknown) {
        let rendered = "";
        if (typeof data === "string") {
          rendered = data;
        } else if (data instanceof ArrayBuffer) {
          rendered = `[binary:${data.byteLength}]`;
        } else if (ArrayBuffer.isView(data)) {
          rendered = `[binary:${data.byteLength}]`;
        } else {
          rendered = String(data);
        }
        wsEvents.push({ url: this.url, data: rendered });
      }

      close(code = 1000, reason = "") {
        this.readyState = MockTerminalWebSocket.CLOSED;
        this.onclose?.({
          code,
          reason,
          wasClean: code === 1000,
        } as CloseEvent);
      }
    }

    (
      window as unknown as {
        __emitTerminalWsMessage: (data: string, index?: number) => void;
        __terminalWsEvents: WsEvent[];
        WebSocket: unknown;
      }
    ).__emitTerminalWsMessage = (data: string, index = 0) => {
      sockets[index]?.onmessage?.({ data } as MessageEvent);
    };
    (
      window as unknown as {
        __terminalWsEvents: WsEvent[];
        WebSocket: unknown;
      }
    ).__terminalWsEvents = wsEvents;
    (window as unknown as { WebSocket: unknown }).WebSocket = MockTerminalWebSocket;
  });
}

async function installClipboardMock(page: Page, initialText: string) {
  await page.addInitScript((startingText) => {
    let clipboardText = startingText;
    const clipboard = {
      readText: async () => clipboardText,
      writeText: async (next: unknown) => {
        clipboardText = typeof next === "string" ? next : String(next ?? "");
      },
    };
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: clipboard,
    });
  }, initialText);
}

async function installDeniedClipboardMock(page: Page) {
  await page.addInitScript(() => {
    const errorFactory = (action: string) => {
      try {
        return new DOMException(`${action} denied`, "NotAllowedError");
      } catch {
        const error = new Error(`${action} denied`);
        error.name = "NotAllowedError";
        return error;
      }
    };
    const clipboard = {
      readText: async () => {
        throw errorFactory("clipboard-read");
      },
      writeText: async () => {
        throw errorFactory("clipboard-write");
      },
    };
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: clipboard,
    });
    document.execCommand = ((command: string) => command !== "copy") as typeof document.execCommand;
  });
}

async function emitTerminalOutput(page: Page, data: string, index = 0) {
  await page.evaluate(
    ({ payload, socketIndex }) => {
      const win = window as unknown as {
        __emitTerminalWsMessage?: (message: string, index?: number) => void;
      };
      win.__emitTerminalWsMessage?.(payload, socketIndex);
    },
    { payload: data, socketIndex: index },
  );
}

async function triggerTerminalShortcut(
  page: Page,
  key: string,
  options?: { shift?: boolean; meta?: boolean; ctrl?: boolean },
) {
  await page.evaluate(
    ({ eventKey, shift, meta, ctrl }) => {
      window.dispatchEvent(new KeyboardEvent("keydown", {
        key: eventKey,
        shiftKey: shift,
        metaKey: meta,
        ctrlKey: ctrl,
        bubbles: true,
        cancelable: true,
      }));
    },
    {
      eventKey: key,
      shift: options?.shift ?? false,
      meta: options?.meta ?? false,
      ctrl: options?.ctrl ?? true,
    },
  );
}

async function commandSendEvents(page: Page, command: string): Promise<Array<{ url: string; data: string }>> {
  return page.evaluate((cmd) => {
    const win = window as unknown as {
      __terminalWsEvents?: Array<{ url: string; data: string }>;
    };
    return (win.__terminalWsEvents ?? []).filter((event) => event.data === cmd);
  }, command);
}

test("terminal multipane layout switching updates pane count and persists", async ({ page }) => {
  const assets = [makeTerminalAsset("node-1", "alpha-host")];
  const workspaceTab = {
    id: "tab-1",
    name: "Default",
    layout: "single",
    panes: [] as Array<{ targetNodeId: string }>,
    sort_order: 0,
  };

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
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
        const layout = typeof requestBody.layout === "string" ? requestBody.layout : workspaceTab.layout;
        const panes = Array.isArray(requestBody.panes)
          ? (requestBody.panes as Array<{ targetNodeId: string }>)
          : workspaceTab.panes;
        workspaceTab.layout = layout;
        workspaceTab.panes = panes;
        await fulfillJSON({ tab: workspaceTab });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");

  const targetButtons = page.getByTitle("Choose terminal target");
  await expect.poll(() => targetButtons.count()).toBe(1);

  await page.getByTitle("Change layout").click();
  await page.getByRole("button", { name: "Grid", exact: true }).click();

  await expect.poll(() => targetButtons.count()).toBe(4);
  await expect.poll(() => workspaceTab.layout).toBe("grid");

  await page.reload();
  await expect.poll(() => targetButtons.count()).toBe(4);
});

test("terminal creates a default workspace tab when none exist", async ({ page }) => {
  const assets = [makeTerminalAsset("node-1", "alpha-host")];
  const createdTab = {
    id: "tab-default",
    name: "Default",
    layout: "single",
    panes: [] as Array<{ targetNodeId: string }>,
    sort_order: 0,
  };
  let getCalls = 0;
  let created = false;

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
        return true;
      }
      if (pathname === "/api/terminal/snippets" && method === "GET") {
        await fulfillJSON({ snippets: [] });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "GET") {
        getCalls += 1;
        await fulfillJSON({ tabs: created && getCalls > 1 ? [createdTab] : [] });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "POST") {
        created = true;
        await fulfillJSON({ tab: createdTab });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");

  await expect(page.getByText("Default", { exact: true })).toBeVisible();
  await expect.poll(() => created).toBe(true);
  await expect.poll(() => page.getByTitle("Choose terminal target").count()).toBe(1);
});

test("terminal creates a default workspace tab when the tabs payload is malformed", async ({ page }) => {
  const assets = [makeTerminalAsset("node-1", "alpha-host")];
  const createdTab = {
    id: "tab-default",
    name: "Default",
    layout: "single",
    panes: [] as Array<{ targetNodeId: string }>,
    sort_order: 0,
  };
  let created = false;

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
        return true;
      }
      if (pathname === "/api/terminal/snippets" && method === "GET") {
        await fulfillJSON({ snippets: [] });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "GET") {
        await fulfillJSON({ tabs: { unexpected: true } }, 200);
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "POST") {
        created = true;
        await fulfillJSON({ tab: createdTab }, 200);
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");

  await expect(page.getByText("Default", { exact: true })).toBeVisible();
  await expect.poll(() => created).toBe(true);
  await expect(page.getByTitle("Choose terminal target").first()).toBeVisible();
});

test("terminal tab switches reuse an existing session when returning to a pane", async ({ page }) => {
  await installTerminalWebSocketMock(page);

  const assets = [
    makeTerminalAsset("node-1", "alpha-host"),
    makeTerminalAsset("node-2", "beta-host"),
  ];
  const tabs = [
    {
      id: "tab-1",
      name: "Alpha",
      layout: "single",
      panes: [{ targetNodeId: "node-1" }],
      sort_order: 0,
    },
    {
      id: "tab-2",
      name: "Beta",
      layout: "single",
      panes: [{ targetNodeId: "node-2" }],
      sort_order: 1,
    },
  ];
  const sessionCalls: string[] = [];
  const sessionIDs = new Map<string, string>();

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/agents/connected") {
        await fulfillJSON({ assets: ["node-1", "node-2"] });
        return true;
      }
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
        return true;
      }
      if (pathname === "/api/terminal/snippets" && method === "GET") {
        await fulfillJSON({ snippets: [] });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "GET") {
        await fulfillJSON({ tabs });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "POST") {
        await fulfillJSON({ tab: tabs[0] });
        return true;
      }
      if (/^\/api\/terminal\/workspace\/tabs\/[^/]+$/.test(pathname) && method === "PUT") {
        const id = pathname.split("/").pop() ?? "";
        const target = tabs.find((tab) => tab.id === id);
        await fulfillJSON({ tab: target ?? tabs[0] });
        return true;
      }
      if (pathname === "/api/terminal/session" && method === "POST") {
        const target = String(requestBody.target ?? "");
        sessionCalls.push(target);
        if (!sessionIDs.has(target)) {
          sessionIDs.set(target, `session-${target}`);
        }
        await fulfillJSON({ session: { id: sessionIDs.get(target) } });
        return true;
      }
      if (pathname === "/api/terminal/stream-ticket" && method === "POST") {
        const sessionID = String(requestBody.sessionId ?? "");
        await fulfillJSON({ wsUrl: `ws://terminal.mock/${sessionID}` });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");

  await expect.poll(() => sessionCalls.filter((target) => target === "node-1").length).toBe(1);
  await expect.poll(() => sessionCalls.filter((target) => target === "node-2").length).toBe(0);

  await page.locator("[data-tab-id='tab-2']").click();
  await expect.poll(() => sessionCalls.filter((target) => target === "node-2").length).toBe(1);

  await page.locator("[data-tab-id='tab-1']").click();
  await page.waitForTimeout(300);

  expect(sessionCalls.filter((target) => target === "node-1")).toHaveLength(1);
});

test("terminal tab rename from right-click menu persists after reload", async ({ page }) => {
  const assets = [makeTerminalAsset("node-1", "alpha-host")];
  const workspaceTab = {
    id: "tab-1",
    name: "Default",
    layout: "single",
    panes: [] as Array<{ targetNodeId: string }>,
    sort_order: 0,
  };

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
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
        if (typeof requestBody.name === "string") {
          workspaceTab.name = requestBody.name;
        }
        if (typeof requestBody.layout === "string") {
          workspaceTab.layout = requestBody.layout;
        }
        if (Array.isArray(requestBody.panes)) {
          workspaceTab.panes = requestBody.panes as Array<{ targetNodeId: string }>;
        }
        await fulfillJSON({ tab: workspaceTab });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");
  const tabChip = page.locator("[data-tab-id='tab-1']");
  await expect(tabChip).toBeVisible();

  await tabChip.click({ button: "right" });
  await page.getByRole("button", { name: "Rename Tab", exact: true }).click();

  const renameInput = tabChip.locator("input");
  await expect(renameInput).toBeVisible();
  await renameInput.fill("Ops");
  await renameInput.press("Enter");

  await expect.poll(() => workspaceTab.name).toBe("Ops");
  await page.reload();
  await expect(page.locator("[data-tab-id='tab-1']")).toContainText("Ops");
});

test("terminal pane context menu paste sends clipboard text to stream", async ({ page }) => {
  await installTerminalWebSocketMock(page);
  const pasteCommand = "uname -a\n";
  await installClipboardMock(page, pasteCommand);

  const assets = [makeTerminalAsset("node-1", "alpha-host")];
  const workspaceTab = {
    id: "tab-1",
    name: "Default",
    layout: "single",
    panes: [{ targetNodeId: "node-1" }],
    sort_order: 0,
  };

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/agents/connected") {
        await fulfillJSON({ assets: ["node-1"] });
        return true;
      }
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
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
        await fulfillJSON({ session: { id: "session-1", target: String(requestBody.target ?? ""), mode: "interactive" } });
        return true;
      }
      if (pathname === "/api/terminal/stream-ticket" && method === "POST") {
        await fulfillJSON({ wsUrl: "ws://terminal.mock/session-1" });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");
  const terminal = page.locator(".xtermContainer").first();
  await expect(terminal).toBeVisible();

  await expect.poll(async () => {
    return page.evaluate(() => {
      const win = window as unknown as {
        __terminalWsEvents?: Array<{ data: string }>;
      };
      const events = win.__terminalWsEvents ?? [];
      return events.filter((event) => event.data.includes('"type":"resize"')).length;
    });
  }).toBeGreaterThanOrEqual(1);

  await terminal.click({ button: "right" });
  await expect(page.getByRole("button", { name: "Copy Selection", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Select All", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Find", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Clear Scrollback", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Paste", exact: true }).click();
  await expect.poll(async () => (await commandSendEvents(page, pasteCommand)).length).toBeGreaterThanOrEqual(1);
});

test("terminal clipboard shortcuts use the focused pane websocket path", async ({ page }) => {
  await installTerminalWebSocketMock(page);
  const pasteCommand = "printf 'hi from shortcut'\\n";
  await installClipboardMock(page, pasteCommand);

  const assets = [makeTerminalAsset("node-1", "alpha-host")];
  const workspaceTab = {
    id: "tab-1",
    name: "Default",
    layout: "single",
    panes: [{ targetNodeId: "node-1" }],
    sort_order: 0,
  };

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/agents/connected") {
        await fulfillJSON({ assets: ["node-1"] });
        return true;
      }
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
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
        await fulfillJSON({ session: { id: "session-1", target: String(requestBody.target ?? ""), mode: "interactive" } });
        return true;
      }
      if (pathname === "/api/terminal/stream-ticket" && method === "POST") {
        await fulfillJSON({ wsUrl: "ws://terminal.mock/session-1" });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");
  const terminal = page.locator(".xtermContainer").first();
  await expect(terminal).toBeVisible();

  await expect.poll(async () => {
    return page.evaluate(() => {
      const win = window as unknown as {
        __terminalWsEvents?: Array<{ data: string }>;
      };
      const events = win.__terminalWsEvents ?? [];
      return events.filter((event) => event.data.includes('"type":"resize"')).length;
    });
  }).toBeGreaterThanOrEqual(1);

  await terminal.click();
  await triggerTerminalShortcut(page, "V", { ctrl: true, shift: true });

  await expect.poll(async () => (await commandSendEvents(page, pasteCommand)).length).toBeGreaterThanOrEqual(1);
});

test("terminal shows clipboard permission feedback for copy and paste actions", async ({ page }) => {
  await installTerminalWebSocketMock(page);
  await installDeniedClipboardMock(page);

  const assets = [makeTerminalAsset("node-1", "alpha-host")];
  const workspaceTab = {
    id: "tab-1",
    name: "Default",
    layout: "single",
    panes: [{ targetNodeId: "node-1" }],
    sort_order: 0,
  };

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/agents/connected") {
        await fulfillJSON({ assets: ["node-1"] });
        return true;
      }
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
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
        await fulfillJSON({ session: { id: "session-1", target: String(requestBody.target ?? ""), mode: "interactive" } });
        return true;
      }
      if (pathname === "/api/terminal/stream-ticket" && method === "POST") {
        await fulfillJSON({ wsUrl: "ws://terminal.mock/session-1" });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");
  const terminal = page.locator(".xtermContainer").first();
  await expect(terminal).toBeVisible();

  await expect.poll(async () => {
    return page.evaluate(() => {
      const win = window as unknown as {
        __terminalWsEvents?: Array<{ data: string }>;
      };
      const events = win.__terminalWsEvents ?? [];
      return events.filter((event) => event.data.includes('"type":"resize"')).length;
    });
  }).toBeGreaterThanOrEqual(1);

  await emitTerminalOutput(page, "permission test\r\n");
  await page.waitForTimeout(50);
  await terminal.click();
  await triggerTerminalShortcut(page, "a", { ctrl: true });
  await triggerTerminalShortcut(page, "C", { ctrl: true, shift: true });
  await expect(page.getByRole("status")).toContainText("Clipboard write was blocked by the browser");

  await terminal.click({ button: "right" });
  await page.getByRole("button", { name: "Paste", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Clipboard read was blocked by the browser");
});

test("terminal tab menu duplicate and close-other actions keep expected tab set", async ({ page }) => {
  const assets = [makeTerminalAsset("node-1", "alpha-host")];
  const tabs = [
    {
      id: "tab-1",
      name: "Default",
      layout: "single",
      panes: [] as Array<{ targetNodeId: string }>,
      sort_order: 0,
    },
    {
      id: "tab-2",
      name: "Ops",
      layout: "single",
      panes: [] as Array<{ targetNodeId: string }>,
      sort_order: 1,
    },
  ];
  let createSeq = 2;

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
        return true;
      }
      if (pathname === "/api/terminal/snippets" && method === "GET") {
        await fulfillJSON({ snippets: [] });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "GET") {
        await fulfillJSON({ tabs });
        return true;
      }
      if (pathname === "/api/terminal/workspace/tabs" && method === "POST") {
        createSeq += 1;
        const newTab = {
          id: `tab-${createSeq}`,
          name: typeof requestBody.name === "string" ? requestBody.name : `Tab ${createSeq}`,
          layout: "single",
          panes: [] as Array<{ targetNodeId: string }>,
          sort_order: tabs.length,
        };
        tabs.push(newTab);
        await fulfillJSON({ tab: newTab });
        return true;
      }
      if (/^\/api\/terminal\/workspace\/tabs\/[^/]+$/.test(pathname) && method === "PUT") {
        const id = pathname.split("/").pop() ?? "";
        const target = tabs.find((tab) => tab.id === id);
        if (target) {
          if (typeof requestBody.name === "string") {
            target.name = requestBody.name;
          }
          if (typeof requestBody.layout === "string") {
            target.layout = requestBody.layout;
          }
          if (Array.isArray(requestBody.panes)) {
            target.panes = requestBody.panes as Array<{ targetNodeId: string }>;
          }
          await fulfillJSON({ tab: target });
          return true;
        }
      }
      if (/^\/api\/terminal\/workspace\/tabs\/[^/]+$/.test(pathname) && method === "DELETE") {
        const id = pathname.split("/").pop() ?? "";
        const index = tabs.findIndex((tab) => tab.id === id);
        if (index >= 0) {
          tabs.splice(index, 1);
          await fulfillJSON({}, 204);
          return true;
        }
      }
      return false;
    },
  });

  await page.goto("/terminal");
  await expect(page.locator("[data-tab-id='tab-1']")).toBeVisible();
  await expect(page.locator("[data-tab-id='tab-2']")).toBeVisible();

  await page.locator("[data-tab-id='tab-1']").click({ button: "right" });
  await page.getByRole("button", { name: "Duplicate Tab", exact: true }).click();

  await expect.poll(() => tabs.length).toBe(3);
  await expect(page.locator("[data-tab-id='tab-3']")).toContainText("Default Copy");

  await page.locator("[data-tab-id='tab-3']").click({ button: "right" });
  await page.getByRole("button", { name: "Close Other Tabs", exact: true }).click();

  await expect.poll(() => tabs.length).toBe(1);
  await expect(page.locator("[data-tab-id='tab-3']")).toBeVisible();
  await expect(page.locator("[data-tab-id='tab-1']")).toHaveCount(0);
  await expect(page.locator("[data-tab-id='tab-2']")).toHaveCount(0);
});

test("terminal broadcast mirrors snippet input across panes", async ({ page }) => {
  await installTerminalWebSocketMock(page);

  const assets = [
    makeTerminalAsset("node-1", "alpha-host"),
    makeTerminalAsset("node-2", "beta-host"),
  ];
  const workspaceTab = {
    id: "tab-1",
    name: "Broadcast",
    layout: "columns",
    panes: [{ targetNodeId: "node-1" }, { targetNodeId: "node-2" }],
    sort_order: 0,
  };
  const sessionByTarget = new Map<string, string>();
  let sessionSeq = 0;
  const broadcastCommand = "whoami\n";

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/agents/connected") {
        await fulfillJSON({ assets: ["node-1", "node-2"] });
        return true;
      }
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
        return true;
      }
      if (pathname === "/api/terminal/snippets" && method === "GET") {
        await fulfillJSON({
          snippets: [
            {
              id: "snippet-1",
              name: "Echo Identity",
              command: broadcastCommand,
              description: "",
              scope: "global",
              shortcut: "",
              sort_order: 0,
            },
          ],
        });
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
        const target = String(requestBody.target ?? "");
        if (!sessionByTarget.has(target)) {
          sessionSeq += 1;
          sessionByTarget.set(target, `term-session-${sessionSeq}`);
        }
        const sessionID = sessionByTarget.get(target) ?? `term-session-${sessionSeq}`;
        await fulfillJSON({ session: { id: sessionID } });
        return true;
      }
      if (pathname === "/api/terminal/stream-ticket" && method === "POST") {
        const sessionID = String(requestBody.sessionId ?? "");
        await fulfillJSON({ wsUrl: `ws://terminal.mock/${sessionID}` });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");
  await expect(page.getByTitle("Insert Echo Identity")).toBeVisible();

  await expect.poll(async () => {
    return page.evaluate(() => {
      const win = window as unknown as {
        __terminalWsEvents?: Array<{ data: string }>;
      };
      const events = win.__terminalWsEvents ?? [];
      return events.filter((event) => event.data.includes('"type":"resize"')).length;
    });
  }).toBeGreaterThanOrEqual(2);

  await page.getByTitle("Insert Echo Identity").click();
  await expect.poll(async () => (await commandSendEvents(page, broadcastCommand)).length).toBe(1);

  await page.getByTitle("Broadcast OFF (Ctrl+Shift+B)").click();
  await page.getByTitle("Insert Echo Identity").click();

  await expect.poll(async () => (await commandSendEvents(page, broadcastCommand)).length).toBe(3);

  const urls = new Set((await commandSendEvents(page, broadcastCommand)).map((event) => event.url));
  expect(urls.size).toBeGreaterThanOrEqual(2);
});

test("terminal shows staged SSH progress until shell is ready", async ({ page }) => {
  await page.addInitScript(() => {
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
        setTimeout(() => {
          this.readyState = MockTerminalWebSocket.OPEN;
          this.onopen?.(new Event("open"));
        }, 20);
        setTimeout(() => {
          this.onmessage?.(new MessageEvent("message", {
            data: JSON.stringify({ lt_event: "terminal", type: "status", stage: "ssh_connecting", message: "Connecting to SSH endpoint (1/2)..." }),
          }));
        }, 120);
        setTimeout(() => {
          this.onmessage?.(new MessageEvent("message", {
            data: JSON.stringify({ lt_event: "terminal", type: "status", stage: "ssh_starting_shell", message: "Starting remote shell..." }),
          }));
        }, 700);
        setTimeout(() => {
          this.onmessage?.(new MessageEvent("message", {
            data: JSON.stringify({ lt_event: "terminal", type: "ready", stage: "connected", message: "Terminal connected" }),
          }));
        }, 2200);
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

  const assets = [makeTerminalAsset("node-1", "ssh-host")];
  const workspaceTab = {
    id: "tab-ssh",
    name: "SSH",
    layout: "single",
    panes: [{ targetNodeId: "node-1" }],
    sort_order: 0,
  };

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets }),
    liveStatusPayload: buildLiveStatusPayload({ assets }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/agents/connected") {
        await fulfillJSON({ assets: [] });
        return true;
      }
      if (pathname === "/api/terminal/preferences" && method === "GET") {
        await fulfillJSON({ preferences: {} });
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
        await fulfillJSON({ session: { id: "ssh-session-1", target: String(requestBody.target ?? ""), mode: "interactive" } });
        return true;
      }
      if (pathname === "/api/terminal/stream-ticket" && method === "POST") {
        await fulfillJSON({ wsUrl: "ws://terminal.mock/ssh-session-1" });
        return true;
      }
      return false;
    },
  });

  await page.goto("/terminal");

  await expect(page.getByText(/Connecting to SSH endpoint/).first()).toBeVisible();
  await expect(page.getByText(/Starting remote shell/).first()).toBeVisible();
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByText(/Starting remote shell/).first()).not.toBeVisible();
});
