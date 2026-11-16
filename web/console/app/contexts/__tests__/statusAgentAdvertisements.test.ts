import { describe, expect, it } from "vitest";

import { mergeFullStatus, mergeLiveStatus, normalizeLiveStatusResponse, normalizeStatusResponse } from "../status/statusPayload";

// The device pages read agentAdvertisements from the status the console polls. If the normalizers or the merges drop
// it, every explicit agent looks like a Go agent again and its dead tabs come back.

const ad = { explicit: true, edition: "monitoring", offered: ["disk.list", "network.list"] };

function live(ads?: unknown) {
  return normalizeLiveStatusResponse({
    timestamp: "2026-11-16T10:00:00Z",
    summary: { servicesUp: 1, servicesTotal: 1, assetCount: 1, staleAssetCount: 0 },
    endpoints: [],
    assets: [{ id: "mon-01", name: "mon-01", type: "host", source: "agent", status: "online", last_seen_at: "" }],
    telemetryOverview: [],
    ...(ads === undefined ? {} : { agentAdvertisements: ads }),
  });
}

describe("status agentAdvertisements", () => {
  // Fault: the live or full normalizer drops the map.
  it("keeps the map from both status answers", () => {
    expect(live({ "mon-01": ad }).agentAdvertisements).toEqual({ "mon-01": ad });
    expect(normalizeStatusResponse({ agentAdvertisements: { "mon-01": ad } }).agentAdvertisements).toEqual({ "mon-01": ad });
  });

  // Fault: a Go-only fleet (the Hub leaves the field out) gets a value that marks devices explicit.
  it("reads a missing map as empty", () => {
    expect(live().agentAdvertisements).toEqual({});
  });

  // Fault: a live poll that changes only the advertisement (an agent switched to the Rust agent) is ignored as
  // "unchanged", or the first status built from a live poll has no map.
  it("takes each live poll's map, also when nothing else changed", () => {
    const first = mergeLiveStatus(null, null, live()).status;
    expect(first.agentAdvertisements).toEqual({});
    const next = mergeLiveStatus(first, first, live({ "mon-01": ad }));
    expect(next.decision).toBe("merged");
    expect(next.status.agentAdvertisements).toEqual({ "mon-01": ad });
  });

  // Fault: the slower full status overwrites the fresher live map.
  it("keeps the live map when the full status arrives", () => {
    const current = mergeLiveStatus(null, null, live({ "mon-01": ad })).status;
    const full = normalizeStatusResponse({ ...current, agentAdvertisements: {} });
    expect(mergeFullStatus(current, full).status.agentAdvertisements).toEqual({ "mon-01": ad });
  });
});
