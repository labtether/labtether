import { describe, expect, it } from "vitest";
import type { DeadLetterAnalytics, EndpointStatus } from "../../../../console/models";
import { groupTone, hubChecks, sortWindows, windowEffects, windowState } from "../healthModel";
import { span, when } from "../healthTime";

const analytics = (total = 0): DeadLetterAnalytics => ({
  window: "24h", bucket: "1h", total, rate_per_hour: 0, rate_per_day: 0, trend: [], top_components: [], top_subjects: [], top_error_classes: [],
});
const endpoint = (name: string, ok: boolean, url = "http://10.20.0.5:8080/healthz"): EndpointStatus =>
  ({ name, url, ok, status: ok ? "up" : "down", latencyMs: 12 });

// The Hub list must never show an all-clear the hub did not report.
describe("hubChecks", () => {
  it("reports a down endpoint, failed jobs with the newest time, and a cleanup error", () => {
    const rows = hubChecks({
      version: { version: "1.7.1", started_at: "2026-10-30T08:00:00Z" },
      endpoints: [endpoint("LabTether", true), endpoint("Node Agent", false)],
      analytics: analytics(3),
      deadLetters: [
        { id: "a", component: "worker", subject: "s", deliveries: 1, error: "e", created_at: "2026-11-01T08:00:00Z" },
        { id: "b", component: "worker", subject: "s", deliveries: 1, error: "e", created_at: "2026-11-01T09:30:00Z" },
      ],
      retentionError: "disk full",
    });
    expect(rows.map((r) => [r.key, r.tone, r.word])).toEqual([
      ["hub", "ok", "running"],
      ["endpoint:LabTether", "ok", "up"],
      ["endpoint:Node Agent", "bad", "down"],
      ["jobs", "bad", "failedCount"],
      ["cleanup", "bad", "failed"],
    ]);
    expect(rows.find((r) => r.key === "jobs")?.at).toBe("2026-11-01T09:30:00Z");
    expect(rows.find((r) => r.key === "endpoint:Node Agent")?.latencyMs).toBeUndefined();
  });

  it("is all clear only with no failures and no cleanup error, and skips the hub row without a version", () => {
    const rows = hubChecks({ version: null, endpoints: [], analytics: analytics(0), deadLetters: [], retentionError: "" });
    expect(rows.map((r) => [r.key, r.tone])).toEqual([["jobs", "ok"], ["cleanup", "ok"]]);
  });
});

describe("groupTone", () => {
  it("follows the hub's grade lines and puts downtime first", () => {
    expect(groupTone({ score: 90, maintenance_active: false }).word).toBe("good");
    expect(groupTone({ score: 75, maintenance_active: false }).word).toBe("fair");
    expect(groupTone({ score: 74, maintenance_active: false }).word).toBe("poor");
    expect(groupTone({ score: 20, maintenance_active: true })).toEqual({ tone: "busy", word: "downtime" });
  });
});

// Running downtime must lead the list; ended windows sink to the bottom.
describe("downtime windows", () => {
  const now = Date.parse("2026-11-01T12:00:00Z");
  const w = (id: string, start: string, end: string) => ({ id, start_at: `2026-11-${start}Z`, end_at: `2026-11-${end}Z` });
  const list = [w("old", "01T01:00:00", "01T02:00:00"), w("later", "03T23:00:00", "04T01:00:00"), w("now", "01T11:00:00", "01T13:00:00"), w("next", "01T23:00:00", "02T01:00:00")];

  it("knows running, upcoming and ended", () => {
    expect(list.map((x) => windowState(x, now))).toEqual(["ended", "upcoming", "active", "upcoming"]);
  });

  it("sorts running first, then by start, then ended", () => {
    expect(sortWindows(list, now).map((x) => x.id)).toEqual(["now", "next", "later", "old"]);
  });

  it("names what a window does", () => {
    expect(windowEffects({ suppress_alerts: true, block_actions: false, block_updates: true })).toEqual(["mutes", "pausesUpdates"]);
  });
});

describe("times", () => {
  const now = new Date(2026, 10, 1, 12, 0).getTime();
  it("shows a clock today, a weekday this week and a date further out", () => {
    expect(when(new Date(2026, 10, 1, 9, 5).toISOString(), now, "en-GB")).toBe("09:05");
    expect(when(new Date(2026, 9, 30, 21, 12).toISOString(), now, "en-GB")).toMatch(/^Fri 21:12$/);
    expect(when(new Date(2025, 9, 12, 8, 0).toISOString(), now, "en-GB")).toBe("12 Oct 2025");
  });

  it("keeps one date for an overnight span", () => {
    expect(span(new Date(2026, 10, 1, 23, 0).toISOString(), new Date(2026, 10, 2, 1, 0).toISOString(), now, "en-GB")).toBe("23:00 – 01:00");
  });
});
