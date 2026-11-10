import { expect, test, type Locator, type Page } from "@playwright/test";
import {
  buildLiveStatusPayload,
  buildStatusPayload,
  installConsoleApiMocks,
} from "./helpers/consoleApiMocks";
import {
  commandSendEvents,
  emitTerminalOutput,
  installClipboardMock,
  installDeniedClipboardMock,
  installTerminalWebSocketMock,
  makeTerminalAsset,
  mockClipboardText,
  pressInShell,
  pressOutsideShell,
} from "./helpers/terminalWorkspaceMocks";

/** Opens /terminal with one connected pane and waits until its shell is ready (first resize frame sent). */
async function openTerminal(page: Page) {
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

  return terminal;
}

/** Selects everything in the pane from its right-click menu, then presses Ctrl+Shift+C while typing in the shell. */
async function selectAllThenCopyInShell(page: Page, terminal: Locator) {
  await terminal.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Select all", exact: true }).click();
  await pressInShell(page, "Control+Shift+KeyC");
}

test("terminal pane context menu paste sends clipboard text to stream", async ({ page }) => {
  await installTerminalWebSocketMock(page);
  const pasteCommand = "uname -a\n";
  await installClipboardMock(page, pasteCommand);
  const terminal = await openTerminal(page);

  await terminal.click({ button: "right" });
  await expect(page.getByRole("menuitem", { name: "Copy", exact: true })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Select all", exact: true })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Find…", exact: true })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Clear", exact: true })).toBeVisible();
  await page.getByRole("menuitem", { name: "Paste", exact: true }).click();
  // Paste goes through xterm's paste(), which sends LF as CR (the Enter key)
  // and adds bracketed-paste markers only when the shell enables them; the
  // mock shell never does, so one CR-terminated frame is expected.
  const pastedFrame = "uname -a\r";
  await expect.poll(async () => (await commandSendEvents(page, pastedFrame)).length).toBeGreaterThanOrEqual(1);
  expect(await commandSendEvents(page, pasteCommand)).toHaveLength(0);
});

// The keys are pressed for real, with the shell focused. This spec used to dispatch them on window, which stayed
// green while Ctrl+Shift+C did nothing for someone typing in the shell: the pane ignored every key whose target was
// xterm's own input.
test("Ctrl+Shift+C copies the selection while typing in the shell", async ({ page }) => {
  await installTerminalWebSocketMock(page);
  await installClipboardMock(page, "");
  const terminal = await openTerminal(page);

  await emitTerminalOutput(page, "copy me from the shell\r\n");
  await expect(async () => {
    await selectAllThenCopyInShell(page, terminal);
    expect(await mockClipboardText(page)).toContain("copy me from the shell");
  }).toPass({ timeout: 10_000 });

  // The copy key belongs to the console: nothing was typed into the shell.
  expect(await commandSendEvents(page, "\u0003")).toHaveLength(0);
});

test("Ctrl+Shift+V pastes into the focused pane when the page has the keys", async ({ page }) => {
  await installTerminalWebSocketMock(page);
  const pasteCommand = "printf 'hi from shortcut'";
  await installClipboardMock(page, pasteCommand);
  const terminal = await openTerminal(page);

  // In the shell the browser's own paste handles this key. With focus on the page, the pane reads the clipboard.
  await terminal.click();
  await pressOutsideShell(page, "Control+Shift+KeyV");

  await expect.poll(async () => (await commandSendEvents(page, pasteCommand)).length).toBeGreaterThanOrEqual(1);
});

test("terminal shows clipboard permission feedback for copy and paste actions", async ({ page }) => {
  await installTerminalWebSocketMock(page);
  await installDeniedClipboardMock(page);
  const terminal = await openTerminal(page);

  await emitTerminalOutput(page, "permission test\r\n");
  await expect(async () => {
    await selectAllThenCopyInShell(page, terminal);
    await expect(page.getByRole("status")).toContainText("The browser blocked copying", { timeout: 1_000 });
  }).toPass({ timeout: 10_000 });

  await terminal.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Paste", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("The browser blocked pasting");
});
