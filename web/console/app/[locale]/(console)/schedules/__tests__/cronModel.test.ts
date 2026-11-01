import { describe, expect, it } from "vitest";
import { nextRun, presetExpr, presetOf, splitZone } from "../cronModel";

// Risk: the editor previews a next run the hub would not use, or accepts a line the hub refuses.
const after = new Date("2026-10-27T22:40:00Z"); // a Tuesday

describe("nextRun", () => {
  it("reads plain cron in UTC, as the hub does", () => {
    expect(nextRun("0 2 * * *", after)?.toISOString()).toBe("2026-10-28T02:00:00.000Z");
    expect(nextRun("*/15 * * * *", after)?.toISOString()).toBe("2026-10-27T22:45:00.000Z");
    expect(nextRun("@hourly", after)?.toISOString()).toBe("2026-10-27T23:00:00.000Z");
  });

  it("uses the CRON_TZ zone", () => {
    // 02:00 in Berlin on 28 Oct 2026 (CET, UTC+1) is 01:00 UTC.
    expect(nextRun("CRON_TZ=Europe/Berlin 0 2 * * *", after)?.toISOString()).toBe("2026-10-28T01:00:00.000Z");
  });

  it("matches day of month or weekday when both are set, and both when one is *", () => {
    // Saturdays: 31 Oct 2026.
    expect(nextRun("0 10 * * SAT", after)?.toISOString()).toBe("2026-10-31T10:00:00.000Z");
    // The 1st or a Thursday: Thursday 29 Oct comes first.
    expect(nextRun("0 4 1 * 4", after)?.toISOString()).toBe("2026-10-29T04:00:00.000Z");
    expect(nextRun("0 4 1 * *", after)?.toISOString()).toBe("2026-11-01T04:00:00.000Z");
  });

  it("finds a run years away and refuses lines the hub refuses", () => {
    expect(nextRun("0 0 29 2 *", after)?.toISOString()).toBe("2028-02-29T00:00:00.000Z");
    expect(nextRun("0 2 * *", after)).toBeNull();
    expect(nextRun("0 24 * * *", after)).toBeNull();
    expect(nextRun("0 2 * * 7", after)).toBeNull();
    expect(nextRun("@every 5m", after)).toBeNull();
    expect(nextRun("CRON_TZ=Nowhere/Town 0 2 * * *", after)).toBeNull();
    expect(nextRun("0 0 30 2 *", after)).toBeNull();
  });
});

describe("presets", () => {
  it("save in the browser zone and are recognised only in that zone", () => {
    expect(presetExpr("daily", "UTC")).toBe("0 2 * * *");
    const berlin = presetExpr("daily", "Europe/Berlin");
    expect(berlin).toBe("CRON_TZ=Europe/Berlin 0 2 * * *");
    expect(presetOf(berlin, "Europe/Berlin")).toBe("daily");
    expect(presetOf(berlin, "America/Chicago")).toBe("");
    expect(presetOf("0  2 * * *", "UTC")).toBe("daily");
    expect(presetOf("0 9 * * *", "UTC")).toBe("");
    expect(splitZone("TZ=Asia/Tokyo 0 3 * * 0")).toEqual({ zone: "Asia/Tokyo", spec: "0 3 * * 0" });
  });
});
