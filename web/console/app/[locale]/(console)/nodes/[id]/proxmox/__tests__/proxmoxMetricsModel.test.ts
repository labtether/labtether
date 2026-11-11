import { describe, expect, it } from "vitest";

import { metricPoints } from "../proxmoxMetricsModel";

// The Hub answers GET /proxmox/assets/{id}/metrics with {"points": [...]}; the
// tab read only "data" and showed "No samples in this range" for every VM.
describe("Proxmox metrics answer", () => {
  it("reads the Hub's points", () => {
    const points = metricPoints({ points: [{ time: 1, cpu: 0.1, memused: 512 }], timeframe: "hour" });
    expect(points).toHaveLength(1);
    expect(points[0].memused).toBe(512);
  });

  it("still reads an older data field and a bare list", () => {
    expect(metricPoints({ data: [{ time: 2 }] })).toHaveLength(1);
    expect(metricPoints([{ time: 3 }])).toHaveLength(1);
    expect(metricPoints(null)).toEqual([]);
  });
});
