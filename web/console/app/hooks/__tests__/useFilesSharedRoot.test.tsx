import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

// Risk: every time a device that shares only its home folder was opened in Files, the page asked its agent for "/"
// to learn what it shares. The agent refuses that, so each open left one refused request (HTTP 400) in the
// browser's network log. The agent reports what it shares; "/" is asked for only when that is the whole file system.
const agents = vi.hoisted(() => ({ connectedAgentIds: new Set(["nas01"]), refreshConnected: async () => {} }));
vi.mock("../../contexts/StatusContext", () => ({
  useFastStatus: () => ({ assets: [{ id: "nas01", name: "nas01", platform: "linux" }] }),
}));
vi.mock("../useConnectedAgents", () => ({ useConnectedAgents: () => agents }));

import { useFiles } from "../useFiles";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let current: ReturnType<typeof useFiles> | null;
/** What the device's agent reports for files_root_mode; undefined for an agent that does not report it. */
let reported: string | undefined;
let listedPaths: string[];

function Harness() {
  current = useFiles();
  return null;
}

const answer = (status: number, body: unknown) => ({ ok: status < 300, status, json: async () => body }) as unknown as Response;

/** Open the device as the Files page does: its home folder first. */
async function openDevice() {
  await act(async () => current?.setTarget("nas01"));
  await act(async () => {
    await current?.listDir("nas01", "~");
    // The shared folder is worked out after the listing shows.
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

beforeEach(async () => {
  current = null;
  reported = "home";
  listedPaths = [];
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), "https://console.example.test");
    if (url.pathname === "/api/agents/nas01/settings") {
      return answer(200, { settings: [{ key: "files_root_mode", state_value: reported, effective_value: "home" }] });
    }
    if (url.pathname === "/api/files/nas01/list") {
      const path = url.searchParams.get("path") ?? "";
      listedPaths.push(path);
      if (path === "~") return answer(200, { path: "/home/demo", entries: [] });
      // Only an agent that shares the whole file system answers for "/".
      if (path === "/" && reported !== "home") return answer(200, { path: "/", entries: [] });
      return answer(400, { error: `path "${path}" is outside the allowed base directory` });
    }
    throw new Error(`unexpected request: ${url.pathname}`);
  }));
  root = createRoot(document.createElement("div"));
  await act(async () => root.render(<Harness />));
});

afterEach(async () => {
  await act(async () => root.unmount());
  vi.unstubAllGlobals();
});

it("asks a device that shares only its home folder for nothing above it", async () => {
  await openDevice();
  expect(listedPaths).toEqual(["~"]);
  expect(current?.rootPath).toBe("/home/demo");
});

it("still finds the root of a device that shares the whole file system", async () => {
  reported = "full";
  await openDevice();
  expect(listedPaths).toEqual(["~", "/"]);
  expect(current?.rootPath).toBe("/");
});

it("asks for / as before when the agent does not report what it shares", async () => {
  reported = undefined;
  await openDevice();
  expect(listedPaths).toEqual(["~", "/"]);
  expect(current?.rootPath).toBe("/");
});
