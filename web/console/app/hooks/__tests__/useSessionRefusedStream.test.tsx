import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../contexts/StatusContext", () => ({
  useFastStatus: () => ({ assets: [{ id: "win01", name: "win01" }] }),
  useStatusSettings: () => ({ defaultActorID: "actor-1" }),
}));
vi.mock("../useConnectedAgents", () => ({
  useConnectedAgents: () => ({
    connectedAgentIds: new Set(["win01"]),
    refreshConnected: vi.fn(),
  }),
}));
vi.mock("../../lib/ws", () => ({
  buildBrowserWsUrl: (path: string) => `wss://console.example.test${path}`,
}));

import { useSession } from "../useSession";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const noGateway = "RDP needs the Hub's RDP gateway (guacd), which is not running on this Hub.";
const json = (body: unknown, status: number) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

let container: HTMLDivElement;
let root: Root;
let current: ReturnType<typeof useSession> | null;
let ticketAnswer: Response;
let fetchMock: ReturnType<typeof vi.fn>;

function Harness() {
  current = useSession({ type: "desktop", fixedTarget: "win01" });
  return null;
}

beforeEach(async () => {
  current = null;
  fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === "/api/desktop/session" && init?.method === "POST") return json({ sessionId: "rdp-session-1" }, 201);
    if (url === "/api/desktop/stream-ticket") return ticketAnswer;
    if (url === "/api/desktop/session/rdp-session-1" && init?.method === "DELETE") return new Response(null, { status: 204 });
    throw new Error(`unexpected request: ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => {
    root.render(<Harness />);
  });
});

afterEach(async () => {
  await act(async () => {
    root.unmount();
  });
  container.remove();
  vi.unstubAllGlobals();
});

async function connectRDP(): Promise<void> {
  await act(async () => {
    await current?.connect(undefined, { protocol: "rdp", display: "", record: false });
  });
}

function expectFailedWith(reason: string) {
  expect(current?.connectionState).toBe("error");
  expect(current?.error).toBe(reason);
  expect(current?.connectionProgress.phase).toBe("error");
  expect(current?.isReconnecting).toBe(false);
  expect(fetchMock).toHaveBeenCalledWith(
    "/api/desktop/session/rdp-session-1",
    expect.objectContaining({ method: "DELETE" }),
  );
}

// RDP on a Hub without its gateway sat on "Opening stream" for ever with only Cancel. Whichever step refuses, the
// page must end as failed with the reason, close the session at the Hub, and not start retrying by itself.
describe("useSession when Remote View's stream is refused", () => {
  it("ends as failed with the Hub's reason when the stream ticket is refused", async () => {
    ticketAnswer = json({ error: noGateway }, 409);
    await connectRDP();
    expectFailedWith(noGateway);
    expect(current?.wsUrl).toBeNull();
  });

  it("ends as failed with the stream's reason when the viewer reports that it did not open", async () => {
    ticketAnswer = json({ streamPath: "/desktop/sessions/rdp-session-1/stream", secure: true }, 200);
    await connectRDP();
    expect(current?.connectionProgress.phase).toBe("opening-stream");

    await act(async () => {
      current?.handleError(noGateway);
    });
    expectFailedWith(noGateway);
  });
});
