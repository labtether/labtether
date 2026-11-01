// Decisions behind the TrueNAS tab: pool layout and health, list order, state words and the disk list that joins the
// disk inventory with SMART health. Pure functions so they can be tested without rendering.

import { sanitizeErrorMessage } from "../../../../../lib/sanitizeErrorMessage";
import type { TrueNASDiskHealth } from "../truenasTabModel";
import type { TrueNASDataset, TrueNASDisk, TrueNASService, TrueNASSnapshot, TrueNASVdev } from "./truenasTypes";

export type Tone = "ok" | "warn" | "bad" | "off" | "busy" | "";
export type SortDir = "asc" | "desc";

const LEAF_TYPES = new Set(["DISK", "FILE"]);

/** Disks under a vdev: a vdev without children is a disk; an empty group holds none. */
function leafCount(vdev: TrueNASVdev): number {
  const children = vdev.children ?? [];
  if (children.length > 0) return children.reduce((sum, child) => sum + leafCount(child), 0);
  return vdev.children === undefined || LEAF_TYPES.has(vdev.type.toUpperCase()) ? 1 : 0;
}

/**
 * A pool's layout from its topology: the first vdev's type ("RAIDZ2", "MIRROR"; "STRIPE" when the data vdevs are
 * plain disks), how many top-level vdevs share it, and the number of disks.
 */
export function poolLayout(topology?: TrueNASVdev[]): { kind: string; groups: number; disks: number } | null {
  if (!topology || topology.length === 0) return null;
  const first = topology[0].type.toUpperCase();
  const kind = LEAF_TYPES.has(first) ? "STRIPE" : first;
  const groups = kind === "STRIPE" ? 1 : topology.filter((vdev) => vdev.type.toUpperCase() === first).length;
  const disks = topology.reduce((sum, vdev) => sum + leafCount(vdev), 0);
  return { kind, groups, disks };
}

/** Pool and vdev health: nothing while ONLINE, amber when degraded, red otherwise. */
export function healthTone(status?: string): Tone {
  const value = (status ?? "").trim().toUpperCase();
  if (value === "ONLINE" || value === "") return "";
  if (value === "DEGRADED") return "warn";
  return "bad";
}

/** Faulted and degraded pools first, then by name. */
export function sortPools<T extends { name: string; status: string }>(pools: T[]): T[] {
  const rank = (status: string) => (healthTone(status) === "bad" ? 0 : healthTone(status) === "warn" ? 1 : 2);
  return [...pools].sort((a, b) => rank(a.status) - rank(b.status) || a.name.localeCompare(b.name));
}

export type DatasetSort = { key: "name" | "used"; dir: SortDir };

export function sortDatasets(list: TrueNASDataset[], sort: DatasetSort): TrueNASDataset[] {
  const sign = sort.dir === "asc" ? 1 : -1;
  return [...list].sort((a, b) => {
    const diff = sort.key === "used" ? (a.used ?? -1) - (b.used ?? -1) : a.name.localeCompare(b.name);
    return diff * sign || a.name.localeCompare(b.name);
  });
}

export type SnapshotSort = { key: "name" | "used" | "created"; dir: SortDir };

function createdMs(snap: TrueNASSnapshot): number {
  const ms = snap.created ? new Date(snap.created).getTime() : NaN;
  return Number.isFinite(ms) ? ms : -1;
}

export function sortSnapshots(list: TrueNASSnapshot[], sort: SnapshotSort): TrueNASSnapshot[] {
  const sign = sort.dir === "asc" ? 1 : -1;
  return [...list].sort((a, b) => {
    const diff = sort.key === "used" ? (a.used ?? -1) - (b.used ?? -1)
      : sort.key === "created" ? createdMs(a) - createdMs(b)
        : a.snapshot_name.localeCompare(b.snapshot_name);
    return diff * sign || a.id.localeCompare(b.id);
  });
}

const PRIORITY_SERVICES = new Set(["smb", "cifs", "nfs", "ssh", "iscsi"]);

/** File-sharing and remote-access services first, then by name. */
export function sortServices(list: TrueNASService[]): TrueNASService[] {
  return [...list].sort((a, b) => {
    const pa = PRIORITY_SERVICES.has(a.name.toLowerCase()) ? 0 : 1;
    const pb = PRIORITY_SERVICES.has(b.name.toLowerCase()) ? 0 : 1;
    return pa - pb || a.name.localeCompare(b.name);
  });
}

export type StateWord = { tone: Tone; word: "running" | "stopped" | "other"; raw?: string };

