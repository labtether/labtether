import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

// Risk: every terminal opened on a Windows device first asked the Hub for a persistent (tmux) session. Windows has
// no tmux, so the Hub refused it each time (HTTP 409) before the plain session was made. A Windows device is asked
// for the plain session only; Linux and macOS devices keep the persistent one.
vi.mock("../../contexts/StatusContext", () => ({
  useFastStatus: () => ({
    assets: [
      { id: "win01", name: "win01", platform: "windows" },
      { id: "nas01", name: "nas01", platform: "linux" },
    ],
  }),
  useStatusSettings: () => ({ defaultActorID: "actor-1" }),
}));
vi.mock("../useConnectedAgents", () => ({
  useConnectedAgents: () => ({ connectedAgentIds: new Set(["win01", "nas01"]), refreshConnected: vi.fn() }),
}));
vi.mock("../../lib/ws", () => ({ buildBrowserWsUrl: (path: string) => `wss://console.example.test${path}` }));

import { useSession } from "../useSession";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let current: ReturnType<typeof useSession> | null;
let fetchMock: ReturnType<typeof vi.fn>;

function Harness() {
  current = useSession({ type: "terminal" });
  return null;
}

const answer = (status: number, body: unknown) => ({ ok: status < 300, status, json: async () => body }) as unknown as Response;
const requested = () => fetchMock.mock.calls.map(([url]) => String(url));

beforeEach(async () => {
  current = null;
  fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === "/api/terminal/persistent-sessions") {
      const target = (JSON.parse(String(init?.body)) as { target: string }).target;
      if (target === "win01") return answer(409, { error: "persistent terminal sessions need tmux on the device" });
      return answer(200, { persistent_session: { id: "p1" } });
    }
    if (url === "/api/terminal/persistent-sessions/p1/attach") return answer(200, { session: { id: "s1" } });
    if (url === "/api/terminal/session") return answer(200, { session: { id: "s2" } });
    if (url === "/api/terminal/stream-ticket") return answer(200, { streamPath: "/terminal/sessions/s/stream" });
    throw new Error(`unexpected request: ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  root = createRoot(document.createElement("div"));
  await act(async () => root.render(<Harness />));
});

afterEach(async () => {
  await act(async () => root.unmount());
  vi.unstubAllGlobals();
});

it("opens a terminal on a Windows device without asking for a persistent session", async () => {
  await act(async () => { await current?.connect("win01"); });
  expect(requested()).toEqual(["/api/terminal/session", "/api/terminal/stream-ticket"]);
  expect(current?.error).toBeNull();
});

it("still asks for a persistent session on a Linux device", async () => {
  await act(async () => { await current?.connect("nas01"); });
  expect(requested()).toEqual([
    "/api/terminal/persistent-sessions",
    "/api/terminal/persistent-sessions/p1/attach",
    "/api/terminal/stream-ticket",
  ]);
});
