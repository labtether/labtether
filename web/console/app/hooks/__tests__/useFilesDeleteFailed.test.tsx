import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

// Risk: a file that was already removed on the device stayed in the Files list after Delete failed, so the page
// showed a file that is not there. After a failed delete the folder is read again and the reason is still shown.
const agents = vi.hoisted(() => ({ connectedAgentIds: new Set(["nas01"]), refreshConnected: async () => {} }));
vi.mock("../../contexts/StatusContext", () => ({
  useFastStatus: () => ({ assets: [{ id: "nas01", name: "nas01", platform: "linux" }] }),
}));
vi.mock("../useConnectedAgents", () => ({ useConnectedAgents: () => agents }));

import { useFiles } from "../useFiles";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const file = (name: string) => ({ name, is_dir: false, size: 4, mode: "", mod_time: "" });
const answer = (status: number, body: unknown) => ({ ok: status < 300, status, json: async () => body }) as unknown as Response;

let root: Root;
let current: ReturnType<typeof useFiles> | null;
/** The files in /home/demo on the device right now. */
let onDevice: string[];

function Harness() {
  current = useFiles();
  return null;
}

const listedNames = () => current?.entries.map((entry) => entry.name);

beforeEach(async () => {
  current = null;
  onDevice = ["gone.txt", "kept.txt"];
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), "https://console.example.test");
    if (url.pathname === "/api/files/nas01/list") return answer(200, { path: "/home/demo", entries: onDevice.map(file) });
    if (url.pathname === "/api/files/nas01/delete") return answer(404, { error: "file not found: /home/demo/gone.txt" });
    throw new Error(`unexpected request: ${url.pathname}`);
  }));
  root = createRoot(document.createElement("div"));
  await act(async () => root.render(<Harness />));
  await act(async () => current?.setTarget("nas01"));
  await act(async () => { await current?.listDir("nas01", "/home/demo"); });
  // The file is removed on the device itself after the page listed it.
  onDevice = ["kept.txt"];
});

afterEach(async () => {
  await act(async () => root.unmount());
  vi.unstubAllGlobals();
});

it("drops the row of a file that was already gone when its delete fails, and keeps the reason", async () => {
  expect(listedNames()).toEqual(["gone.txt", "kept.txt"]);
  await act(async () => { await current?.deleteEntry("/home/demo/gone.txt"); });
  expect(listedNames()).toEqual(["kept.txt"]);
  expect(current?.error).toBe("file not found: /home/demo/gone.txt");
});

it("keeps the reason when deleting several files stops at one that is gone", async () => {
  await act(async () => { await current?.deleteSelected(["gone.txt", "kept.txt"]); });
  expect(listedNames()).toEqual(["kept.txt"]);
  expect(current?.error).toBe("file not found: /home/demo/gone.txt");
});
