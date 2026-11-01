// Which views the Proxmox tab offers, in switch order. A node leads with its tasks, storage and network; a guest
// leads with its snapshots and console. Views that need data the hub did not send (HA, Ceph) stay hidden.

import type { ProxmoxDetails } from "../nodeDetailTypes";

export const PROXMOX_VIEWS = [
  "overview", "snapshots", "tasks", "backup", "storage", "network", "firewall", "ha", "ceph",
  "console", "replication", "cluster", "metrics", "logs", "updates", "certificates",
] as const;

export type ProxmoxView = (typeof PROXMOX_VIEWS)[number];

const NODE_ORDER: ProxmoxView[] = [
  "overview", "tasks", "storage", "network", "backup", "firewall",
  "cluster", "ha", "ceph", "replication", "metrics", "logs", "updates", "certificates", "snapshots",
];
const GUEST_ORDER: ProxmoxView[] = [
  "overview", "snapshots", "tasks", "backup", "firewall", "console",
  "ha", "storage", "network", "replication", "metrics", "logs", "updates", "certificates", "ceph",
];

export function hasProxmoxCeph(details: ProxmoxDetails | null): boolean {
  return !!(details?.ceph_status?.health?.status || (details?.ceph_osds && details.ceph_osds.length > 0));
}

export function hasProxmoxHA(details: ProxmoxDetails | null): boolean {
  return !!(details?.ha?.match || (details?.ha?.resources && details.ha.resources.length > 0));
}

/** The views for this resource kind ("node", "qemu", "lxc" or another Proxmox kind) and the data at hand. */
export function proxmoxViewsFor(kind: string, details: ProxmoxDetails | null): ProxmoxView[] {
  const guest = kind === "qemu" || kind === "lxc";
  const node = kind === "node";
  const order = guest ? GUEST_ORDER : NODE_ORDER;
  return order.filter((view) => {
    if (view === "ceph") return hasProxmoxCeph(details);
    if (view === "ha") return hasProxmoxHA(details);
    if (view === "console") return guest;
    if (view === "cluster") return node;
    if (view === "snapshots") return guest || (details?.snapshots?.length ?? 0) > 0;
    return true;
  });
}
