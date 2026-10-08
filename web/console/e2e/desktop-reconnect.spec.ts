import { expect, test } from "@playwright/test";
import { installDesktopApiMocks } from "./helpers/desktopApiMocks";
import { installDesktopBrowserMocks } from "./helpers/desktopBrowserMocks";

test("desktop relay-backed WebRTC falls back to VNC and recovers back to WebRTC", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    accelerateReconnectTimers: true,
    webrtcStatsProfile: "fair",
    webrtcRouteType: "relay",
  });
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");
  await page.getByRole("combobox", { name: "Desktop protocol" }).selectOption("webrtc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(0)?.protocol ?? null)
    .toBe("webrtc");
  await expect(page.getByText("webrtc relay")).toBeVisible();

  await expect
    .poll(() => sessionRequests.some((entry) => entry.protocol === "vnc"))
    .toBe(true);

  await page.evaluate(() => {
    const win = window as unknown as {
      __desktopAudit: {
        webrtcStatsProfile: "good" | "fair" | "poor";
        webrtcRouteType: "direct" | "reflexive" | "relay";
      };
    };
    win.__desktopAudit.webrtcStatsProfile = "good";
    win.__desktopAudit.webrtcRouteType = "direct";
  });

  await expect
    .poll(
      () => sessionRequests.filter((entry) => entry.protocol === "webrtc").length,
    )
    .toBe(2);
});

test("desktop VNC reconnect now preserves the chosen monitor and clears the queued retry", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    vncDisconnects: [
      {
        afterMs: 300,
        clean: false,
        reason: "network interrupted",
      },
    ],
  });
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await selectors.nth(2).selectOption("Display 2");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(0) ?? null)
    .toMatchObject({
      protocol: "vnc",
      display: "Display 2",
    });
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByText("Reconnecting (attempt 1/5)...")).toBeVisible();

  await page.getByRole("button", { name: "Reconnect Now" }).click();

  await expect
    .poll(() => sessionRequests.length)
    .toBe(2);
  await expect(sessionRequests[1]).toMatchObject({
    target: "agent-host-1",
    protocol: "vnc",
    display: "Display 2",
    record: false,
  });
  await expect(page.getByText("Reconnecting (attempt 1/5)...")).toHaveCount(0);
  await expect(page.getByTitle("Disconnect")).toBeVisible();

  await page.waitForTimeout(1100);
  expect(sessionRequests).toHaveLength(2);
});

test("desktop VNC audio unavailable state clears after reconnect recovery", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    audioBehaviors: [
      {
        state: "unavailable",
        error: "Desktop audio unavailable.",
      },
      {
        state: "started",
      },
    ],
  });
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(0)?.protocol ?? null)
    .toBe("vnc");
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByTitle("Audio unavailable")).toBeDisabled();
  await expect(page.getByLabel("Volume")).toHaveCount(0);

  await page.getByTitle("Disconnect").click();
  await expect(page.getByRole("button", { name: "Connect", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await expect.poll(() => sessionRequests.length).toBe(2);
  await expect(page.getByTitle("Audio unavailable")).toHaveCount(0);
  await expect(page.getByTitle("Mute audio")).toBeVisible();
  await expect(page.getByLabel("Volume")).toBeVisible();
});

test("desktop VNC reconnect exhaustion surfaces Try Again and can recover", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    accelerateReconnectTimers: true,
    vncDisconnects: [
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
      },
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
        beforeConnect: true,
      },
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
        beforeConnect: true,
      },
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
        beforeConnect: true,
      },
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
        beforeConnect: true,
      },
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
        beforeConnect: true,
      },
    ],
  });
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.length)
    .toBe(6);
  await expect(page.getByText("Connection lost", { exact: true })).toBeVisible();
  await expect(
    page.getByText("Unable to reconnect after 5 attempts", { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Try Again" })).toBeVisible();

  await page.getByRole("button", { name: "Try Again" }).click();
  await expect
    .poll(() => sessionRequests.length)
    .toBe(7);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByRole("button", { name: "Try Again" })).toHaveCount(0);
});
