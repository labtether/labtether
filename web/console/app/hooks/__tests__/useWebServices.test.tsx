import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../useRoutePerfTelemetry", () => ({ reportRoutePerfMetric: vi.fn() }));

import { useWebServices, type UseWebServicesOptions } from "../useWebServices";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
let current: ReturnType<typeof useWebServices>;
let fetchMock: ReturnType<typeof vi.fn>;
let visibility: DocumentVisibilityState;

function response(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), { status });
}

function service(overrides: Record<string, unknown> = {}) {
  return {
    id: "service-1", host_asset_id: "host-1", name: "Home App", service_key: "home-app",
    url: "https://home.example.test", status: "up", response_ms: 12,
    metadata: { source: "docker" },
    health: { window: "24h", checks: 1, up_checks: 1, uptime_percent: 100,
      recent: [{ at: "2026-10-08T00:00:00Z", status: "up", response_ms: 12 }] },
    alt_urls: [{ id: "alt-1", url: "https://alternate.example.test" }],
    ...overrides,
  };
}

function Harness({ options }: { options: UseWebServicesOptions }) {
  current = useWebServices(options);
  return null;
}

async function mount(options: UseWebServicesOptions = {}) {
  await act(async () => { root.render(<Harness options={options} />); });
}

async function setVisibility(value: DocumentVisibilityState) {
  visibility = value;
  await act(async () => { document.dispatchEvent(new Event("visibilitychange")); });
}

