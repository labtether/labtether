import { expect, test } from "@playwright/test";
import { buildLiveStatusPayload, buildStatusPayload, installConsoleApiMocks } from "./helpers/consoleApiMocks";

type FileEntry = {
  name: string;
  size: number;
  mode: string;
  mod_time: string;
  is_dir: boolean;
};

const BASE_TS = "2026-01-01T12:00:00.000Z";
const ASSET_ID = "node-files-1";

function normalizeDir(path: string): string {
  const trimmed = path.trim();
  if (trimmed === "" || trimmed === "~") return "~";
  if (trimmed === "/") return "/";
  if (trimmed.endsWith("/") && trimmed.length > 1) {
    return trimmed.slice(0, -1);
  }
  return trimmed;
}

function parentDir(path: string): string {
  const normalized = normalizeDir(path);
  if (normalized === "~" || normalized === "/") return normalized;
  const idx = normalized.lastIndexOf("/");
  if (idx < 0) return "~";
  if (idx === 0) return "/";
  if (idx === 1 && normalized.startsWith("~")) return "~";
  return normalized.slice(0, idx);
}

function baseName(path: string): string {
  const normalized = normalizeDir(path);
  const idx = normalized.lastIndexOf("/");
  if (idx < 0) return normalized;
  return normalized.slice(idx + 1);
}

function cloneEntries(entries: FileEntry[]): FileEntry[] {
  return entries.map((entry) => ({ ...entry }));
}

function upsertEntry(tree: Map<string, FileEntry[]>, dir: string, entry: FileEntry) {
  const normalizedDir = normalizeDir(dir);
  const list = tree.get(normalizedDir) ?? [];
  const next = list.filter((item) => item.name !== entry.name);
  next.push(entry);
  tree.set(normalizedDir, next);
}

function removeEntry(tree: Map<string, FileEntry[]>, fullPath: string) {
  const dir = parentDir(fullPath);
  const name = baseName(fullPath);
  const list = tree.get(dir) ?? [];
  tree.set(dir, list.filter((item) => item.name !== name));
}

function getEntry(tree: Map<string, FileEntry[]>, fullPath: string): FileEntry | null {
  const dir = parentDir(fullPath);
  const name = baseName(fullPath);
  const list = tree.get(dir) ?? [];
  return list.find((item) => item.name === name) ?? null;
}

