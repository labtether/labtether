import { describe, expect, it } from "vitest";
import type { AssetTelemetryDetails } from "../../../../../console/models";
import { formatStep } from "../nodeMetricsPresentationModel";
import { analyzeTelemetryDetails } from "../nodeMetricsSeriesAnalysis";

// A busy host near 100% CPU drew its chart up to "104%"; a percent chart must top out at 100%.
const details = (values: number[]): AssetTelemetryDetails => ({
  asset: { id: "nas01", name: "nas01", type: "host", source: "agent", status: "online", last_seen_at: "2026-11-01T12:00:00Z" },
  window: "1h",
  step: "1m",
  from: "2026-11-01T11:00:00Z",
  to: "2026-11-01T12:00:00Z",
  series: [{ metric: "cpu_used_percent", unit: "percent", points: values.map((value, i) => ({ ts: 1793530800 + i * 60, value })) }],
} as AssetTelemetryDetails);

describe("percent chart scale", () => {
  it("stops at 100% when readings stay at or under 100", () => {
    const [cpu] = analyzeTelemetryDetails(details([20, 60, 99]));
    expect(cpu.yMin).toBe(0);
    expect(cpu.yMax).toBe(100);
  });

  it("leaves headroom only for a reading over 100", () => {
    const [cpu] = analyzeTelemetryDetails(details([40, 130]));
    expect(cpu.yMax).toBeGreaterThan(130);
  });
});

// The Hub prints its step as a Go duration: "1m0s", "10m0s", "30m0s". Read as nothing, the Gaps and Stale checks
// never ran on a 24h, 7d or 30d chart, and the Metrics bar said "every 10m0s".
describe("the Hub's step", () => {
  const week = (step: string, stepSeconds: number, skipFrom: number, skipTo: number): AssetTelemetryDetails => ({
    asset: { id: "nas01", name: "nas01", type: "host", source: "agent", status: "online", last_seen_at: "2026-11-08T12:00:00Z" },
    window: "168h0m0s",
    step,
    from: "2026-11-01T12:00:00Z",
    to: "2026-11-08T12:00:00Z",
    series: [{
      metric: "cpu_used_percent",
      unit: "percent",
      points: Array.from({ length: 1009 }, (_, i) => ({ ts: 1793534400 + i * stepSeconds, value: 20 + (i % 7) }))
        .filter((_, i) => i < skipFrom || i >= skipTo),
    }],
  } as AssetTelemetryDetails);

  it("flags a two-hour hole in a 7d chart", () => {
    const [cpu] = analyzeTelemetryDetails(week("10m0s", 600, 400, 412));
    expect(cpu.flags.find((flag) => flag.kind === "gaps")?.seconds).toBe(13 * 600);
  });

  it("flags nothing when every sample is there", () => {
    const [cpu] = analyzeTelemetryDetails(week("10m0s", 600, 0, 0));
    expect(cpu.flags.map((flag) => flag.kind)).not.toContain("gaps");
  });

  it("reads as short text", () => {
    expect(["15s", "1m0s", "10m0s", "30m0s", "1m30s"].map(formatStep)).toEqual(["15s", "1m", "10m", "30m", "1m 30s"]);
  });
});
