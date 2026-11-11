import { describe, expect, it } from "vitest";

import { deviceFreshnessLabel, parseIntSetting } from "../formatters";

describe("parseIntSetting", () => {
  it("accepts strict positive decimal values", () => {
    expect(parseIntSetting("15", 5)).toBe(15);
    expect(parseIntSetting(" 120 ", 5)).toBe(120);
  });

  it("rejects malformed numeric-looking values", () => {
    expect(parseIntSetting("15junk", 5)).toBe(5);
    expect(parseIntSetting("1e3", 5)).toBe(5);
    expect(parseIntSetting("+15", 5)).toBe(5);
  });

  it("rejects empty and non-positive values", () => {
    expect(parseIntSetting("", 5)).toBe(5);
    expect(parseIntSetting("0", 5)).toBe(5);
  });
});

// Devices of a 120 s connection flapped "Not responding" against the 65 s agent limit, and a stopped connection
// read the same as a slow one. The connection's interval sets the limits now.
describe("deviceFreshnessLabel", () => {
  const seenSecondsAgo = (seconds: number) => new Date(Date.now() - seconds * 1000).toISOString();

  it("keeps a 120 s connection's device online between its syncs", () => {
    const asset = { last_seen_at: seenSecondsAgo(150), metadata: { collector_interval_seconds: "120" } };
    expect(deviceFreshnessLabel(asset)).toBe("online");
  });

  it("marks a 60 s connection's device offline after five missed syncs", () => {
    const asset = { last_seen_at: seenSecondsAgo(600), metadata: { collector_interval_seconds: "60" } };
    expect(deviceFreshnessLabel(asset)).toBe("offline");
  });

  it("keeps the agent limits for devices without an interval", () => {
    expect(deviceFreshnessLabel({ last_seen_at: seenSecondsAgo(150) })).toBe("unresponsive");
  });
});
