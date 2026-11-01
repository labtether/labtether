import type { Asset, Group } from "../../../console/models";
import type { GroupTreeItem } from "./useGroupTree";

/** The rows the list shows: every root, and the children of each open group, in tree order. */
export function visibleRows(tree: GroupTreeItem[]): GroupTreeItem[] {
  const out: GroupTreeItem[] = [];
  const walk = (items: GroupTreeItem[]) => {
    for (const item of items) {
      out.push(item);
      if (item.expanded) walk(item.children);
    }
  };
  walk(tree);
  return out;
}

/** Device names per group (the group a device is directly in), sorted by name. */
export function deviceNamesByGroup(assets: Pick<Asset, "name" | "group_id">[]): Map<string, string[]> {
  const out = new Map<string, string[]>();
  for (const asset of assets) {
    const id = asset.group_id?.trim();
    if (!id) continue;
    const list = out.get(id) ?? [];
    list.push(asset.name);
    out.set(id, list);
  }
  for (const list of out.values()) list.sort((a, b) => a.localeCompare(b));
  return out;
}

/** Devices in no group, or in a group the hub no longer lists. */
export function ungroupedCount(assets: Pick<Asset, "group_id">[], groups: Pick<Group, "id">[]): number {
  const known = new Set(groups.map((g) => g.id));
  return assets.filter((a) => !a.group_id?.trim() || !known.has(a.group_id.trim())).length;
}

/** The first few names and how many more there are, for a row's grey line. */
export function firstNames(names: string[], max = 4): { shown: string[]; more: number } {
  return { shown: names.slice(0, max), more: Math.max(0, names.length - max) };
}

/** "bastion → 10.20.0.9": the hosts a group's devices are reached through, or "" when they are reached directly. */
export const hopsLine = (group: Pick<Group, "jump_chain">) =>
  (group.jump_chain?.hops ?? []).map((hop) => hop.host).filter(Boolean).join(" → ");

/** A slug from a name, as the hub wants it: lower case letters, digits and dashes. */
export const slugFromName = (name: string) =>
  name
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64);

/** The group's path from the top, "Home / Rack A", for selects and sentences. */
export function groupPath(groups: Group[], id: string): string {
  const byId = new Map(groups.map((g) => [g.id, g]));
  const parts: string[] = [];
  let current = byId.get(id);
  for (let guard = 0; current && guard < 32; guard++) {
    parts.unshift(current.name);
    current = current.parent_group_id ? byId.get(current.parent_group_id) : undefined;
  }
  return parts.join(" / ");
}
