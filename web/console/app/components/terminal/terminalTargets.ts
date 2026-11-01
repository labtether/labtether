// What the Terminal can open: the devices and containers the hub knows, how each one is reached, and the lists the
// picker shows (Recent, All devices, Containers, Offline). Also the address form's parser and the tab strip's labels.
import type { Asset } from "../../console/models";
import { assetFreshnessLabel } from "../../console/formatters";
import { childParentKey, hostParentKey, isDeviceTier, isHiddenAsset, isInfraHost } from "../../console/taxonomy";

const CONTAINER_TYPES = new Set(["container", "docker-container", "pod", "deployment", "app", "stack", "compose-stack"]);

export const isContainerTarget = (asset: Pick<Asset, "type">) => CONTAINER_TYPES.has(asset.type);

/** How a shell reaches the target: the agent (the default, shown as nothing), the Proxmox API, SSH, or a container exec. */
export type TargetVia = "agent" | "proxmox" | "ssh" | "container";
/** The hub's last word on the target: checked in recently, late, not seen for five minutes, or never. */
export type TargetState = "online" | "unresponsive" | "offline" | "unknown";

export type TerminalTarget = {
  asset: Asset;
  via: TargetVia;
  state: TargetState;
  /** What it runs ("Debian 13"), or "" when the hub has no OS name. */
  os: string;
  /** The host a VM or container runs on, by name; "" for a host of its own. */
  host: string;
  /** A Proxmox guest's VM ID, or "". */
  vmid: string;
  container: boolean;
};

export type TargetLists = {
  recent: TerminalTarget[];
  devices: TerminalTarget[];
  containers: TerminalTarget[];
  offline: TerminalTarget[];
};

function stateOf(asset: Asset): TargetState {
  const fresh = assetFreshnessLabel(asset.last_seen_at);
  if (fresh === "online" || fresh === "unresponsive" || fresh === "offline") return fresh;
  return "unknown";
}

/** Host names by the key their guests and containers point at (Proxmox node name, Docker agent, Portainer endpoint). */
export function hostNamesByKey(assets: Asset[]): Map<string, string> {
  const names = new Map<string, string>();
  for (const asset of assets) {
    if (isInfraHost(asset)) names.set(hostParentKey(asset), asset.name || asset.id);
  }
  return names;
}

export function describeTarget(asset: Asset, connectedAgentIds: ReadonlySet<string>, hostNames: Map<string, string>): TerminalTarget {
  const meta = asset.metadata ?? {};
  const container = isContainerTarget(asset);
  const key = isInfraHost(asset) ? undefined : childParentKey(asset);
  const host = (key ? hostNames.get(key) : undefined) ?? (container ? meta.node || meta.host || meta.endpoint_name || "" : meta.node || "");
  const via: TargetVia = connectedAgentIds.has(asset.id)
    ? "agent"
    : container
      ? "container"
      : asset.source === "proxmox"
        ? "proxmox"
        : "ssh";
  return {
    asset,
    via,
    state: stateOf(asset),
    os: meta.os_pretty_name || meta.os_name || "",
    host: host === (asset.name || asset.id) ? "" : host,
    vmid: meta.vmid || "",
    container,
  };
}

/** True when every word of the query appears in the target's name, address, OS, host, type or tags. */
export function matchesTarget(target: TerminalTarget, query: string): boolean {
  const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (words.length === 0) return true;
  const meta = target.asset.metadata ?? {};
  const hay = [
    target.asset.name, target.asset.id, target.asset.type, target.asset.platform ?? "", target.os, target.host,
    meta.ip_address ?? "", meta.ip ?? "", ...(target.asset.tags ?? []),
  ].join(" ").toLowerCase();
  return words.every((word) => hay.includes(word));
}

const byName = (a: TerminalTarget, b: TerminalTarget) =>
  (a.asset.name || a.asset.id).localeCompare(b.asset.name || b.asset.id, undefined, { numeric: true, sensitivity: "base" });

