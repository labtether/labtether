import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../contexts/StatusContext", () => ({
  useFastStatus: () => ({ assets: [{ id: "asset-1", name: "Asset 1" }] }),
  useStatusSettings: () => ({ defaultActorID: "actor-1" }),
}));
vi.mock("../useConnectedAgents", () => ({
  useConnectedAgents: () => ({
    connectedAgentIds: new Set(["asset-1"]),
    refreshConnected: vi.fn(),
  }),
}));
vi.mock("../../lib/ws", () => ({
  buildBrowserWsUrl: (path: string) => `wss://console.example.test${path}`,
}));

import { useSession } from "../useSession";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

type DesktopSessionHook = ReturnType<typeof useSession>;

let container: HTMLDivElement;
let root: Root;
let current: DesktopSessionHook | null;
let fetchMock: ReturnType<typeof vi.fn>;

function Harness() {
  current = useSession({ type: "desktop", fixedTarget: "asset-1" });
  return null;
}

beforeEach(async () => {
  current = null;
  fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === "/api/desktop/session" && init?.method === "POST") {
      return new Response(
        JSON.stringify({
          sessionId: "desktop-session-1",
          session: { id: "desktop-session-1" },
        }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      );
    }
    if (url === "/api/desktop/stream-ticket") {
      return new Response(
        JSON.stringify({
          streamPath: "/desktop/sessions/desktop-session-1/stream",
          vncPassword: "  exact password\n",
          secure: true,
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    if (
      url === "/api/desktop/session/desktop-session-1" &&
      init?.method === "DELETE"
    ) {
      return new Response(null, { status: 204 });
    }
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
    await Promise.resolve();
  });
  container.remove();
  vi.unstubAllGlobals();
});

async function connectDesktop(): Promise<void> {
  if (!current) throw new Error("session hook not mounted");
  await act(async () => {
    await current?.connect(undefined, {
      protocol: "vnc",
      display: "Display 1",
      record: false,
    });
  });
}

describe("useSession desktop lifecycle", () => {
  it("preserves ticket password bytes and explicitly terminates on disconnect", async () => {
    await connectDesktop();
    expect(current?.activeSessionId).toBe("desktop-session-1");
    expect(current?.vncPassword).toBe("  exact password\n");

    await act(async () => {
      current?.disconnect();
      await Promise.resolve();
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/desktop/session/desktop-session-1",
      expect.objectContaining({
        method: "DELETE",
        cache: "no-store",
        keepalive: true,
      }),
    );
  });

  it("terminates a created session when ticket acquisition fails", async () => {
    fetchMock.mockImplementationOnce(async () =>
      new Response(
        JSON.stringify({
          sessionId: "desktop-session-1",
          session: { id: "desktop-session-1" },
        }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
    );
    fetchMock.mockImplementationOnce(async () =>
      new Response(JSON.stringify({ error: "ticket failed" }), {
        status: 502,
        headers: { "Content-Type": "application/json" },
      }),
    );
    fetchMock.mockImplementationOnce(async () =>
      new Response(null, { status: 204 }),
    );

    await connectDesktop();
    expect(current?.connectionState).toBe("error");
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/desktop/session/desktop-session-1",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("terminates an active desktop session when the viewer unmounts", async () => {
    await connectDesktop();
    fetchMock.mockClear();

    await act(async () => {
      root.unmount();
      await Promise.resolve();
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/desktop/session/desktop-session-1",
      expect.objectContaining({ method: "DELETE", keepalive: true }),
    );

    root = createRoot(container);
  });
});
