// The Remote View new tab page's lists: which devices can be opened, how a row reads, and the "Connect to…" search.
import type { Asset } from "../../../console/models";
import { isDeviceTier } from "../../../console/taxonomy";
import type { RemoteBookmark } from "./remoteBookmarksClient";

export type RemoteDevice = {
  id: string;
  name: string;
  address: string;
  os: string;
  /** online opens; offline and stale are shown with their problem and cannot open. */
  state: "online" | "offline" | "stale";
  lastSeen: string;
};

/** Devices (not services or containers) in the order the hub lists them. */
export function remoteDevices(assets: Asset[] | undefined): RemoteDevice[] {
  return (assets ?? []).filter(isDeviceTier).map((asset) => {
    const meta = asset.metadata ?? {};
    return {
      id: asset.id,
      name: asset.name,
      address: meta.primary_ip || meta.ip_address || meta.ip || "",
      os: meta.os_pretty_name || meta.os_name || "",
      state: asset.status === "online" ? "online" : asset.status === "offline" ? "offline" : "stale",
      lastSeen: asset.last_seen_at,
    };
  });
}

const has = (query: string, ...fields: Array<string | undefined>) => fields.some((f) => !!f && f.toLowerCase().includes(query));

export function filterDevices(list: RemoteDevice[], query: string): RemoteDevice[] {
  const q = query.trim().toLowerCase();
  return q ? list.filter((d) => has(q, d.name, d.address)) : list;
}

export function filterBookmarks(list: RemoteBookmark[], query: string): RemoteBookmark[] {
  const q = query.trim().toLowerCase();
  return q ? list.filter((b) => has(q, b.label, b.host)) : list;
}

/**
 * A search that matched nothing but reads like an address (a host name with a dot, an IP, or vnc://…) is offered as
 * Quick connect to that address.
 */
export function quickConnectCandidate(query: string): string | null {
  const q = query.trim();
  if (!q) return null;
  if (/^(vnc|rdp|spice|ard):\/\/[^\s/]+$/i.test(q)) return q;
  return /^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+(:\d{1,5})?$/.test(q) || /^\[[0-9a-f:]+\]$/i.test(q) ? q : null;
}

/** A VNC or ARD bookmark saved before plain VNC needed consent cannot open until it is edited to allow it. */
export const bookmarkBlocked = (b: Pick<RemoteBookmark, "protocol" | "allow_insecure_vnc">) =>
  (b.protocol === "vnc" || b.protocol === "ard") && !b.allow_insecure_vnc;