/** A service: running; stopped (amber when it should start on boot); any other reported state as it came. */
export function serviceState(svc: TrueNASService): StateWord {
  if (svc.running === true) return { tone: "", word: "running" };
  if (svc.running === false) return { tone: svc.enabled ? "warn" : "off", word: "stopped" };
  return { tone: "busy", word: "other", raw: svc.state };
}

/** A VM's power state from the TrueNAS status word. */
export function vmState(status?: string): StateWord {
  const value = (status ?? "").trim().toUpperCase();
  if (value === "RUNNING") return { tone: "", word: "running" };
  if (value === "STOPPED") return { tone: "off", word: "stopped" };
  return { tone: value === "" ? "off" : "busy", word: "other", raw: status };
}

export type ReplicationWord = { tone: Tone; word: "finished" | "running" | "waiting" | "failed" | "never" | "other"; raw?: string };

export function replicationState(state?: string, lastRun?: string): ReplicationWord {
  const value = (state ?? "").trim().toUpperCase();
  if (value === "SUCCESS" || value === "FINISHED") return { tone: "", word: "finished" };
  if (value === "RUNNING") return { tone: "busy", word: "running" };
  if (value === "WAITING" || value === "PENDING") return { tone: "busy", word: "waiting" };
  if (value === "ERROR" || value === "FAILED" || value === "ABORTED") return { tone: "bad", word: "failed" };
  if (value === "" && !lastRun) return { tone: "", word: "never" };
  return { tone: "", word: "other", raw: state };
}

/** Event and alert levels: red for errors and critical, amber for warnings. */
export function levelTone(level: string): Tone {
  const value = level.toLowerCase();
  if (value.includes("error") || value.includes("crit") || value.includes("alert") || value.includes("emerg")) return "bad";
  if (value.includes("warn")) return "warn";
  return "";
}

export type DiskRow = {
  name: string;
  model?: string;
  serial?: string;
  type?: string;
  size?: number;
  temperature?: number;
  pool?: string;
  fault: { tone: "warn" | "bad"; text: string | null } | null;
  lastTest?: { type?: string; status?: string; at?: string };
};

function smartFault(disk: TrueNASDisk | undefined, health: TrueNASDiskHealth | undefined): DiskRow["fault"] {
  const status = (health?.status ?? "").toLowerCase();
  if (status === "critical") return { tone: "bad", text: health?.smart_health || health?.last_test_status || null };
  if (status === "warning") return { tone: "warn", text: health?.smart_health || health?.last_test_status || null };
  const smart = (disk?.smart_status ?? "").trim();
  if (smart !== "" && !["PASSED", "OK", "HEALTHY"].includes(smart.toUpperCase())) return { tone: "warn", text: smart };
  return null;
}

/** The disk inventory joined with SMART health by name; disks known only to SMART are kept. Faults first. */
export function diskRows(disks: TrueNASDisk[], health: TrueNASDiskHealth[]): DiskRow[] {
  const byName = new Map(health.filter((h) => h.name).map((h) => [h.name, h]));
  const names = [...new Set([...disks.map((d) => d.name), ...byName.keys()])];
  const inventory = new Map(disks.map((d) => [d.name, d]));
  const rows = names.map((name): DiskRow => {
    const disk = inventory.get(name);
    const smart = byName.get(name);
    return {
      name,
      model: disk?.model ?? smart?.model,
      serial: disk?.serial ?? smart?.serial,
      type: disk?.type ?? smart?.type,
      size: disk?.size ?? smart?.size_bytes,
      temperature: disk?.temperature_celsius ?? smart?.temperature_celsius,
      pool: disk?.pool,
      fault: smartFault(disk, smart),
      lastTest: smart?.last_test_at || smart?.last_test_status
        ? { type: smart?.last_test_type, status: smart?.last_test_status, at: smart?.last_test_at }
        : undefined,
    };
  });
  const rank = (row: DiskRow) => (row.fault?.tone === "bad" ? 0 : row.fault ? 1 : 2);
  return rows.sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name, undefined, { numeric: true }));
}

/** A failure message for the screen: no bare status codes, no secrets. Empty input gives the fallback. */
export function failureText(raw: string, fallback: string): string {
  const cleaned = raw.replace(/\s*\(\d{3}\)\s*$/, "").trim();
  const generic = /^(action|request) failed$/i.test(cleaned) || /^failed to fetch$/i.test(cleaned);
  return sanitizeErrorMessage(generic ? "" : cleaned, fallback);
}