beforeEach(() => {
  vi.useFakeTimers();
  visibility = "visible";
  vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
  vi.spyOn(window, "requestAnimationFrame").mockReturnValue(1);
  fetchMock = vi.fn(async () => response({ services: [] }));
  vi.stubGlobal("fetch", fetchMock);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => { root.unmount(); });
  container.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("useWebServices list lifecycle", () => {
  it("keeps unchanged services and nested data stable across polls", async () => {
    const payload = {
      services: [service(), service({ host_asset_id: "host-2", name: "Other host" })],
      discovery_stats: [{ host_asset_id: "host-1", last_seen: "now", discovery: {
        collected_at: "now", total_services: 2, cycle_duration_ms: 12,
        sources: { docker: { enabled: true, duration_ms: 12, services_found: 2 } },
        final_source_count: { docker: 2 },
      } }],
      suggestions: [{ id: "suggestion-1", suggested_url: "https://other.example.test" }],
    };
    fetchMock.mockImplementation(async () => response(payload));
    await mount();
    const before = current;
    await act(async () => { await current.refresh(); });
    expect(current.services).toBe(before.services);
    expect(current.discoveryStats).toBe(before.discoveryStats);
    expect(current.suggestions).toBe(before.suggestions);

    payload.services[0].response_ms = 20;
    await act(async () => { await current.refresh(); });
    expect(current.services).not.toBe(before.services);
    expect(current.services[0].response_ms).toBe(20);
    expect(current.services[0].metadata).toBe(before.services[0].metadata);
    expect(current.services[0].health).toBe(before.services[0].health);
    expect(current.services[0].alt_urls).toBe(before.services[0].alt_urls);
    expect(current.services[1]).toBe(before.services[1]);
  });

  it("normalizes malformed collections and keeps internal API services hidden by default", async () => {
    fetchMock.mockImplementation(async () => response({
      services: [null, "bad", service({ metadata: null }),
        service({ id: "api", service_key: "labtether", url: "http://host:8080", metadata: null }),
        service({ id: "console", service_key: "labtether", metadata: { labtether_component: "console" } })],
      discovery_stats: [null, { discovery: null }, { host_asset_id: 12, discovery: {
        total_services: "bad", sources: { bad: null }, final_source_count: { docker: "bad" },
      } }],
      suggestions: [null, {}, { id: "suggestion", confidence: "bad" }],
    }));
    await mount();
    expect(current.services.map(({ id }) => id)).toEqual(["service-1", "console"]);
    expect(current.services[0].metadata).toBeUndefined();
    expect(current.discoveryStats).toEqual([{ host_asset_id: "", last_seen: "", discovery: {
      collected_at: "", cycle_duration_ms: 0, total_services: 0,
      sources: undefined, final_source_count: undefined,
    } }]);
    expect(current.suggestions).toEqual([expect.objectContaining({ id: "suggestion", confidence: 0 })]);

    await mount({ includeHidden: true });
    expect(current.services.map(({ id }) => id)).toEqual(["service-1", "api", "console"]);
    expect(fetchMock.mock.lastCall?.[0]).toBe("/api/services/web?include_hidden=true");

    fetchMock.mockResolvedValueOnce(response({ services: null, discovery_stats: {}, suggestions: "bad" }));
    await act(async () => { await current.refresh(); });
    expect(current.services).toEqual([]);
    expect(current.discoveryStats).toEqual([]);
    expect(current.suggestions).toEqual([]);
  });

  it("polls only while visible and aborts outstanding requests on hide and unmount", async () => {
    visibility = "hidden";
    const signals: AbortSignal[] = [];
    fetchMock.mockImplementation((_url: string, init: RequestInit) => {
      const signal = init.signal as AbortSignal;
      signals.push(signal);
      return new Promise<Response>((_resolve, reject) => {
        signal.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")));
      });
    });
    await mount({ pollInterval: 100 });
    expect(fetchMock).not.toHaveBeenCalled();
    await setVisibility("visible");
    expect(signals).toHaveLength(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(100); });
    expect(signals).toHaveLength(2);
    expect(signals[0].aborted).toBe(true);
    await setVisibility("hidden");
    expect(signals[1].aborted).toBe(true);
    await act(async () => { await vi.advanceTimersByTimeAsync(500); });
    expect(signals).toHaveLength(2);
    expect(current.error).toBeNull();
    await setVisibility("visible");
    expect(signals).toHaveLength(3);
    await act(async () => { root.unmount(); });
    expect(signals[2].aborted).toBe(true);
    root = createRoot(container);
  });

  it("retains the last good list on errors and clears the error after recovery", async () => {
    fetchMock.mockResolvedValueOnce(response({ services: [service()] }));
    await mount();
    const services = current.services;
    fetchMock.mockResolvedValueOnce(response({ error: "unavailable" }, 503));
    await act(async () => { await current.refresh(); });
    expect(current.error).toBe("HTTP 503");
    expect(current.loading).toBe(false);
    expect(current.services).toBe(services);
    fetchMock.mockRejectedValueOnce(new Error("offline"));
    await act(async () => { await current.refresh(); });
    expect(current.error).toBe("offline");
    expect(current.services).toBe(services);
    await act(async () => { await current.refresh(); });
    expect(current.error).toBeNull();
  });
});

