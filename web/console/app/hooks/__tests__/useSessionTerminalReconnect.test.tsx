import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

// Risk: with "Reconnect automatically" on, typing `exit` opened a new shell, every time, for ever. The terminal
// reported its close as text, and useSession took every close that was not an object for a dropped connection.
vi.mock("../../contexts/StatusContext", () => ({
  useFastStatus: () => ({ assets: [{ id: "nas01", name: "nas01" }] }),
  useStatusSettings: () => ({ defaultActorID: "actor-1" }),
}));
vi.mock("../useConnectedAgents", () => ({
  useConnectedAgents: () => ({ connectedAgentIds: new Set(["nas01"]), refreshConnected: vi.fn() }),
}));
vi.mock("../../lib/ws", () => ({ buildBrowserWsUrl: (path: string) => `wss://console.example.test${path}` }));

import { terminalCloseFrom } from "../../components/terminal/terminalClose";
import { useSession } from "../useSession";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let current: ReturnType<typeof useSession> | null;
let fetchMock: ReturnType<typeof vi.fn>;

function Harness() {
  current = useSession({ type: "terminal", autoReconnect: true });
  return null;
}

const answer = (body: unknown) => ({ ok: true, status: 200, json: async () => body }) as unknown as Response;
const ticketRequests = () => fetchMock.mock.calls.filter(([url]) => String(url) === "/api/terminal/stream-ticket").length;

/** Connect to nas01 and let the shell start, as XTerminal reports it. */
async function openShell() {
  await act(async () => { await current?.connect("nas01"); });
  await act(async () => {
    current?.handleConnected();
    current?.handleStreamReady();
  });
  expect(current?.connectionState).toBe("connected");
  expect(ticketRequests()).toBe(1);
}

beforeEach(async () => {
  vi.useFakeTimers();
  current = null;
  fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === "/api/terminal/persistent-sessions") return answer({ persistent_session: { id: "p1" } });
    if (url === "/api/terminal/persistent-sessions/p1/attach") return answer({ session: { id: "s1" } });
    if (url === "/api/terminal/stream-ticket") return answer({ streamPath: "/terminal/sessions/s1/stream" });
    throw new Error(`unexpected request: ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  root = createRoot(document.createElement("div"));
  await act(async () => root.render(<Harness />));
});

afterEach(async () => {
  await act(async () => root.unmount());
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("does not open a new shell after the shell ended", async () => {
  await openShell();
  // The Hub closes with code 1000 and no reason when the shell exits.
  await act(async () => current?.handleDisconnected(terminalCloseFrom({ code: 1000, reason: "" })));
  expect(current?.isReconnecting).toBe(false);
  expect(current?.connectionState).toBe("idle");

  await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
  expect(ticketRequests()).toBe(1);
  expect(current?.connectionState).toBe("idle");
});

it("still reconnects after the connection dropped", async () => {
  await openShell();
  // Code 1001 with this reason is the Hub's close when the device's agent drops.
  await act(async () => current?.handleDisconnected(terminalCloseFrom({ code: 1001, reason: "Agent disconnected" })));
  expect(current?.isReconnecting).toBe(true);

  await act(async () => { await vi.advanceTimersByTimeAsync(1_000); });
  expect(ticketRequests()).toBe(2);
});
