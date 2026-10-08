import { expect, test } from "@playwright/test";
import { installDesktopApiMocks } from "./helpers/desktopApiMocks";
import { installDesktopBrowserMocks } from "./helpers/desktopBrowserMocks";

test("desktop VNC viewer focuses on click for input readiness", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");

  const viewer = page.locator(".vncContainer").first();
  await expect(viewer).toBeVisible();
  await viewer.click({ position: { x: 24, y: 24 } });

  await expect
    .poll(() =>
      page.evaluate(() => {
        const active = document.activeElement;
        return active instanceof Element
          ? Boolean(active.closest(".vncContainer"))
          : false;
      }),
    )
    .toBe(true);
});

test("desktop VNC native scaling keeps remote drag input enabled", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");
  await expect(page.getByTitle("Disconnect")).toBeVisible();

  await page
    .getByRole("group", { name: "Scaling mode" })
    .getByRole("button", { name: "1:1" })
    .click();

  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            vncRuntime: {
              scaleViewport: boolean;
              resizeSession: boolean;
              clipViewport: boolean;
              dragViewport: boolean;
            };
          };
        };
        const runtime = win.__desktopAudit.vncRuntime;
        return {
          scaleViewport: runtime.scaleViewport,
          resizeSession: runtime.resizeSession,
          clipViewport: runtime.clipViewport,
          dragViewport: runtime.dragViewport,
        };
      }),
    )
    .toEqual({
      scaleViewport: false,
      resizeSession: false,
      clipViewport: true,
      dragViewport: false,
    });
});

test("desktop VNC toolbar shortcuts pass through to remote keys", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");

  await page.getByTitle("More viewer tools").click();
  await page.getByTitle("Alt+Tab").click();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-key",
        );
      }),
    )
    .toEqual([
      { label: "vnc-key", payload: JSON.stringify({ keysym: 65513, down: true }) },
      { label: "vnc-key", payload: JSON.stringify({ keysym: 65289, down: true }) },
      { label: "vnc-key", payload: JSON.stringify({ keysym: 65289, down: false }) },
      { label: "vnc-key", payload: JSON.stringify({ keysym: 65513, down: false }) },
    ]);
});

test("desktop VNC mouse button returns focus and enters pointer lock", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");

  await page.getByTitle("Send mouse to remote session").click();

  await expect
    .poll(() =>
      page.evaluate(() => {
        const active = document.activeElement;
        return active instanceof Element
          ? Boolean(active.closest(".vncContainer"))
          : false;
      }),
    )
    .toBe(true);
  await expect
    .poll(() =>
      page.evaluate(() => document.pointerLockElement?.tagName ?? ""),
    )
    .toBe("CANVAS");
});

test("desktop VNC keyboard capture returns typing to the remote viewer", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");

  await page.getByTitle("More viewer tools").click();
  await page.getByTitle("Send keyboard to remote session").click();
  await expect
    .poll(() =>
      page.evaluate(() => document.pointerLockElement?.className ?? ""),
    )
    .toBe("");
  await page.keyboard.press("a");

  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-dom-keydown",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-dom-keydown",
        payload: JSON.stringify({ key: "a", code: "KeyA" }),
      },
    ]);
});

test("desktop VNC shows a fallback cursor when the remote cursor is hidden", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");

  await expect(page.locator(".vncContainer").first()).toHaveAttribute(
    "data-cursor-fallback",
    "visible",
  );

  await page.getByTitle("Send mouse to remote session").click();

  await expect(page.locator(".vncContainer").first()).not.toHaveAttribute(
    "data-cursor-fallback",
    "visible",
  );

  await page.getByTitle("Mouse ready in remote session").click();
  await expect(page.locator(".vncContainer").first()).toHaveAttribute(
    "data-cursor-fallback",
    "visible",
  );
});

test("desktop fullscreen toolbar defaults to bottom and supports top toggle plus auto-hide", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");
  await page.getByRole("combobox", { name: "Desktop protocol" }).selectOption("webrtc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("webrtc");

  await page.getByTitle("Fullscreen").click();
  await expect
    .poll(() => page.evaluate(() => document.fullscreenElement !== null))
    .toBe(true);

  const disconnectButton = page.getByTitle("Disconnect");
  const viewport = page.viewportSize();
  if (!viewport) {
    throw new Error("viewport unavailable");
  }

  const bottomBox = await disconnectButton.boundingBox();
  if (!bottomBox) {
    throw new Error("disconnect button not rendered");
  }
  expect(bottomBox.y).toBeGreaterThan(viewport.height * 0.65);

  await expect(page.getByTitle("More viewer tools")).toBeVisible();
  await page.getByTitle("More viewer tools").click();
  await expect(page.getByTitle("Alt+Tab")).toBeVisible();

  await page.getByTitle("Move toolbar to top").click();
  const topBox = await disconnectButton.boundingBox();
  if (!topBox) {
    throw new Error("disconnect button missing after moving toolbar");
  }
  expect(topBox.y).toBeLessThan(viewport.height * 0.35);

  await page.getByTitle("Enable auto-hide").click();
  await expect(page.getByTitle("Disable auto-hide")).toBeVisible();
  await page.waitForTimeout(5200);
  await expect(disconnectButton).toBeHidden();
  await expect(page.getByLabel("Show remote view tools")).toBeVisible();

  await page.mouse.move(Math.round(viewport.width / 2), 2);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
});
