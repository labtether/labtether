import { describe, it, expect } from "vitest";
import { getStatusColors, getStatusLabel, statusConfig } from "../status";

describe("getStatusLabel", () => {
  it("returns mapped label for known statuses", () => {
    expect(getStatusLabel("firing")).toBe("Active");
    expect(getStatusLabel("resolved")).toBe("Resolved");
    expect(getStatusLabel("online")).toBe("Online");
    expect(getStatusLabel("offline")).toBe("Offline");
    expect(getStatusLabel("critical")).toBe("Critical");
  });

  it("is case-insensitive", () => {
    expect(getStatusLabel("FIRING")).toBe("Active");
    expect(getStatusLabel("Online")).toBe("Online");
  });

  it("returns raw status for unknown values", () => {
    expect(getStatusLabel("unknown-status")).toBe("unknown-status");
  });
});

describe("getStatusColors", () => {
  it("returns correct color classes for known statuses", () => {
    const online = getStatusColors("online");
    expect(online.dot).toContain("ok");

    const offline = getStatusColors("offline");
    expect(offline.dot).toContain("bad");

    const stale = getStatusColors("stale");
    expect(stale.dot).toContain("warn");
  });

  it("returns zinc fallback for unknown statuses", () => {
    const unknown = getStatusColors("nonexistent");
    expect(unknown.dot).toContain("muted");
  });
});

describe("statusConfig", () => {
  it("has entries for all expected severity levels", () => {
    expect(statusConfig.critical).toBeDefined();
    expect(statusConfig.major).toBeDefined();
    expect(statusConfig.warning).toBeDefined();
    expect(statusConfig.minor).toBeDefined();
    expect(statusConfig.info).toBeDefined();
  });

  it("has entries for all asset freshness states", () => {
    expect(statusConfig.online).toBeDefined();
    expect(statusConfig.stale).toBeDefined();
    expect(statusConfig.offline).toBeDefined();
  });

  it("has entries for action/run statuses", () => {
    expect(statusConfig.succeeded).toBeDefined();
    expect(statusConfig.failed).toBeDefined();
    expect(statusConfig.running).toBeDefined();
    expect(statusConfig.queued).toBeDefined();
  });
});
