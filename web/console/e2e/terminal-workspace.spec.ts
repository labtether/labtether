import { expect,test } from "@playwright/test";
import {
buildLiveStatusPayload,
buildStatusPayload,
installConsoleApiMocks,
} from "./helpers/consoleApiMocks";
import { installTerminalWebSocketMock,makeTerminalAsset } from './helpers/terminalWorkspaceMocks';

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
