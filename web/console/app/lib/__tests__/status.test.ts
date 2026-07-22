import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Asset } from "../../console/models";
import { assetFreshness, buildDeviceCardSections } from "../../[locale]/(console)/nodes/nodesPageUtils";
import {
  getStatusColors,
  getStatusLabel,
  resolveAssetPresentationStatus,
  statusConfig,
} from "../status";

const observedAt = new Date("2026-07-22T09:47:01.215Z");

function assetWithStatus(status: string, lastSeenAt = "2026-07-22T09:46:47.722Z"): Asset {
  return {
    id: "homeassistant-hub-disposable-ha-qa-r17",
    type: "connector-cluster",
    name: "Disposable HA QA r17",
    source: "homeassistant",
    status,
    last_seen_at: lastSeenAt,
    metadata: {
      collector_id: "collector-ha",
      discovered: "17",
    },
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(observedAt);
});

afterEach(() => {
  vi.useRealTimers();
});

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

describe("resolveAssetPresentationStatus", () => {
  it.each(["offline", "down", "critical", "error", "unknown", "unavailable"])(
    "keeps explicit %s status offline despite fresh telemetry",
    (status) => {
      expect(resolveAssetPresentationStatus(status, "online")).toBe("offline");
    },
  );

  it.each(["warning", "degraded", "restarting", "stale", "unresponsive"])(
    "keeps explicit %s status unresponsive despite fresh telemetry",
    (status) => {
      expect(resolveAssetPresentationStatus(status, "online")).toBe("unresponsive");
    },
  );

  it("uses timestamp freshness when the explicit status is healthy", () => {
    expect(resolveAssetPresentationStatus("online", "unresponsive")).toBe("unresponsive");
    expect(resolveAssetPresentationStatus("online", "online")).toBe("online");
  });
});

describe("device-list asset status", () => {
  it("does not show a freshly updated offline Home Assistant parent as online", () => {
    const asset = assetWithStatus("offline");

    expect(assetFreshness(asset)).toBe("offline");

    const sections = buildDeviceCardSections({
      assets: [asset],
      telemetryOverview: [],
      query: "",
      groupLabelByID: new Map(),
    });

    expect(sections).toHaveLength(1);
    expect(sections[0]?.devices).toHaveLength(1);
    expect(sections[0]?.devices[0]?.freshness).toBe("offline");
    expect(sections[0]?.counts).toMatchObject({ online: 0, offline: 1, issues: 1 });
  });

  it("still lets stale timestamp freshness downgrade an explicitly healthy asset", () => {
    expect(assetFreshness(assetWithStatus("online", "2026-07-22T09:45:01.215Z"))).toBe("unresponsive");
  });

  it("keeps a fresh explicitly healthy asset online", () => {
    expect(assetFreshness(assetWithStatus("online"))).toBe("online");
  });
});
