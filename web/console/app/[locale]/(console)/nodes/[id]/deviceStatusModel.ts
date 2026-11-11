// What the device header and Details say about a device: its state as a word and a time, its address and the
// facts read from metadata. Pure functions, so the rules can be checked without rendering.
import { deviceFreshnessLabel } from "../../../../console/formatters";
import type { Asset } from "../../../../console/models";
import { assetCategory, childParentKey, hostParentKey } from "../../../../console/taxonomy";
import { resolveAssetPresentationStatus, type AssetPresentationStatus } from "../../../../lib/status";

export type DeviceStateWord = "online" | "notResponding" | "offline" | "noCheckIn";
export type DeviceState = { tone: "ok" | "warn" | "bad"; word: DeviceStateWord; since: Date | null };

function validDate(iso: string | undefined): Date | null {
  if (!iso) return null;
  const date = new Date(iso);
  return Number.isFinite(date.getTime()) && date.getTime() > 0 ? date : null;
}

/**
 * The device's state. An explicit status from the hub (offline, degraded …) wins over a fresh check-in; a stale
 * check-in turns a healthy status into "Not responding" and then "Offline". since: the last check-in, when known.
 */
export function deviceState(asset: Pick<Asset, "status" | "last_seen_at" | "metadata">): DeviceState {
  const fresh = deviceFreshnessLabel(asset) as AssetPresentationStatus;
  const resolved = resolveAssetPresentationStatus(asset.status ?? "", fresh);
  const since = validDate(asset.last_seen_at);
  if (resolved === "online") return { tone: "ok", word: "online", since: null };
  if (resolved === "unresponsive") return { tone: "warn", word: "notResponding", since };
  if (resolved === "unknown") return { tone: "warn", word: "noCheckIn", since: null };
  return { tone: "bad", word: "offline", since };
}

/** "21:12" for a time today, otherwise a short date ("28 Oct"), in the page's language. */
export function shortWhen(date: Date, locale: string, now: Date = new Date()): string {
  const sameDay = date.toDateString() === now.toDateString();
  return sameDay
    ? date.toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" })
    : date.toLocaleDateString(locale, { day: "numeric", month: "short" });
}

const meta = (asset: Asset, key: string): string => asset.metadata?.[key]?.trim() ?? "";

/** The address people reach the device on. */
export function deviceAddress(asset: Asset): string {
  return meta(asset, "ip_address") || meta(asset, "ip") || meta(asset, "primary_ip") || meta(asset, "collector_endpoint_host");
}

/** What it runs: the OS name, else the kind of device its integration reports. */
export function deviceOS(asset: Asset): string {
  return meta(asset, "os_pretty_name") || meta(asset, "os_name");
}

/** Days since the last backup from metadata: whole or decimal days, nothing else ("1e2", "+3" and "-1" are not days). */
export function parseBackupAgeDays(raw: string): number | null {
  const trimmed = raw.trim();
  if (!/^(?:0|[1-9]\d*)(?:\.\d+)?$/.test(trimmed)) return null;
  const days = Number(trimmed);
  return Number.isFinite(days) ? days : null;
}

/** Uptime in seconds from metadata as days, or hours and minutes; "" when missing. */
export function uptimeParts(asset: Asset): { days: number; hours: number; minutes: number } | null {
  const raw = meta(asset, "uptime") || meta(asset, "uptime_seconds");
  if (!/^\d+(?:\.\d+)?$/.test(raw)) return null;
  const total = Math.floor(Number(raw));
  return { days: Math.floor(total / 86400), hours: Math.floor((total % 86400) / 3600), minutes: Math.floor((total % 3600) / 60) };
}

/** The agent's version as metadata reports it. */
export function agentVersion(asset: Asset): string {
  return meta(asset, "agent_version") || meta(asset, "agent");
}

/** A hypervisor's guests (VMs and LXCs from the same integration), problems first, then by name. */
export function guestsOf(host: Asset, assets: Asset[]): Asset[] {
  const key = hostParentKey(host);
  const rank = (a: Asset) => (deviceState(a).tone === "ok" ? 1 : 0);
  return assets
    .filter((a) => a.id !== host.id && a.source === host.source && childParentKey(a) === key && assetCategory(a.type) === "compute")
    .sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name));
}
