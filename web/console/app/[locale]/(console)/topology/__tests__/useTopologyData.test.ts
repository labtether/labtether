import { describe, expect, it } from "vitest";

import { normalizeTopologyState } from "../useTopologyData";

describe("normalizeTopologyState", () => {
  it("normalizes nullable collection fields from an empty installed hub", () => {
    expect(normalizeTopologyState({
      id: "topology-1",
      name: "My Homelab",
      zones: null,
      members: null,
      connections: null,
      unsorted: null,
      viewport: { x: 0, y: 0, zoom: 1 },
    })).toMatchObject({
      zones: [],
      members: [],
      connections: [],
      unsorted: [],
    });
  });

  it("rejects a non-object response", () => {
    expect(normalizeTopologyState(null)).toBeNull();
    expect(normalizeTopologyState([])).toBeNull();
  });
});
