import { describe, expect, it } from "vitest";
import type { Asset } from "../../../../console/models";
import type { DeviceCardData, Freshness } from "../../nodes/nodesPageUtils";
import { buildMapGraph } from "../mapModel";
import type { TopologyConnection, TopologyState } from "../topologyCanvasTypes";

const asset = (id: string, type: string, extra: Partial<Asset> = {}): Asset =>
  ({ id, name: id, type, source: "agent", status: "online", last_seen_at: "2026-01-01T00:00:00Z", ...extra });
const card = (a: Asset, extra: Partial<DeviceCardData> = {}, freshness: Freshness = "online"): DeviceCardData => ({
  asset: a, freshness, cpu: null, mem: null, disk: null,
  workloads: { vms: 0, containers: 0, stacks: 0, datastores: 0, other: 0 },
  hostedOn: null, dockerHost: null, haHub: null, ...extra,
});
const link = (id: string, a: string, b: string, relationship: TopologyConnection["relationship"] = "connected_to"): TopologyConnection =>
  ({ id, source_asset_id: a, target_asset_id: b, relationship, user_defined: true, label: "", origin: "user" });
const topo = (over: Partial<TopologyState>): TopologyState =>
  ({ id: "t", name: "lab", zones: [], members: [], connections: [], unsorted: [], viewport: { x: 0, y: 0, zoom: 1 }, ...over });

const router = asset("gw", "host", { metadata: { os_pretty_name: "OPNsense 25.7" } });
const sw = asset("sw-core", "device", { source: "manual", tags: ["switch"] });
const pve = asset("pve-01", "hypervisor-node", { source: "proxmox" });
const vm = asset("docker-01", "vm", { source: "proxmox", metadata: { node: "pve-01", vmid: "110" } });
const nas = asset("nas01", "nas", { source: "truenas" });
const desk = asset("desk-01", "host");
const cards = [card(router), card(pve), card(vm, { hostedOn: { id: "pve-01", name: "pve-01" } }), card(nas), card(desk)];
const assets = [router, sw, pve, vm, nas, desk];
const all = [...assets.map((a) => a.id)];

// Risk: the Map draws the network from links people add in any direction, so the tree must still start at the
// router, keep guests on their host's spine, never invent links, and keep devices the person left off the map off it.
describe("Map graph", () => {
  it("roots the tree at the router whichever way the links were drawn", () => {
    const g = buildMapGraph({
      assets, cards,
      topology: topo({ unsorted: all, connections: [link("1", "pve-01", "sw-core"), link("2", "sw-core", "gw"), link("3", "nas01", "sw-core")] }),
    });
    expect(g.forest.roots).toEqual(["gw"]);
    expect(g.forest.kids.gw).toEqual(["sw-core"]);
    expect(g.forest.kids["sw-core"].sort()).toEqual(["nas01", "pve-01"]);
    expect(g.forest.lone).toEqual(["desk-01"]);
    // The switch is a manual device the Devices list does not show; its network link puts it on the map.
    expect(g.byId.get("sw-core")?.net).toBe("switch");
  });

  it("puts guests on their host's spine and leaves them out of the tree", () => {
    const g = buildMapGraph({ assets, cards, topology: topo({ unsorted: all }) });
    expect(g.guests.get("pve-01")?.map((d) => d.id)).toEqual(["docker-01"]);
    expect(g.forest.lone).not.toContain("docker-01");
    expect(g.forest.roots).toEqual([]);
  });

  it("takes a guest's host from the hierarchy, but never makes a host a guest", () => {
    const agentVm = asset("guest-vm", "vm");
    const g = buildMapGraph({
      assets: [...assets, agentVm], cards: [...cards, card(agentVm)], topology: topo({ unsorted: [...all, "guest-vm"] }),
      parentOf: new Map([["guest-vm", "desk-01"], ["nas01", "pve-01"]]),
      isHost: (a) => a.type !== "vm",
    });
    expect(g.byId.get("guest-vm")?.host).toBe("desk-01");
    expect(g.byId.get("nas01")?.host).toBe("");
  });

  it("leaves dismissed devices off the map", () => {
    const g = buildMapGraph({ assets, cards, topology: topo({ unsorted: all.filter((id) => id !== "desk-01") }) });
    expect(g.byId.has("desk-01")).toBe(false);
  });

  it("drops a zone that only names its one device, and keeps real zones in order", () => {
    const zone = (id: string, label: string, sort_order: number) =>
      ({ id, topology_id: "t", parent_zone_id: null, label, color: "blue", icon: "", position: { x: 0, y: 0 }, size: { width: 0, height: 0 }, collapsed: false, sort_order });
    const member = (zone_id: string, asset_id: string) => ({ zone_id, asset_id, position: { x: 0, y: 0 }, sort_order: 0 });
    const g = buildMapGraph({
      assets, cards,
      topology: topo({
        zones: [zone("z-desk", "Desk", 1), zone("z-rack", "Rack", 0), zone("z-pve", "pve-01", 2)],
        members: [member("z-rack", "nas01"), member("z-rack", "gw"), member("z-desk", "desk-01"), member("z-pve", "pve-01"), member("z-pve", "docker-01")],
      }),
    });
    expect(g.zones.map((z) => z.label)).toEqual(["Rack", "Desk"]);
    expect(g.byId.get("pve-01")?.zone).toBe("");
    expect(g.byId.get("nas01")?.zone).toBe("z-rack");
  });
});
