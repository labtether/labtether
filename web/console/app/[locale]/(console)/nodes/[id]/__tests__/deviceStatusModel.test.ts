import { describe, expect, it } from "vitest";

import { deviceState, parseBackupAgeDays } from "../deviceStatusModel";

// The header and Details must never call a device healthy when the hub says otherwise, and must notice a device
// that stopped checking in even while its last reported status was healthy.
const secondsAgo = (s: number) => new Date(Date.now() - s * 1000).toISOString();

describe("deviceState", () => {
  it("says Offline when the hub reports offline despite a fresh check-in", () => {
    expect(deviceState({ status: "offline", last_seen_at: secondsAgo(5) }).word).toBe("offline");
  });

  it.each(["unknown", "unavailable"])("says Offline for an explicit %s status despite a fresh check-in", (status) => {
    expect(deviceState({ status, last_seen_at: secondsAgo(5) }).word).toBe("offline");
  });

  it("says Not responding for a degraded status despite a fresh check-in", () => {
    const state = deviceState({ status: "degraded", last_seen_at: secondsAgo(5) });
    expect(state).toMatchObject({ tone: "warn", word: "notResponding" });
  });

  it("lets a stale check-in override a healthy status, keeping the time it was last seen", () => {
    const seen = secondsAgo(120);
    const state = deviceState({ status: "online", last_seen_at: seen });
    expect(state.word).toBe("notResponding");
    expect(state.since?.toISOString()).toBe(seen);
  });

  it("says Online, with no time, for a fresh healthy device", () => {
    expect(deviceState({ status: "online", last_seen_at: secondsAgo(5) })).toEqual({ tone: "ok", word: "online", since: null });
  });

  it("says No check-in yet when the device never checked in", () => {
    expect(deviceState({ status: "online", last_seen_at: "" }).word).toBe("noCheckIn");
  });
});

describe("parseBackupAgeDays", () => {
  it("accepts strict non-negative decimal day values", () => {
    expect(parseBackupAgeDays("0")).toBe(0);
    expect(parseBackupAgeDays(" 1.5 ")).toBe(1.5);
    expect(parseBackupAgeDays("30")).toBe(30);
  });

  it("rejects malformed numeric-looking values", () => {
    expect(parseBackupAgeDays("1e2")).toBeNull();
    expect(parseBackupAgeDays("30abc")).toBeNull();
    expect(parseBackupAgeDays("+30")).toBeNull();
    expect(parseBackupAgeDays("-1")).toBeNull();
  });
});
