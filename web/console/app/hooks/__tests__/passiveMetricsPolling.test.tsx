import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useLatency } from "../useLatency";
import { useViewerMetrics, type UseViewerMetricsOptions } from "../useViewerMetrics";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let container: HTMLDivElement;
let visibility: DocumentVisibilityState;
let latency: number | null;
let metrics: ReturnType<typeof useViewerMetrics>;
let renders: number;

function Latency({ enabled = true }: { enabled?: boolean }) {
  latency = useLatency(enabled);
  return null;
}

function Viewer(props: UseViewerMetricsOptions) {
  metrics = useViewerMetrics(props);
  renders++;
  return null;
}

async function show(value: DocumentVisibilityState) {
  visibility = value;
  await act(async () => document.dispatchEvent(new Event("visibilitychange")));
}

beforeEach(() => {
  vi.useFakeTimers();
  visibility = "visible";
  renders = 0;
  vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("passive metrics stay idle when hidden", () => {
  it("pauses health requests while hidden and refreshes on return", async () => {
    const fetchMock = vi.fn(async () => new Response("ok"));
    vi.stubGlobal("fetch", fetchMock);
    visibility = "hidden";
    await act(async () => root.render(<Latency />));
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(fetchMock).not.toHaveBeenCalled();
    await show("visible");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await show("hidden");
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
    await show("visible");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not stack slow health requests or publish a disabled request", async () => {
    let finish!: (response: Response) => void;
    const fetchMock = vi.fn(() => new Promise<Response>((resolve) => { finish = resolve; }));
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<Latency />));
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const signal = (fetchMock.mock.calls[0] as unknown as [string, RequestInit])[1].signal;
    await act(async () => root.render(<Latency enabled={false} />));
    expect(signal?.aborted).toBe(true);
    await act(async () => finish(new Response("ok")));
    expect(latency).toBeNull();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("stops VNC frame callbacks and timers when hidden", async () => {
    const requestFrame = vi.spyOn(window, "requestAnimationFrame").mockReturnValue(42);
    const cancelFrame = vi.spyOn(window, "cancelAnimationFrame");
    await act(async () => root.render(<Viewer protocol="vnc" connected />));
    expect(requestFrame).toHaveBeenCalledTimes(1);
    await show("hidden");
    expect(cancelFrame).toHaveBeenCalledWith(42);
    expect(vi.getTimerCount()).toBe(0);
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(requestFrame).toHaveBeenCalledTimes(1);
    await show("visible");
    expect(requestFrame).toHaveBeenCalledTimes(2);
  });

  it("coalesces slow WebRTC reads and rejects results from a hidden view", async () => {
    let finish!: (report: RTCStatsReport) => void;
    const getStats = vi.fn(() => new Promise<RTCStatsReport>((resolve) => { finish = resolve; }));
    const peer = { getStats } as unknown as RTCPeerConnection;
    await act(async () => root.render(<Viewer protocol="webrtc" connected peerConnection={peer} />));
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(getStats).toHaveBeenCalledTimes(1);
    const initial = metrics;
    await show("hidden");
    await act(async () => finish(new Map() as unknown as RTCStatsReport));
    expect(metrics).toBe(initial);
    expect(vi.getTimerCount()).toBe(0);
    await show("visible");
    expect(getStats).toHaveBeenCalledTimes(2);
  });

  it("keeps unchanged WebRTC readings stable between polls", async () => {
    const report = new Map([["video", { type: "inbound-rtp", kind: "video", framesPerSecond: 30 }]]);
    const getStats = vi.fn(async () => report);
    const peer = { getStats } as unknown as RTCPeerConnection;
    await act(async () => root.render(<Viewer protocol="webrtc" connected peerConnection={peer} />));
    const first = metrics;
    await act(async () => vi.advanceTimersByTimeAsync(2000));
    const settledRenders = renders;
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(metrics).toBe(first);
    expect(metrics.fps).toBe(30);
    expect(renders).toBe(settledRenders);
    expect(getStats.mock.calls.length).toBeGreaterThan(2);
  });
});
