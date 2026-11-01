import {
  AppWindow,
  Computer,
  Container,
  DatabaseBackup,
  HardDrive,
  House,
  Laptop,
  Monitor,
  Server,
  type LucideIcon,
} from "lucide-react";
import type { Asset, Group } from "../../../console/models";
import { matchesQuery } from "./deviceTreeModel";
import type { DeviceCardData } from "./nodesPageUtils";

/** How the list is cut into sections (the Group menu). */
export type Grouping = "type" | "status" | "group" | "none";
export const GROUPINGS: Grouping[] = ["type", "status", "group", "none"];

/** Type sections, in page order. */
export type TypeSection = "servers" | "storage" | "computers" | "vms" | "other";
const TYPE_ORDER: TypeSection[] = ["servers", "storage", "computers", "vms", "other"];

const SECTION_OF_TYPE: Record<string, TypeSection> = {
  "hypervisor-node": "servers",
  "storage-controller": "servers",
  "container-host": "servers",
  "connector-cluster": "servers",
  nas: "storage",
  vm: "vms",
  host: "computers",
  node: "computers",
};

export const typeSection = (asset: Asset): TypeSection => SECTION_OF_TYPE[asset.type] ?? "other";

/** The drawing in a row's tile: what the device is, not where its data comes from. */
export function deviceIcon(asset: Asset): LucideIcon {
  switch (asset.type) {
    case "hypervisor-node": return Server;
    case "storage-controller": return DatabaseBackup;
    case "container-host": return Container;
    case "connector-cluster": return House;
    case "nas": return HardDrive;
    case "vm": return AppWindow;
  }
  const platform = (asset.platform ?? "").toLowerCase();
  if (platform.includes("darwin") || platform.includes("mac")) return Laptop;
  if (platform.includes("windows")) return Monitor;
  return Computer;
}

/** Anything that is not answering normally needs attention. */
export const needsAttention = (card: DeviceCardData) => card.freshness !== "online";

/** Problems to the top, then by name. */
const problemsFirst = (a: DeviceCardData, b: DeviceCardData) =>
  Number(needsAttention(b)) - Number(needsAttention(a)) || a.asset.name.localeCompare(b.asset.name);

/** A row and, in the Type view, the guests folded under it. */
export type ListRow = { card: DeviceCardData; guests: DeviceCardData[] };
/** label: a translation key under devices.list.sections, or a group's own name (labelIsName). */
export type ListSection = { key: string; label: string; labelIsName?: boolean; rows: ListRow[] };

/** "Lab / Rack" for a nested group. */
function groupPath(group: Group, byID: Map<string, Group>): string {
  const names = [group.name];
  let parent = group.parent_group_id ? byID.get(group.parent_group_id) : undefined;
  for (let depth = 0; parent && depth < 8; depth++) {
    names.unshift(parent.name);
    parent = parent.parent_group_id ? byID.get(parent.parent_group_id) : undefined;
  }
  return names.join(" / ");
}

export function buildSections(params: {
  cards: DeviceCardData[];
  grouping: Grouping;
  groups: Group[];
  query: string;
  problemsOnly: boolean;
}): ListSection[] {
  const { cards, grouping, groups, query, problemsOnly } = params;
  const shown = cards.filter((c) => matchesQuery(c.asset, query) && (!problemsOnly || needsAttention(c)));
  const plain = (list: DeviceCardData[]): ListRow[] => [...list].sort(problemsFirst).map((card) => ({ card, guests: [] }));

  if (grouping === "none") return [{ key: "all", label: "", rows: plain(shown) }];

  if (grouping === "status") {
    return [
      { key: "attention", label: "attention", rows: plain(shown.filter(needsAttention)) },
      { key: "online", label: "online", rows: plain(shown.filter((c) => !needsAttention(c))) },
    ].filter((s) => s.rows.length);
  }

  if (grouping === "group") {
    const byID = new Map(groups.map((g) => [g.id, g]));
    const named = groups
      .map((g) => ({ key: `group:${g.id}`, label: groupPath(g, byID), labelIsName: true, rows: plain(shown.filter((c) => c.asset.group_id === g.id)) }))
      .sort((a, b) => a.label.localeCompare(b.label));
    const loose = { key: "group:none", label: "noGroup", rows: plain(shown.filter((c) => !c.asset.group_id || !byID.has(c.asset.group_id))) };
    return [...named, loose].filter((s) => s.rows.length);
  }

  // Type: a guest whose host is shown folds under that host's row instead of standing on its own.
  const shownIDs = new Set(shown.map((c) => c.asset.id));
  const folded = (c: DeviceCardData) => !!c.hostedOn && shownIDs.has(c.hostedOn.id);
  const guestsOf = new Map<string, DeviceCardData[]>();
  for (const c of shown) {
    if (!folded(c)) continue;
    const list = guestsOf.get(c.hostedOn!.id) ?? [];
    list.push(c);
    guestsOf.set(c.hostedOn!.id, list);
  }
  const top = shown.filter((c) => !folded(c));
  return TYPE_ORDER.map((type) => ({
    key: `type:${type}`,
    label: type,
    rows: top
      .filter((c) => typeSection(c.asset) === type)
      .sort(problemsFirst)
      .map((card) => ({ card, guests: (guestsOf.get(card.asset.id) ?? []).sort(problemsFirst) })),
  })).filter((s) => s.rows.length);
}
