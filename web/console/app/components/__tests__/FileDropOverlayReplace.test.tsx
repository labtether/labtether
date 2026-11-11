import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import common from "../../../messages/en/common.json";
import files from "../../../messages/en/files.json";

// Risk: a file dropped onto the Remote View screen was written over a file of the same name in Downloads without a
// question, and the old file was gone. The drop must upload nothing until the folder is read and the person picks
// Replace, and nothing at all when the folder cannot be read.
vi.mock("../../contexts/StatusContext", () => ({
  useFastStatus: () => ({ assets: [{ id: "nas01", name: "nas01", platform: "linux" }] }),
}));

import FileDropOverlay from "../FileDropOverlay";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
/** What the Hub answers when the drop target is listed. */
let listing: { status: number; body: unknown };
const listed = vi.fn<(url: string) => void>();
const upload = vi.fn<(file: File, targetPath: string) => Promise<void>>(async () => {});

const settle = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });

async function drop(names: string[]) {
  const event = new Event("drop", { bubbles: true, cancelable: true });
  Object.defineProperty(event, "dataTransfer", { value: { files: names.map((name) => new File(["new"], name)), types: ["Files"] } });
  await act(async () => { container.querySelector(".screen")?.dispatchEvent(event); });
  await settle();
}

function dialogButton(label: string): HTMLButtonElement {
  const dialog = [...document.body.querySelectorAll<HTMLElement>('[role="dialog"]')].at(-1);
  const match = [...(dialog?.querySelectorAll<HTMLButtonElement>("button") ?? [])].find((node) => node.textContent?.trim() === label);
  if (!match) throw new Error(`missing dialog button: ${label}`);
  return match;
}

const dialogText = () => [...document.body.querySelectorAll<HTMLElement>('[role="dialog"]')].at(-1)?.textContent ?? "";
const uploadedPaths = () => upload.mock.calls.map(([, targetPath]) => targetPath);

beforeEach(async () => {
  listing = { status: 200, body: { path: "/home/demo/Downloads", entries: [{ name: "two.txt", is_dir: false, size: 4, mode: "", mod_time: "" }] } };
  listed.mockClear();
  upload.mockClear();
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    listed(String(input));
    return { ok: listing.status < 300, status: listing.status, json: async () => listing.body } as unknown as Response;
  }));
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  // uploadFile is the viewer's own channel (WebRTC); the folder is still read through the Hub's list call.
  await act(async () => root.render(
    <NextIntlClientProvider locale="en" messages={{ common, files }}>
      <FileDropOverlay assetId="nas01" targetDir="~/Downloads" uploadFile={upload}>
        <div className="screen" />
      </FileDropOverlay>
    </NextIntlClientProvider>,
  ));
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

it("asks before a dropped file replaces one in the folder, and uploads only after Replace", async () => {
  await drop(["two.txt"]);
  expect(listed).toHaveBeenCalledWith(expect.stringContaining("/api/files/nas01/list?"));
  expect(dialogText()).toContain("two.txt is already in ~/Downloads");
  expect(upload).not.toHaveBeenCalled();

  await act(async () => dialogButton("Cancel").click());
  await settle();
  expect(upload).not.toHaveBeenCalled();

  await drop(["two.txt"]);
  await act(async () => dialogButton("Replace").click());
  await settle();
  expect(uploadedPaths()).toEqual(["~/Downloads/two.txt"]);
});

it("uploads a file with a new name without asking", async () => {
  await drop(["notes.txt"]);
  expect(uploadedPaths()).toEqual(["~/Downloads/notes.txt"]);
});

it("uploads nothing and says so when the folder cannot be read", async () => {
  listing = { status: 502, body: { error: "agent disconnected before it answered" } };
  await drop(["two.txt"]);
  expect(upload).not.toHaveBeenCalled();
  expect(container.querySelector('[role="alert"]')?.textContent).toBe("~/Downloads could not be read. Nothing was uploaded.");
});

it("uploads into a folder that does not exist yet", async () => {
  listing = { status: 404, body: { error: "folder not found: ~/Downloads" } };
  await drop(["two.txt"]);
  expect(uploadedPaths()).toEqual(["~/Downloads/two.txt"]);
});
