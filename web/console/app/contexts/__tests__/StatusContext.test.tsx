import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../AuthContext", () => ({ useAuth: () => ({ user: { role: "viewer" } }) }));
vi.mock("../../hooks/useRoutePerfTelemetry", () => ({
  currentRoutePerfName: () => null,
  reportRoutePerfMetric: vi.fn(),
}));

import { StatusProvider, useSlowStatus, useStatus, useStatusAssetNameMap } from "../StatusContext";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

function payload(group = "all", name = "Live node", cpu = 10) {
  return {
    timestamp: `2026-10-08T00:00:${cpu}Z`,
    summary: { servicesUp: 1, servicesTotal: 2, assetCount: 1, staleAssetCount: 0, groupCount: 1 },
    endpoints: [],
    assets: [{ id: `${group}-node`, name, status: "online", type: "host", source: "agent", group_id: group }],
    telemetryOverview: [{ asset_id: `${group}-node`, status: "online", metrics: { cpu_used_percent: cpu } }],
    groups: [{ id: group, name: `${group} group` }],
  };
}

function json(value: unknown, etag = '"current"'): Response {
  return new Response(JSON.stringify(value), { headers: { "Content-Type": "application/json", etag } });
}

function deferredBody(body: Promise<unknown>, etag: string): Response {
  return { ok: true, status: 200, headers: new Headers({ etag }), json: () => body } as Response;
}

let root: Root;
let container: HTMLDivElement;
let current: ReturnType<typeof useStatus>;
let slow: ReturnType<typeof useSlowStatus>;
let names: Map<string, string>;
let fetchMock: ReturnType<typeof vi.fn>;

function Consumer() {
  current = useStatus();
  slow = useSlowStatus();
  names = useStatusAssetNameMap();
  return null;
}

async function mount() {
  await act(async () => {
    root.render(<StatusProvider><Consumer /></StatusProvider>);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  vi.stubGlobal("WebSocket", class { close() {} });
  fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), "https://console.example.test");
    if (url.pathname === "/api/ws/events") return json({ wsUrl: "wss://console.example.test/events" });
    return json(payload(url.searchParams.get("group_id") ?? "all"));
  });
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

describe("StatusProvider data ownership", () => {
  it("keeps newer live data when the full response arrives, then retains the slow view on a live-only change", async () => {
    const full = deferred<Response>();
    let live = payload();
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      if (String(input) === "/api/status") return full.promise;
      if (String(input) === "/api/status/live") return json(live);
      return json({});
    });
    await mount();
    expect(current.status?.assets[0].name).toBe("Live node");
    expect(current.status?.groups).toEqual([]);

    await act(async () => { full.resolve(json(payload("all", "Old full node", 5))); });
    expect(current.status?.assets[0].name).toBe("Live node");
    expect(current.status?.groups[0].name).toBe("all group");
    const previousSlow = slow;
    const previousNames = names;
    live = payload("all", "Live node", 20);
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => String(input) === "/api/status"
      ? new Response(null, { status: 304 }) : json(live));
    await act(async () => { await current.fetchStatus(); });

    expect(current.status?.telemetryOverview[0].metrics.cpu_used_percent).toBe(20);
    expect(slow).toBe(previousSlow);
    expect(names).toBe(previousNames);
    expect(fetchMock).toHaveBeenCalledWith("/api/status", expect.objectContaining({ headers: { "If-None-Match": '"current"' } }));
  });

  it("discards an old group's parsed response without unlocking the new group's pending request", async () => {
    const oldBody = deferred<unknown>();
    const newBody = deferred<unknown>();
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/status") return deferredBody(oldBody.promise, '"old"');
      if (url === "/api/status?group_id=new") return deferredBody(newBody.promise, '"new"');
      if (url.includes("/api/status/live")) return json(payload(url.includes("group_id=new") ? "new" : "all"));
      return json({});
    });
    await mount();
    await act(async () => { current.setSelectedGroupFilter("new"); });
    expect(fetchMock).toHaveBeenCalledWith("/api/status?group_id=new", expect.objectContaining({ headers: undefined }));
    expect(current.status?.assets[0].id).toBe("new-node");

    await act(async () => { oldBody.resolve(payload("old")); });
    await act(async () => { await current.fetchStatus(); });
    expect(fetchMock.mock.calls.filter(([url]) => url === "/api/status?group_id=new")).toHaveLength(1);
    expect(current.status?.groups).toEqual([]);

    await act(async () => { newBody.resolve(payload("new")); });
    expect(current.status?.groups[0].id).toBe("new");
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => String(input) === "/api/status?group_id=new"
      ? new Response(null, { status: 304 }) : json(payload("new")));
    await act(async () => { await current.fetchStatus(); });
    expect(fetchMock).toHaveBeenLastCalledWith("/api/status?group_id=new", expect.objectContaining({ headers: { "If-None-Match": '"new"' } }));
  });

  it("refreshes a new group immediately while the old live request is pending, and ignores the old failure", async () => {
    const oldLive = deferred<Response>();
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/status/live") return oldLive.promise;
      if (url.startsWith("/api/status")) return json(payload(url.includes("group_id=new") ? "new" : "all"));
      return json({});
    });
    await mount();
    await act(async () => { current.setSelectedGroupFilter("new"); });
    expect(current.status?.assets[0].id).toBe("new-node");
    await act(async () => { oldLive.resolve(new Response(null, { status: 503 })); });
    expect(current.error).toBeNull();
    expect(current.status?.assets[0].id).toBe("new-node");
  });

  it("keeps the current object for unchanged polls and supplies safe defaults for missing fields", async () => {
    fetchMock.mockImplementation(async () => json({}));
    await mount();
    expect(current.status?.assets).toEqual([]);
    expect(current.status?.deadLetterAnalytics.total).toBe(0);
    const previous = current.status;
    fetchMock.mockImplementation(async () => json({}));
    await act(async () => { await current.fetchStatus(); });
    expect(current.status).toBe(previous);
    expect(current.loading).toBe(false);
    expect(current.defaultTelemetryWindow).toBe("1h");
  });
});
