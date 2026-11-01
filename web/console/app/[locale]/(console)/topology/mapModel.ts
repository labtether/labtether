// The Map's picture of the lab, built from what the hub reports: the devices (status), their guests (hierarchy), the
// zones and the connections (topology). Network links (connected to, peer of) between devices that are not guests
// build the tree: one root per linked group (a router or switch first), then breadth first. Pure: data in, graph out.
import type { Asset } from "../../../console/models";
import { assetFreshness, type DeviceCardData, type Freshness } from "../nodes/nodesPageUtils";
import type { RelationshipType, TopologyConnection, TopologyState } from "./topologyCanvasTypes";

export type Tone = "ok" | "warn" | "bad";
/** What a device does on the network, from its tags or its OS name. "" for everything else. */
export type NetRole = "router" | "switch" | "ap" | "";

export type MapDevice = {
  id: string;
  name: string;
  asset: Asset;
  freshness: Freshness;
  tone: Tone;
  /** The host a guest runs on, or "" for a device that sits in the tree itself. */
  host: string;
  /** The zone id, or "" (Other). Guests follow their host. */
  zone: string;
  net: NetRole;
  address: string;
};

export type MapLink = { id: string; a: string; b: string; rel: RelationshipType; label: string; origin: TopologyConnection["origin"] };
export type MapZone = { id: string; label: string };
export type Forest = { roots: string[]; lone: string[]; parent: Record<string, string>; kids: Record<string, string[]> };

export type MapGraph = {
  devices: MapDevice[];
  byId: Map<string, MapDevice>;
  /** Guests per host, problems first, then by VM id and name. */
  guests: Map<string, MapDevice[]>;
  zones: MapZone[];
  links: MapLink[];
  forest: Forest;
};

const NET_RANK: Record<NetRole, number> = { router: 3, switch: 2, ap: 1, "": 0 };
const TONE_RANK: Record<Tone, number> = { bad: 2, warn: 1, ok: 0 };
const NETWORK_RELS = new Set<string>(["connected_to", "peer_of"]);
const HOST_TYPES = new Set(["hypervisor-node", "storage-controller", "nas", "container-host", "node", "host"]);
const ROUTER_OS = /\b(opnsense|pfsense|openwrt|routeros|vyos|edgeos|ipfire|sophos|fortios|unifi gateway|udm)\b/i;
const SWITCH_OS = /\b(switchos|swos|cisco ios|aruba|unifi switch|edgeswitch)\b/i;

export const toneOf = (f: Freshness): Tone => (f === "online" ? "ok" : f === "offline" ? "bad" : "warn");
export const isNetworkLink = (rel: string) => NETWORK_RELS.has(rel);

/** Router, switch or access point from the device's own tags first, then the OS name the agent reports. */
export function netRoleOf(asset: Asset): NetRole {
  const tags = (asset.tags ?? []).map((t) => t.toLowerCase());
  if (tags.some((t) => ["router", "firewall", "gateway"].includes(t))) return "router";
  if (tags.includes("switch")) return "switch";
  if (tags.some((t) => ["ap", "access-point", "wifi"].includes(t))) return "ap";
  const os = `${asset.metadata?.os_pretty_name ?? ""} ${asset.metadata?.os_name ?? ""}`;
  if (ROUTER_OS.test(os)) return "router";
  if (SWITCH_OS.test(os)) return "switch";
  return "";
}

const vmOrder = (d: MapDevice) => Number(/(\d+)/.exec(d.asset.metadata?.vmid ?? "")?.[1] ?? 0);
const byProblemThenName = (a: MapDevice, b: MapDevice) =>
  TONE_RANK[b.tone] - TONE_RANK[a.tone] || vmOrder(a) - vmOrder(b) || a.name.localeCompare(b.name);
/** Tree order: network gear, then hosts, then the rest, each by name. */
const treeOrder = (a: MapDevice, b: MapDevice) =>
  NET_RANK[b.net] - NET_RANK[a.net]
  || Number(HOST_TYPES.has(b.asset.type)) - Number(HOST_TYPES.has(a.asset.type))
  || a.name.localeCompare(b.name);

export type GraphInput = {
  /** Every asset the hub reports (status). */
  assets: Asset[];
  /** The Devices list's cards: the devices the Map draws, with their freshness and taxonomy host. */
  cards: DeviceCardData[];
  topology: TopologyState | null;
  /** Hierarchy parents (explicit runs on / hosted on / contains, then taxonomy). */
  parentOf?: Map<string, string>;
  /** Hosts: assets that keep their own place in the tree even when something contains them. */
  isHost?: (asset: Asset) => boolean;
};

/** Zones whose only device carries the zone's own name (one zone per host, as the hub seeds them) say nothing. */
function namedZones(topology: TopologyState | null, devices: Map<string, { name: string; host: string }>) {
  const members = new Map<string, string[]>();
  for (const m of topology?.members ?? []) {
    const d = devices.get(m.asset_id);
    if (!d || d.host) continue;
    members.set(m.zone_id, [...(members.get(m.zone_id) ?? []), d.name]);
  }
  return [...(topology?.zones ?? [])]
    .sort((a, b) => a.sort_order - b.sort_order || a.label.localeCompare(b.label))
    .filter((z) => { const names = members.get(z.id) ?? []; return !(names.length === 1 && names[0] === z.label); })
    .map((z) => ({ id: z.id, label: z.label }));
}

