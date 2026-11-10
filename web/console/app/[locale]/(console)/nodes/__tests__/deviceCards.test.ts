import { describe, expect, it } from "vitest";
import type { Asset } from "../../../../console/models";
import { isDeviceTier } from "../../../../console/taxonomy";
import { buildDeviceCards } from "../deviceCards";
import { typeSection } from "../deviceListModel";

// Risk: a device added by hand (Add device › Manual, Hub type "device") is saved but never listed, so it is missing
// from Devices, the Dashboard count, the Map and Remote View. Seen on a real Hub on 2026-11-10.
describe("a device added by hand", () => {
  const manual = {
    id: "manual_1000000000000000001_1", name: "nas01", type: "device", source: "manual", status: "online",
    last_seen_at: "2026-11-10T11:45:00Z",
  } as Asset;

  it("is a device the console lists", () => {
    expect(isDeviceTier(manual)).toBe(true);
    const ids = [...buildDeviceCards([manual], []).cardsByGroupID.values()].flat().map((card) => card.asset.id);
    expect(ids).toEqual([manual.id]);
    expect(typeSection(manual)).toBe("other");
  });
});
