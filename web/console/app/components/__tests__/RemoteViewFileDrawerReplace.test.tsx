import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import common from "../../../messages/en/common.json";
import desktop from "../../../messages/en/desktop.json";
import files from "../../../messages/en/files.json";
import { RemoteViewFileDrawer } from "../RemoteViewFileDrawer";

// Risk: on a Windows device "Report.txt" and "report.txt" are one file. The Remote View files drawer compared names
// exactly, so an upload of report.txt replaced Report.txt without a question. Names on a Windows path match in any
// case; on a Linux or macOS path they are two files and the upload starts.

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
/** The folder the device's agent answers with. */
let folder: string;
let uploads: string[];

class FakeUpload {
  upload = {};
  open(_method: string, url: string) {
    uploads.push(new URL(url, "https://console.example.test").searchParams.get("path") ?? "");
  }
  send() {}
}

const settle = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });

async function drop(name: string) {
  const event = new Event("drop", { bubbles: true, cancelable: true });
  Object.defineProperty(event, "dataTransfer", { value: { files: [new File(["new"], name)], types: ["Files"] } });
  await act(async () => { container.querySelector("aside")?.dispatchEvent(event); });
  await settle();
}

const dialogText = () => [...document.body.querySelectorAll<HTMLElement>('[role="dialog"]')].at(-1)?.textContent ?? "";

async function openDrawer(path: string) {
  folder = path;
  await act(async () => root.render(
    <NextIntlClientProvider locale="en" messages={{ common, desktop, files }}>
      <RemoteViewFileDrawer nodeId="nas01" open onClose={() => {}} />
    </NextIntlClientProvider>,
  ));
  await settle();
}

beforeEach(() => {
  uploads = [];
  vi.stubGlobal("fetch", vi.fn(async () => ({
    ok: true,
    status: 200,
    json: async () => ({ path: folder, entries: [{ name: "Report.txt", is_dir: false, size: 4, mode: "", mod_time: "" }] }),
  }) as unknown as Response));
  vi.stubGlobal("XMLHttpRequest", FakeUpload);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

it("asks before an upload replaces a file whose name differs only in case on a Windows device", async () => {
  await openDrawer("C:\\Users\\demo");
  await drop("report.txt");
  expect(dialogText()).toContain("report.txt is already in C:\\Users\\demo");
  expect(uploads).toEqual([]);
});

it("uploads a name that differs in case without asking on a Linux device", async () => {
  await openDrawer("/home/demo");
  await drop("report.txt");
  expect(dialogText()).toBe("");
  expect(uploads).toEqual(["/home/demo/report.txt"]);
});
