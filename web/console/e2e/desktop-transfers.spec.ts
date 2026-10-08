import { expect, test } from "@playwright/test";
import { installDesktopApiMocks } from "./helpers/desktopApiMocks";
import { installDesktopBrowserMocks } from "./helpers/desktopBrowserMocks";

test("desktop VNC toolbar supports clipboard sync through the agent API", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, {
    sessionRequests,
  });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  if ((await page.getByTitle("Pull clipboard from remote").count()) === 0) {
    await page.getByTitle("More viewer tools").click();
  }
  await expect(page.getByTitle("Pull clipboard from remote")).toBeVisible();
  await expect(page.getByTitle("Push clipboard to remote")).toBeVisible();

  await page.evaluate(async () => navigator.clipboard.writeText("local clipboard text"));
  await page.getByTitle("Push clipboard to remote").click();
  await expect
    .poll(() =>
      page.evaluate(async () => {
        const response = await fetch("/api/v1/nodes/agent-host-1/clipboard/get", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ format: "text" }),
        });
        const payload = (await response.json()) as { text?: string };
        return payload.text ?? "";
      }),
    )
    .toBe("local clipboard text");

  await page.evaluate(async () => {
    await fetch("/api/v1/nodes/agent-host-1/clipboard/set", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ format: "text", text: "remote clipboard text" }),
    });
  });
  await page.getByTitle("Pull clipboard from remote").click();
  await expect.poll(() => page.evaluate(async () => navigator.clipboard.readText())).toBe(
    "remote clipboard text",
  );
});

test("desktop VNC drag-and-drop upload falls back to the files API", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  const fileUploads: Array<{ path: string; body: string }> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests, fileUploads });

  await page.goto("/nodes/agent-host-1?panel=desktop");
  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");
  await expect(page.getByTitle("Disconnect")).toBeVisible({ timeout: 15000 });

  await page.evaluate(() => {
    const target = document.querySelector(
      ".vncViewerStage .relative.h-full.w-full",
    );
    if (!(target instanceof HTMLDivElement)) {
      throw new Error("file drop target unavailable");
    }
    const transfer = new DataTransfer();
    transfer.items.add(
      new File(["desktop upload"], "notes.txt", { type: "text/plain" }),
    );
    target.dispatchEvent(
      new DragEvent("dragenter", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
    target.dispatchEvent(
      new DragEvent("dragover", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
    target.dispatchEvent(
      new DragEvent("drop", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
  });

  await expect.poll(() => fileUploads).toEqual([
    {
      path: "~/Downloads/notes.txt",
      body: "desktop upload",
    },
  ]);
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            fileTransfers: Array<{
              requestId: string;
              name: string;
              path: string;
              chunks: string[];
            }>;
          };
        };
        return win.__desktopAudit.fileTransfers;
      }),
    )
    .toEqual([]);
});

test("desktop WebRTC toolbar supports browser recording and clipboard sync", async ({
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
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: { webrtcEvents: string[] };
        };
        return win.__desktopAudit.webrtcEvents;
      }),
    )
    .toContain("pc:connected");
  await expect(page.getByTitle("Disconnect")).toBeVisible({ timeout: 15000 });
  await expect(page.getByTitle("Start recording")).toBeVisible({ timeout: 15000 });
  await expect(page.getByLabel("Volume")).toBeVisible({ timeout: 15000 });

  await page.getByTitle("Start recording").click();
  await expect(page.getByTitle("Stop recording")).toBeVisible();
  await page.getByTitle("Stop recording").click();

  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            mediaRecorderStarts: number;
            mediaRecorderStops: number;
          };
        };
        return {
          starts: win.__desktopAudit.mediaRecorderStarts,
          stops: win.__desktopAudit.mediaRecorderStops,
        };
      }),
    )
    .toEqual({ starts: 1, stops: 1 });

  await page.getByTitle("More viewer tools").click();
  await expect(page.getByTitle("Pull clipboard from remote")).toBeVisible();
  await expect(page.getByTitle("Push clipboard to remote")).toBeVisible();
  await page.getByTitle("Push clipboard to remote").click();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: { clipboardRemoteWriteText: string };
        };
        return win.__desktopAudit.clipboardRemoteWriteText;
      }),
    )
    .toBe("local clipboard text");

  await page.getByTitle("Pull clipboard from remote").click();
  await expect.poll(() => page.evaluate(async () => navigator.clipboard.readText())).toBe(
    "remote clipboard text",
  );
});

test("desktop WebRTC drag-and-drop upload sends file transfer chunks", async ({
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
  await expect(page.getByTitle("Disconnect")).toBeVisible({ timeout: 15000 });

  await page.evaluate(() => {
    const target = document.querySelector(
      ".vncViewerStage .relative.h-full.w-full",
    );
    if (!(target instanceof HTMLDivElement)) {
      throw new Error("file drop target unavailable");
    }
    const transfer = new DataTransfer();
    transfer.items.add(
      new File(["desktop upload"], "notes.txt", { type: "text/plain" }),
    );
    target.dispatchEvent(
      new DragEvent("dragenter", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
    target.dispatchEvent(
      new DragEvent("dragover", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
    target.dispatchEvent(
      new DragEvent("drop", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
  });

  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            fileTransfers: Array<{
              requestId: string;
              name: string;
              path: string;
              chunks: string[];
            }>;
          };
        };
        return win.__desktopAudit.fileTransfers;
      }),
    )
    .toEqual([
      {
        requestId: expect.any(String),
        name: "notes.txt",
        path: "~/Downloads/notes.txt",
        chunks: ["ZGVza3RvcCB1cGxvYWQ="],
      },
    ]);
});
