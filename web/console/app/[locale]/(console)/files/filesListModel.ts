// Pure helpers for the 2.0 file list, shared by the Files page and a device's Files tab: short modified times, the
// search filter, click selection and which actions an item menu offers. Paths and their crumbs are in remotePaths.ts.

/** "14:02" today (24-hour clock), "2 Oct" this year, "2 Oct 2024" before. The raw text when it is not a date. */
export function formatModified(raw: string, locale: string, now: Date = new Date()): string {
  if (!raw) return "";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  if (d.toDateString() === now.toDateString()) {
    return d.toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  }
  const opts: Intl.DateTimeFormatOptions = d.getFullYear() === now.getFullYear()
    ? { day: "numeric", month: "short" }
    : { day: "numeric", month: "short", year: "numeric" };
  return d.toLocaleDateString(locale, opts);
}

type Named = { name: string };
type Sized = { name: string; size: number; is_dir: boolean };

/** Search in this folder: names that contain the text, any case. */
export function filterEntries<T extends Named>(entries: T[], query: string): T[] {
  const q = query.trim().toLowerCase();
  return q ? entries.filter((e) => e.name.toLowerCase().includes(q)) : entries;
}

/** Bytes of the picked files (folders count as nothing: the agent does not size them). */
export function pickedBytes(entries: Sized[], picked: ReadonlySet<string>): number {
  return entries.reduce((sum, e) => (picked.has(e.name) && !e.is_dir ? sum + (e.size || 0) : sum), 0);
}

export type ClickMods = { toggle: boolean; range: boolean };

/**
 * A click on a row: alone it picks only that row; ⌘/Ctrl adds or removes it; Shift picks the run from the last
 * clicked row (added to the current picks with ⌘/Ctrl). ordered is the list as shown.
 */
export function clickSelection(
  prev: ReadonlySet<string>,
  ordered: string[],
  name: string,
  anchor: string | null,
  mods: ClickMods,
): { selected: Set<string>; anchor: string } {
  if (mods.range && anchor && ordered.includes(anchor) && ordered.includes(name)) {
    const a = ordered.indexOf(anchor);
    const b = ordered.indexOf(name);
    const next = mods.toggle ? new Set(prev) : new Set<string>();
    for (const n of ordered.slice(Math.min(a, b), Math.max(a, b) + 1)) next.add(n);
    return { selected: next, anchor };
  }
  if (mods.toggle) {
    const next = new Set(prev);
    if (next.has(name)) next.delete(name);
    else next.add(name);
    return { selected: next, anchor: name };
  }
  return { selected: new Set([name]), anchor: name };
}

export type MenuAction =
  | "open" | "download" | "rename" | "copy" | "cut" | "paste" | "copyPath" | "delete"
  | "newFolder" | "uploadHere" | "refresh";
export type MenuSlot = MenuAction | "sep";

export type MenuContext = {
  /** The row clicked, or null for the folder background. */
  entry: { name: string; is_dir: boolean } | null;
  /** How many items the action works on (the picked rows, or the one row). */
  count: number;
  /** Some of those items are files (Download skips folders). */
  hasFiles: boolean;
  canPaste: boolean;
  /** Changes are allowed (a device's Files tab starts read-only). */
  canWrite: boolean;
};

/** Which actions an item's ⋯ or right-click menu offers, with separators between groups. */
export function menuActions(ctx: MenuContext): MenuSlot[] {
  const { entry, count, hasFiles, canPaste, canWrite } = ctx;
  const one = count <= 1;
  const out: MenuSlot[] = [];
  if (!entry) {
    if (canWrite) out.push("newFolder", "uploadHere");
    if (canPaste && canWrite) out.push("paste");
    out.push("sep", "refresh");
    return tidy(out);
  }
  if (one) out.push("open");
  if (hasFiles) out.push("download");
  out.push("sep");
  if (one && canWrite) out.push("rename");
  out.push("copy", "cut");
  if (canPaste && canWrite && (entry.is_dir || one)) out.push("paste");
  if (one) out.push("copyPath");
  if (entry.is_dir && one && canWrite) out.push("uploadHere");
  if (canWrite) out.push("sep", "delete");
  return tidy(out);
}

/** No separator first, last or twice in a row. */
function tidy(slots: MenuSlot[]): MenuSlot[] {
  const out: MenuSlot[] = [];
  for (const s of slots) {
    if (s === "sep" && (out.length === 0 || out[out.length - 1] === "sep")) continue;
    out.push(s);
  }
  while (out[out.length - 1] === "sep") out.pop();
  return out;
}

const DEFAULT_PORTS: Record<string, number[]> = { sftp: [22], smb: [445], ftp: [21], webdav: [443, 80] };

/** A saved connection's address as people write it: "10.20.0.21/media", "backup.example.net:2222/home/demo". */
export function connectionAddress(c: { protocol: string; host: string; port?: number; initial_path?: string; extra_config?: Record<string, unknown> }): string {
  const port = c.port && !(DEFAULT_PORTS[c.protocol] ?? []).includes(c.port) ? `:${c.port}` : "";
  const rawShare = c.extra_config?.smb_share;
  const share = c.protocol === "smb" && typeof rawShare === "string" ? rawShare.replace(/^\/+/, "") : "";
  const path = share ? `/${share}` : c.initial_path && c.initial_path !== "/" ? `/${c.initial_path.replace(/^\/+/, "")}` : "";
  return `${c.host}${port}${path}`;
}

/** "sftp://host/path" typed into Connect to: the protocol to make a new connection for, or null. */
export function parseQuickConnect(input: string): { protocol: string; host: string; path: string } | null {
  const match = input.trim().match(/^(sftp|smb|ftp|webdav):\/\/([^/]+)(\/.*)?$/i);
  if (!match) return null;
  return { protocol: match[1].toLowerCase(), host: match[2], path: match[3] ?? "/" };
}
