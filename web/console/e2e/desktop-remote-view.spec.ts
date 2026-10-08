import { expect, test } from "@playwright/test";
import { installDesktopApiMocks } from "./helpers/desktopApiMocks";
import { installDesktopBrowserMocks } from "./helpers/desktopBrowserMocks";

test("desktop protocol switching hides stale display selections before WebRTC connect", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await expect(page.getByRole("button", { name: "Connect", exact: true })).toBeVisible();
  await expect(selectors.nth(1)).toHaveValue("vnc");
  await expect(page.getByText("Display", { exact: true })).toBeVisible();
  await selectors.nth(2).selectOption("Display 2");

  await selectors.nth(1).selectOption("webrtc");
  await expect(page.getByText("Display", { exact: true })).toHaveCount(0);

  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await expect
    .poll(() => sessionRequests.at(-1) ?? null)
    .toMatchObject({
      target: "agent-host-1",
      protocol: "webrtc",
      display: "",
      record: false,
    });
});

test("desktop Proxmox QEMU defaults to VNC while still offering SPICE", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, {
    sessionRequests,
    vncStreamTicketStatus: 500,
    assetOverrides: {
      id: "proxmox-vm-100",
      name: "ContainerVM",
      type: "vm",
      source: "proxmox",
      platform: "",
      metadata: {
        proxmox_type: "qemu",
      },
    },
    connectedAgentAssetIDs: [],
  });

  await page.goto("/nodes/proxmox-vm-100?panel=desktop");

  const selectors = page.locator("main select");
  await expect(selectors.nth(1)).toHaveValue("vnc");
  await expect(selectors.nth(1)).toContainText("VNC (Recommended)");
  await expect(selectors.nth(1)).toContainText("SPICE (Optional)");
  await expect(page.getByText("Display", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");
});

test("desktop VNC connect preserves the selected monitor in the session request", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await expect(page.getByText("Display", { exact: true })).toBeVisible();
  await selectors.nth(2).selectOption("Display 2");

  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1) ?? null)
    .toMatchObject({
      target: "agent-host-1",
      protocol: "vnc",
      display: "Display 2",
      record: false,
    });
});

test("desktop WebRTC renders multi-monitor streams as a stitched layout", async ({
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

  await expect(page.locator('[data-webrtc-layout="stitched"]')).toBeVisible();
  await expect(page.locator('[data-webrtc-display="Display 1"]')).toBeVisible();
  await expect(page.locator('[data-webrtc-display="Display 2"]')).toBeVisible();
});

test("desktop WebRTC native scaling uses a scrollable 1:1 container", async ({
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
  await expect(page.getByTitle("Disconnect")).toBeVisible();

  await page
    .getByRole("group", { name: "Scaling mode" })
    .getByRole("button", { name: "1:1" })
    .click();

  await expect
    .poll(() =>
      page.evaluate(() => {
        const container = document.querySelector(".vncContainer");
        const stage = document.querySelector('[data-webrtc-layout="stitched"]');
        return {
          nativeClass: container?.className.includes("vncNative") ?? false,
          stageWidth:
            stage instanceof HTMLDivElement ? stage.style.width : null,
          stageHeight:
            stage instanceof HTMLDivElement ? stage.style.height : null,
          stageMaxWidth:
            stage instanceof HTMLDivElement ? stage.style.maxWidth : null,
        };
      }),
    )
    .toEqual({
      nativeClass: true,
      stageWidth: "4480px",
      stageHeight: "1440px",
      stageMaxWidth: "none",
    });
});
