import { describe, expect, it } from "vitest";
import type { AssetTelemetryDetails } from "../../../../../console/models";
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
