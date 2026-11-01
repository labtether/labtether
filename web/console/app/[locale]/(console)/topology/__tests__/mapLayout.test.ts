import { describe, expect, it } from "vitest";
import type { Asset } from "../../../../console/models";
import { COL, layoutMap } from "../mapLayout";
import { buildForest, type MapDevice, type MapGraph, type MapLink } from "../mapModel";

const dev = (id: string, extra: Partial<MapDevice> = {}): MapDevice => ({
  id, name: id, asset: { id, name: id, type: "host", source: "agent", status: "online", last_seen_at: "" } as Asset,
  freshness: "online", tone: "ok", host: "", zone: "", net: "", address: "", ...extra,
});
function graph(devices: MapDevice[], links: MapLink[] = [], zones: MapGraph["zones"] = []): MapGraph {
  const guests = new Map<string, MapDevice[]>();
  for (const d of devices) if (d.host) guests.set(d.host, [...(guests.get(d.host) ?? []), d]);
  return { devices, byId: new Map(devices.map((d) => [d.id, d])), guests, zones, links, forest: buildForest(devices, links) };
}
const net = (a: string, b: string): MapLink => ({ id: `${a}-${b}`, a, b, rel: "connected_to", label: "", origin: "user" });

// Risk: the tree is laid out by hand-written geometry. Tiles must never overlap, a long guest list must fold so the
// map stays readable, a find must open the fold that hides its match, and an unlinked lab must wrap instead of
// running off the screen.
describe("Map layout", () => {
  it("keeps sibling tiles at least one column apart and under their parent", () => {
    const kids = ["a", "b", "c", "d"];
    const g = graph([dev("sw", { net: "switch" }), ...kids.map((k) => dev(k))], kids.map((k) => net("sw", k)));
    const L = layoutMap(g);
    const xs = kids.map((k) => L.nodeOf.get(k)!.cx).sort((p, q) => p - q);
    for (let i = 1; i < xs.length; i++) expect(xs[i] - xs[i - 1]).toBeGreaterThanOrEqual(COL);
    const sw = L.nodeOf.get("sw")!;
    expect(sw.cx).toBeCloseTo((xs[0] + xs[xs.length - 1]) / 2);
    expect(L.nodeOf.get("a")!.y).toBeGreaterThan(sw.y);
  });

  it("folds a host's guests after six and opens the fold for a find that matches a hidden one", () => {
    const guests = Array.from({ length: 9 }, (_, i) => dev(`vm-${i}`, { host: "pve" }));
    const g = graph([dev("pve"), ...guests]);
    const folded = layoutMap(g);
    expect(folded.rows.filter((r) => r.id)).toHaveLength(6);
    expect(folded.rows.find((r) => !r.id)?.fold).toBe(3);
    const found = layoutMap(g, { q: "vm-8" });
    expect(found.rows.filter((r) => r.id)).toHaveLength(9);
    expect(layoutMap(g, { open: new Set(["pve"]) }).rows.find((r) => !r.id)?.fold).toBe("less");
  });

  it("wraps an unlinked lab into rows no wider than asked, with zone labels when there is more than one zone", () => {
    const zones = [{ id: "rack", label: "Rack" }, { id: "desk", label: "Desk" }];
    const devices = Array.from({ length: 12 }, (_, i) => dev(`h${String(i).padStart(2, "0")}`, { zone: i < 6 ? "rack" : "desk" }));
    const L = layoutMap(graph(devices, [], zones), { wrap: 400 });
    const rows = new Set(L.nodes.map((n) => n.y));
    expect(rows.size).toBeGreaterThan(1);
    for (const n of L.nodes) expect(n.cx + COL / 2).toBeLessThanOrEqual(400 + COL);
    expect(L.labels.map((l) => l.key)).toEqual(expect.arrayContaining(["rack", "desk"]));
  });

  it("stands unlinked devices beside the tree under one title when a tree exists", () => {
    const g = graph([dev("gw", { net: "router" }), dev("sw"), dev("lone-1")], [net("gw", "sw")]);
    const L = layoutMap(g);
    expect(L.labels.map((l) => l.key)).toEqual(["lone"]);
    expect(L.nodeOf.get("lone-1")!.cx).toBeGreaterThan(L.nodeOf.get("gw")!.cx);
  });
});