/**
 * The picker's lists. Devices and containers the hub has not seen for five minutes go to Offline (they can still be
 * tried). Recent (at most three, newest first) shows only while the query is empty and never repeats an offline target.
 * firstId: the device listed first in All devices ("this device").
 */
export function buildTargetLists({
  assets,
  connectedAgentIds,
  recentIds = [],
  query = "",
  firstId = "",
}: {
  assets: Asset[];
  connectedAgentIds: ReadonlySet<string>;
  recentIds?: string[];
  query?: string;
  firstId?: string;
}): TargetLists {
  const hostNames = hostNamesByKey(assets);
  const all = assets
    .filter((asset) => !isHiddenAsset(asset) && (isDeviceTier(asset) || isContainerTarget(asset)))
    .map((asset) => describeTarget(asset, connectedAgentIds, hostNames));
  const shown = all.filter((target) => matchesTarget(target, query)).sort(byName);
  const first = shown.filter((target) => target.asset.id === firstId);
  const rest = shown.filter((target) => target.asset.id !== firstId);
  const live = (target: TerminalTarget) => target.state !== "offline";
  const byId = new Map(all.map((target) => [target.asset.id, target]));
  const recent = query.trim()
    ? []
    : recentIds.map((id) => byId.get(id)).filter((t): t is TerminalTarget => Boolean(t && live(t))).slice(0, 3);
  return {
    recent,
    devices: [...first, ...rest].filter((t) => live(t) && !t.container),
    containers: rest.filter((t) => live(t) && t.container),
    offline: [...first, ...rest].filter((t) => !live(t)),
  };
}

export type ParsedAddress = { user: string; host: string; port: number };

/** "user@host:port" → its parts (port 22 when left out), or null. IPv6 needs brackets: root@[2001:db8::1]:22. */
export function parseAddress(text: string): ParsedAddress | null {
  const match = /^(?:([A-Za-z0-9._-]+)@)?(\[[0-9A-Fa-f:]+\]|[A-Za-z0-9.-]+)(?::(\d{1,5}))?$/.exec(String(text ?? "").trim());
  if (!match) return null;
  return { user: match[1] ?? "", host: match[2].replace(/^\[|\]$/g, ""), port: match[3] ? Number(match[3]) : 22 };
}

/** True when the picker's query is an address to SSH to rather than a device name: user@host, host:port or an IP. */
export function looksLikeAddress(query: string): boolean {
  const q = String(query ?? "").trim();
  return parseAddress(q) !== null && /@|:\d|^\d+\.\d+\.\d+\.\d+$|^\[/.test(q);
}

/** The address as the form shows it: user@host, with the port only when it is not 22. */
export function formatAddress({ user, host, port }: ParsedAddress): string {
  const shownHost = host.includes(":") ? `[${host}]` : host;
  return `${user ? `${user}@` : ""}${shownHost}${port && port !== 22 ? `:${port}` : ""}`;
}

/** Names the workspace gives tabs on its own ("Default", "Tab 3", their copies); a renamed tab keeps its name. */
export function isAutoTabName(name: string): boolean {
  return /^(Default|Untitled|Tab \d+)( Copy)*$/.test(name.trim());
}

/**
 * Labels for the tab strip: a renamed tab keeps its name; any other tab reads as its first device's name, numbered
 * from that device's second tab on ("pve-01", "pve-01 (2)"). null: a tab with no device yet ("New tab").
 */
export function tabLabels(
  tabs: Array<{ name: string; panes: Array<{ targetNodeId: string }> }>,
  nameOf: (id: string) => string,
): Array<string | null> {
  const seen = new Map<string, number>();
  return tabs.map((tab) => {
    if (!isAutoTabName(tab.name)) return tab.name;
    const id = tab.panes.find((pane) => pane.targetNodeId)?.targetNodeId;
    if (!id) return null;
    const name = nameOf(id) || id;
    const n = (seen.get(name) ?? 0) + 1;
    seen.set(name, n);
    return n > 1 ? `${name} (${n})` : name;
  });
}