describe("useWebServices detail and commands", () => {
  it("loads full details separately from the compact list and respects host changes", async () => {
    fetchMock.mockResolvedValueOnce(response({ services: [service({ health: undefined, alt_urls: undefined })] }));
    await mount({ host: "host-1", detailLevel: "compact", includeHidden: true });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/services/web?host=host-1&include_hidden=true&detail=compact");
    const services = current.services;
    fetchMock.mockResolvedValueOnce(response({ services: [service()] }));
    const details = await current.loadServiceDetails("service-1", "host/override");
    expect(fetchMock.mock.lastCall?.[0]).toBe("/api/services/web?host=host%2Foverride&include_hidden=true&service_id=service-1");
    expect(details?.health?.checks).toBe(1);
    expect(current.services).toBe(services);
    expect(current.services[0].health).toBeUndefined();

    await mount({ host: "host-2", detailLevel: "compact" });
    expect(fetchMock.mock.lastCall?.[0]).toBe("/api/services/web?host=host-2&detail=compact");
    fetchMock.mockResolvedValueOnce(response({ services: [service({ host_asset_id: "host-2" })] }));
    await current.loadServiceDetails("service-1");
    expect(fetchMock.mock.lastCall?.[0]).toBe("/api/services/web?host=host-2&service_id=service-1");
    fetchMock.mockResolvedValueOnce(response({ error: "service gone" }, 404));
    await expect(current.loadServiceDetails("service-1")).rejects.toThrow("service gone");
    fetchMock.mockResolvedValueOnce(response({ services: [] }));
    await expect(current.loadServiceDetails("service-1")).resolves.toBeNull();
  });

  it("shares sync failures with the list and resets syncing on both exit paths", async () => {
    await mount({ host: "host-1" });
    let finish!: (value: Response) => void;
    fetchMock.mockImplementationOnce(() => new Promise<Response>((resolve) => { finish = resolve; }));
    let sync!: Promise<void>;
    await act(async () => { sync = current.sync("host/override"); });
    expect(current.syncing).toBe(true);
    expect(fetchMock.mock.lastCall?.[0]).toBe("/api/services/web/sync?host=host%2Foverride");
    await act(async () => {
      finish(response({ error: "agent unavailable" }, 502));
      await expect(sync).rejects.toThrow("agent unavailable");
    });
    expect(current.syncing).toBe(false);
    expect(current.error).toBe("agent unavailable");
    const requests = fetchMock.mock.calls.length;
    await act(async () => { await current.sync(); });
    expect(fetchMock.mock.calls).toHaveLength(requests + 1);
    expect(fetchMock.mock.lastCall?.[0]).toBe("/api/services/web/sync?host=host-1");
    expect(current.syncing).toBe(false);
    expect(current.error).toBeNull();
  });

  it("keeps edit helpers stable, escapes IDs and preserves API errors", async () => {
    await mount();
    const update = current.updateManualService;
    await mount({ host: "another-host" });
    expect(current.updateManualService).toBe(update);
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await current.updateManualService("service/one", { name: "Renamed" });
    expect(fetchMock).toHaveBeenLastCalledWith("/api/services/web/manual/service%2Fone", {
      method: "PATCH", cache: "no-store", headers: { "content-type": "application/json" },
      body: JSON.stringify({ name: "Renamed" }),
    });
    fetchMock.mockResolvedValueOnce(response({ error: "name required" }, 400));
    await expect(current.createManualService({ host_asset_id: "host-1", name: "", category: "", url: "" }))
      .rejects.toThrow("name required");
    fetchMock.mockResolvedValueOnce(new Response("not JSON", { status: 502 }));
    await expect(current.deleteManualService("service/one")).rejects.toThrow("HTTP 502");
    fetchMock.mockResolvedValueOnce(response({ overrides: [null, {}, {
      host_asset_id: "host-1", service_id: "service-1", hidden: true, name_override: 123,
    }] }));
    await expect(current.listServiceOverrides("host-1")).resolves.toEqual([
      expect.objectContaining({ host_asset_id: "host-1", service_id: "service-1", hidden: true, name_override: undefined }),
    ]);
  });

  it("validates icon responses and sends icon rename/delete IDs in the query", async () => {
    await mount();
    fetchMock.mockResolvedValueOnce(response({ icons: [null, {}, { id: "icon-1", name: "Home", data_url: 123 }] }));
    await expect(current.listCustomServiceIcons()).resolves.toEqual([
      expect.objectContaining({ id: "icon-1", name: "Home", data_url: "" }),
    ]);
    fetchMock.mockResolvedValueOnce(response({ icon: { id: "icon/one", name: "Renamed", data_url: "data:image/png;base64,AAAA" } }));
    await expect(current.renameCustomServiceIcon({ id: "icon/one", name: "Renamed" }))
      .resolves.toEqual(expect.objectContaining({ id: "icon/one", name: "Renamed" }));
    expect(fetchMock.mock.lastCall?.[0]).toBe("/api/services/web/icon-library?id=icon%2Fone");
    fetchMock.mockResolvedValueOnce(response({ icon: null }));
    await expect(current.createCustomServiceIcon({ name: "Home", data_url: "data:image/png;base64,AAAA" }))
      .rejects.toThrow("service icon create response missing icon");
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await current.deleteCustomServiceIcon("icon/one");
    expect(fetchMock).toHaveBeenLastCalledWith("/api/services/web/icon-library?id=icon%2Fone", {
      method: "DELETE", cache: "no-store",
    });
  });
});