test.describe("files browser context actions", () => {
  test("supports right-click copy/cut/paste and download", async ({ page }) => {
    const copyCalls: Array<{ src: string; dst: string }> = [];
    const renameCalls: Array<{ oldPath: string; newPath: string }> = [];
    const downloadCalls: string[] = [];

    const tree = new Map<string, FileEntry[]>();
    tree.set("~", [
      { name: "docs", size: 0, mode: "drwxr-x---", mod_time: BASE_TS, is_dir: true },
      { name: "alpha.txt", size: 12, mode: "-rw-r-----", mod_time: BASE_TS, is_dir: false },
      { name: "beta.txt", size: 7, mode: "-rw-r-----", mod_time: BASE_TS, is_dir: false },
    ]);
    tree.set("~/docs", []);

    const statusPayload = buildStatusPayload({
      assets: [
        {
          id: ASSET_ID,
          type: "host",
          name: "Files Node",
          platform: "darwin",
          source: "agent",
          status: "online",
          last_seen_at: BASE_TS,
        },
      ],
    });

    await installConsoleApiMocks(page, {
      statusPayload,
      liveStatusPayload: buildLiveStatusPayload({
        assets: statusPayload.assets as unknown[],
      }),
      customRoute: async ({ pathname, method, url, route, fulfillJSON }) => {
        if (pathname === "/api/agents/connected") {
          await fulfillJSON({ assets: [ASSET_ID] }, 200);
          return true;
        }

        if (!pathname.startsWith(`/api/files/${ASSET_ID}/`)) {
          return false;
        }

        if (pathname.endsWith("/list") && method === "GET") {
          const path = normalizeDir(url.searchParams.get("path") ?? "~");
          const entries = cloneEntries(tree.get(path) ?? []);
          await fulfillJSON({ path, entries }, 200);
          return true;
        }

        if (pathname.endsWith("/copy") && method === "POST") {
          const src = normalizeDir(url.searchParams.get("src_path") ?? "");
          const dst = normalizeDir(url.searchParams.get("dst_path") ?? "");
          copyCalls.push({ src, dst });

          const srcEntry = getEntry(tree, src);
          if (!srcEntry) {
            await fulfillJSON({ error: "source not found" }, 404);
            return true;
          }

          upsertEntry(tree, parentDir(dst), {
            ...srcEntry,
            name: baseName(dst),
            mod_time: BASE_TS,
          });
          await fulfillJSON({ request_id: "copy-1", ok: true }, 200);
          return true;
        }

        if (pathname.endsWith("/rename") && method === "POST") {
          const oldPath = normalizeDir(url.searchParams.get("old_path") ?? "");
          const newPath = normalizeDir(url.searchParams.get("new_path") ?? "");
          renameCalls.push({ oldPath, newPath });

          const srcEntry = getEntry(tree, oldPath);
          if (!srcEntry) {
            await fulfillJSON({ error: "source not found" }, 404);
            return true;
          }

          removeEntry(tree, oldPath);
          upsertEntry(tree, parentDir(newPath), {
            ...srcEntry,
            name: baseName(newPath),
            mod_time: BASE_TS,
          });
          await fulfillJSON({ request_id: "move-1", ok: true }, 200);
          return true;
        }

        if (pathname.endsWith("/download") && method === "GET") {
          const path = normalizeDir(url.searchParams.get("path") ?? "");
          downloadCalls.push(path);
          await route.fulfill({
            status: 200,
            contentType: "text/plain",
            headers: { "content-length": "12" },
            body: "hello world!\n",
          });
          return true;
        }

        await fulfillJSON({ error: `unhandled files endpoint: ${method} ${pathname}` }, 400);
        return true;
      },
    });

    await page.goto("/files");

    await page.getByLabel("Target Device").selectOption(ASSET_ID);
    await expect(page.locator('[data-file-entry-name="alpha.txt"]').first()).toBeVisible();

    const rowFor = (name: string) => page.locator(`[data-file-entry-name="${name}"]`).first();
    const contextMenu = () => page.getByRole("menu", { name: "Files context menu" });

    await rowFor("alpha.txt").click({ button: "right" });
    await contextMenu().getByRole("button", { name: /^Copy/ }).click();

    await rowFor("docs").click({ button: "right" });
    await contextMenu().getByRole("button", { name: "Paste" }).click();

    await expect.poll(() => copyCalls.length).toBe(1);
    expect(copyCalls[0]).toEqual({ src: "~/alpha.txt", dst: "~/docs/alpha.txt" });

    await rowFor("beta.txt").click({ button: "right" });
    await contextMenu().getByRole("button", { name: /^Cut/ }).click();

    await rowFor("docs").click({ button: "right" });
    await contextMenu().getByRole("button", { name: "Paste" }).click();

    await expect.poll(() => renameCalls.length).toBe(1);
    expect(renameCalls[0]).toEqual({ oldPath: "~/beta.txt", newPath: "~/docs/beta.txt" });
    await expect(page.getByText(/^Clipboard:/)).toHaveCount(0);

    await rowFor("alpha.txt").click({ button: "right" });
    await contextMenu().getByRole("button", { name: "Download" }).click();

    await expect.poll(() => downloadCalls.length).toBe(1);
    expect(downloadCalls[0]).toBe("~/alpha.txt");
  });

  test("supports right-click upload-here destination", async ({ page }) => {
    const uploadCalls: string[] = [];

    const tree = new Map<string, FileEntry[]>();
    tree.set("~", [
      { name: "docs", size: 0, mode: "drwxr-x---", mod_time: BASE_TS, is_dir: true },
      { name: "readme.md", size: 10, mode: "-rw-r-----", mod_time: BASE_TS, is_dir: false },
    ]);
    tree.set("~/docs", []);

    const statusPayload = buildStatusPayload({
      assets: [
        {
          id: ASSET_ID,
          type: "host",
          name: "Files Node",
          platform: "linux",
          source: "agent",
          status: "online",
          last_seen_at: BASE_TS,
        },
      ],
    });

    await installConsoleApiMocks(page, {
      statusPayload,
      liveStatusPayload: buildLiveStatusPayload({
        assets: statusPayload.assets as unknown[],
      }),
      customRoute: async ({ pathname, method, url, fulfillJSON }) => {
        if (pathname === "/api/agents/connected") {
          await fulfillJSON({ assets: [ASSET_ID] }, 200);
          return true;
        }

        if (!pathname.startsWith(`/api/files/${ASSET_ID}/`)) {
          return false;
        }

        if (pathname.endsWith("/list") && method === "GET") {
          const path = normalizeDir(url.searchParams.get("path") ?? "~");
          const entries = cloneEntries(tree.get(path) ?? []);
          await fulfillJSON({ path, entries }, 200);
          return true;
        }

        if (pathname.endsWith("/upload") && method === "POST") {
          const path = normalizeDir(url.searchParams.get("path") ?? "");
          uploadCalls.push(path);
          upsertEntry(tree, parentDir(path), {
            name: baseName(path),
            size: 5,
            mode: "-rw-r-----",
            mod_time: BASE_TS,
            is_dir: false,
          });
          await fulfillJSON({ request_id: "upload-1", bytes_written: 5, done: true }, 200);
          return true;
        }

        await fulfillJSON({ error: `unhandled files endpoint: ${method} ${pathname}` }, 400);
        return true;
      },
    });

    await page.goto("/files");

    await page.getByLabel("Target Device").selectOption(ASSET_ID);
    await expect(page.locator('[data-file-entry-name="docs"]').first()).toBeVisible();

    const docsRow = page.locator('[data-file-entry-name="docs"]').first();
    await docsRow.click({ button: "right" });
    await page.getByRole("menu", { name: "Files context menu" }).getByRole("button", { name: "Upload Here" }).click();

    await page.locator('input[type="file"]').setInputFiles({
      name: "upload.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("hello"),
    });

    await expect.poll(() => uploadCalls.length).toBe(1);
    expect(uploadCalls[0]).toBe("~/docs/upload.txt");
  });
});
