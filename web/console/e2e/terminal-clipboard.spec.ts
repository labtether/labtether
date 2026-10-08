import { expect,test } from "@playwright/test";
import {
buildLiveStatusPayload,
buildStatusPayload,
installConsoleApiMocks,
} from "./helpers/consoleApiMocks";
import { commandSendEvents,emitTerminalOutput,installClipboardMock,installDeniedClipboardMock,installTerminalWebSocketMock,makeTerminalAsset,triggerTerminalShortcut } from './helpers/terminalWorkspaceMocks';

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
