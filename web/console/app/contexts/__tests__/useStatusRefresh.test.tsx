import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useStatusRefresh } from "../status/useStatusRefresh";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

class FakeSocket {
  static instances: FakeSocket[] = [];
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  close = vi.fn(() => { this.onclose?.(); });
  constructor() { FakeSocket.instances.push(this); }
  message(type: string) { this.onmessage?.({ data: JSON.stringify({ type }) }); }
}

let root: Root;
let container: HTMLDivElement;
let visibility: DocumentVisibilityState;
let live: ReturnType<typeof vi.fn<(group: string) => Promise<void>>>;
let full: ReturnType<typeof vi.fn<(group: string) => Promise<void>>>;
let fetchMock: ReturnType<typeof vi.fn>;

function Harness({ group = "all" }: { group?: string }) {
  useStatusRefresh(group, 5000, live, full);
  return null;
}

async function render(group = "all") {
  await act(async () => { root.render(<Harness group={group} />); });
}

async function changeVisibility(next: DocumentVisibilityState) {
  await act(async () => {
    visibility = next;
    document.dispatchEvent(new Event("visibilitychange"));
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  visibility = "visible";
  vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
  FakeSocket.instances = [];
  vi.stubGlobal("WebSocket", FakeSocket);
  live = vi.fn(async () => {});
  full = vi.fn(async () => {});
  fetchMock = vi.fn(async () => new Response(JSON.stringify({ wsUrl: "wss://console.example.test/events" })));
  vi.stubGlobal("fetch", fetchMock);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => { root.unmount(); });
  container.remove();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("status refresh lifecycle", () => {
  it("pauses both polling schedules while hidden and refreshes immediately when visible", async () => {
    await render();
    expect(live).toHaveBeenCalledTimes(1);
    expect(full).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(live).toHaveBeenCalledTimes(2);
    expect(full).toHaveBeenCalledTimes(1);

    await changeVisibility("hidden");
    await act(async () => { await vi.advanceTimersByTimeAsync(120_000); });
    expect(live).toHaveBeenCalledTimes(2);
    expect(full).toHaveBeenCalledTimes(1);
    await changeVisibility("visible");
    expect(live).toHaveBeenCalledTimes(3);
    expect(full).toHaveBeenCalledTimes(2);
    await act(async () => { await vi.advanceTimersByTimeAsync(120_000); });
    expect(full).toHaveBeenCalledTimes(3);
  });

  it("debounces push events using the latest group without reconnecting the socket", async () => {
    await render();
    const socket = FakeSocket.instances[0];
    const alert = vi.fn();
    window.addEventListener("labtether:alert-event", alert);
    try {
      await act(async () => { socket.message("alert.fired"); socket.message("heartbeat.update"); });
      await render("new");
      live.mockClear();
      full.mockClear();
      await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
      expect(live).toHaveBeenCalledExactlyOnceWith("new");
      expect(full).not.toHaveBeenCalled();
      expect(alert).toHaveBeenCalledTimes(1);
      expect(FakeSocket.instances).toHaveLength(1);
      expect(socket.close).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("labtether:alert-event", alert);
    }
  });

  it("reconnects on visibility after a previous reconnect timer already fired", async () => {
    await render();
    await act(async () => { FakeSocket.instances[0].close(); });
    await act(async () => { await vi.advanceTimersByTimeAsync(2000); });
    expect(FakeSocket.instances).toHaveLength(2);
    await changeVisibility("hidden");
    await act(async () => { FakeSocket.instances[1].close(); });
    await changeVisibility("visible");
    expect(FakeSocket.instances).toHaveLength(3);
  });

  it("retries a failed event URL lookup while polling continues", async () => {
    fetchMock.mockRejectedValueOnce(new Error("hub starting"));
    await render();
    expect(live).toHaveBeenCalledTimes(1);
    expect(FakeSocket.instances).toHaveLength(0);
    await act(async () => { await vi.advanceTimersByTimeAsync(2000); });
    expect(FakeSocket.instances).toHaveLength(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("drops a pending push refresh while hidden and cleans up every timer on unmount", async () => {
    await render();
    await act(async () => { FakeSocket.instances[0].message("heartbeat.update"); });
    await changeVisibility("hidden");
    live.mockClear();
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(live).not.toHaveBeenCalled();
    await act(async () => { root.unmount(); });
    expect(vi.getTimerCount()).toBe(0);
    expect(FakeSocket.instances[0].close).toHaveBeenCalledTimes(1);
    root = createRoot(container);
  });

  it("does not open a socket after unmount while its URL is loading", async () => {
    let resolve!: (value: Response) => void;
    fetchMock.mockReturnValue(new Promise<Response>((done) => { resolve = done; }));
    await render();
    await act(async () => { root.unmount(); });
    await act(async () => { resolve(new Response(JSON.stringify({ wsUrl: "wss://console.example.test/events" }))); });
    expect(FakeSocket.instances).toHaveLength(0);
    expect(vi.getTimerCount()).toBe(0);
    root = createRoot(container);
  });
});