export function buildMapGraph({ assets, cards, topology, parentOf, isHost }: GraphInput): MapGraph {
  const assetById = new Map(assets.map((a) => [a.id, a]));
  // Assets the person took off the map (dismissed) are neither zone members nor unsorted.
  const placed = topology ? new Set([...topology.members.map((m) => m.asset_id), ...topology.unsorted]) : null;
  const onMap = (id: string) => !placed || placed.has(id);
  const cardIds = new Set(cards.map((c) => c.asset.id));
  const extra = (topology?.connections ?? [])
    .filter((c) => isNetworkLink(c.relationship))
    .flatMap((c) => [c.source_asset_id, c.target_asset_id])
    .filter((id) => !cardIds.has(id) && assetById.has(id));

  const base = new Map<string, { card?: DeviceCardData; asset: Asset }>();
  for (const c of cards) if (onMap(c.asset.id)) base.set(c.asset.id, { card: c, asset: c.asset });
  for (const id of extra) if (onMap(id) && !base.has(id)) base.set(id, { asset: assetById.get(id)! });

  // A guest runs on a device that is on the map itself; hosts keep their own place.
  const hostOf = (id: string, asset: Asset, card?: DeviceCardData) => {
    if (isHost?.(asset)) return "";
    const h = card?.hostedOn?.id ?? parentOf?.get(id) ?? "";
    return h && h !== id && base.has(h) ? h : "";
  };
  const draft = new Map([...base].map(([id, b]) => [id, { ...b, name: b.asset.name || id, host: hostOf(id, b.asset, b.card) }]));
  // A host that is itself a guest would hide its own guests: keep it in the tree.
  for (const d of draft.values()) if (d.host && draft.get(d.host)?.host) d.host = "";

  const zones = namedZones(topology, draft);
  const zoneIds = new Set(zones.map((z) => z.id));
  const zoneOfAsset = new Map((topology?.members ?? []).filter((m) => zoneIds.has(m.zone_id)).map((m) => [m.asset_id, m.zone_id]));

  const devices: MapDevice[] = [...draft].map(([id, d]) => {
    const freshness = d.card?.freshness ?? assetFreshness(d.asset);
    const meta = d.asset.metadata ?? {};
    return {
      id, name: d.name, asset: d.asset, freshness, tone: toneOf(freshness), host: d.host,
      zone: zoneOfAsset.get(d.host || id) ?? "", net: netRoleOf(d.asset), address: meta.ip_address || meta.ip || "",
    };
  });
  const byId = new Map(devices.map((d) => [d.id, d]));
  const guests = new Map<string, MapDevice[]>();
  for (const d of devices) if (d.host) guests.set(d.host, [...(guests.get(d.host) ?? []), d]);
  for (const list of guests.values()) list.sort(byProblemThenName);

  const links: MapLink[] = (topology?.connections ?? [])
    .filter((c) => byId.has(c.source_asset_id) && byId.has(c.target_asset_id) && c.source_asset_id !== c.target_asset_id)
    .map((c) => ({ id: c.id, a: c.source_asset_id, b: c.target_asset_id, rel: c.relationship, label: c.label ?? "", origin: c.origin }));

  return { devices, byId, guests, zones, links, forest: buildForest(devices, links) };
}

/** Roots (one per linked group: the highest network role, then the most links), parents and children by network link. */
export function buildForest(devices: MapDevice[], links: MapLink[]): Forest {
  const list = devices.filter((d) => !d.host).sort(treeOrder);
  const rank = new Map(list.map((d, i) => [d.id, i]));
  const adj = new Map<string, string[]>(list.map((d) => [d.id, []]));
  for (const l of links) {
    if (!isNetworkLink(l.rel) || !adj.has(l.a) || !adj.has(l.b)) continue;
    if (!adj.get(l.a)!.includes(l.b)) adj.get(l.a)!.push(l.b);
    if (!adj.get(l.b)!.includes(l.a)) adj.get(l.b)!.push(l.a);
  }
  for (const n of adj.values()) n.sort((x, y) => rank.get(x)! - rank.get(y)!);
  const byId = new Map(list.map((d) => [d.id, d]));
  const seen = new Set<string>();
  const parent: Record<string, string> = {};
  const kids: Record<string, string[]> = {};
  const roots: string[] = [];
  const lone: string[] = [];
  for (const d of list) {
    if (seen.has(d.id)) continue;
    if (!adj.get(d.id)!.length) { lone.push(d.id); seen.add(d.id); continue; }
    const group = new Set([d.id]);
    for (const stack = [d.id]; stack.length;) for (const y of adj.get(stack.pop()!)!) if (!group.has(y)) { group.add(y); stack.push(y); }
    const root = [...group].sort((a, b) =>
      NET_RANK[byId.get(b)!.net] - NET_RANK[byId.get(a)!.net] || adj.get(b)!.length - adj.get(a)!.length || rank.get(a)! - rank.get(b)!)[0];
    roots.push(root); seen.add(root); kids[root] = [];
    for (const queue = [root]; queue.length;) {
      const x = queue.shift()!;
      for (const y of adj.get(x)!) if (!seen.has(y)) { seen.add(y); parent[y] = x; kids[x].push(y); kids[y] = []; queue.push(y); }
    }
  }
  return { roots, lone, parent, kids };
}

/** Children grouped by zone, in zone order, Other ("") last: [[zoneId, ids]]. */
export function byZone(graph: MapGraph, ids: string[]): Array<[string, string[]]> {
  const out: Array<[string, string[]]> = [];
  for (const z of [...graph.zones.map((q) => q.id), ""]) {
    const g = ids.filter((id) => (graph.byId.get(id)?.zone ?? "") === z);
    if (g.length) out.push([z, g]);
  }
  return out;
}

/** A device matches the find text by name, address, type, source, OS or tags. q is lower case. */
export function hitOf(d: MapDevice, q: string): boolean {
  if (!q) return false;
  const a = d.asset;
  return [d.name, d.address, a.type, a.source, a.platform ?? "", ...(a.tags ?? []), ...Object.values(a.metadata ?? {})]
    .join(" ").toLowerCase().includes(q);
}
