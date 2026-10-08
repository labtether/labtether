import { expect,test } from "@playwright/test";
import {
buildLiveStatusPayload,
buildStatusPayload,
installConsoleApiMocks,
} from "./helpers/consoleApiMocks";
import { commandSendEvents,installTerminalWebSocketMock,makeTerminalAsset } from './helpers/terminalWorkspaceMocks';

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
