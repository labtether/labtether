import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ChartPoint } from "../nodeMetricsChartGeometry";
import { timeTicks } from "../metricsTimeTicks";
import { useNodeTelemetryData } from "../useNodeTelemetryData";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
let current: ReturnType<typeof useNodeTelemetryData>;
let fetchMock: ReturnType<typeof vi.fn>;

function Harness() {
  current = useNodeTelemetryData({ activeTab: "overview", nodeId: "nas01" });
  return null;
}

function lastQuery(): URLSearchParams {
  const call = fetchMock.mock.calls.at(-1) as unknown[] | undefined;
  return new URL(String(call?.[0]), "http://console.test").searchParams;
}

beforeEach(async () => {
  fetchMock = vi.fn(async () => new Response(JSON.stringify({ series: [] })));
  vi.stubGlobal("fetch", fetchMock);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => { root.render(<Harness />); });
});

afterEach(async () => {
  await act(async () => { root.unmount(); });
  container.remove();
  vi.unstubAllGlobals();
});

// The Hub reads `window` as a Go duration, which has no day unit. Sent "7d", it answers with one hour and the
// 7d chart would show an hour without saying so.
describe("device chart range request", () => {
  it.each([["7d", "168h"], ["30d", "720h"]] as const)("asks for %s as %s and leaves the step to the Hub", async (range, hours) => {
    await act(async () => { current.setTelemetryWindow(range); });
    expect(lastQuery().get("window")).toBe(hours);
    expect(lastQuery().has("step")).toBe(false);
  });
});

// Clock times repeat every day: a week of "00:00" labels does not say which day a spike was on.
describe("chart time labels", () => {
  const clock = /^\d{2}:\d{2}$/;
  const points = (hours: number, stepMinutes: number): ChartPoint[] => {
    const count = (hours * 60) / stepMinutes;
    return Array.from({ length: count + 1 }, (_, i) => ({ x: (i / count) * 100, y: 50, value: 1, ts: 1793530800 + i * stepMinutes * 60 }));
  };

  it("uses clock times for a day", () => {
    const labels = timeTicks(points(24, 1)).map((tick) => tick.label);
    expect(labels.length).toBeGreaterThan(1);
    expect(labels.every((label) => clock.test(label))).toBe(true);
  });

  it.each([[7, 10], [30, 30]])("uses a few distinct dates for %i days", (days, stepMinutes) => {
    const labels = timeTicks(points(days * 24, stepMinutes)).map((tick) => tick.label);
    expect(labels.length).toBeGreaterThanOrEqual(2);
    expect(labels.length).toBeLessThanOrEqual(4);
    expect(labels.some((label) => clock.test(label))).toBe(false);
    expect(new Set(labels).size).toBe(labels.length);
  });
});
