/** A Remote View session recording as the hub lists it. */
export type Recording = {
  id: string;
  session_id: string;
  asset_id: string;
  protocol: string;
  file_size: number;
  duration_ms: number;
  status: string;
  created_at: string;
  stopped_at?: string;
  // Hub-relative attachment path, e.g. /recordings/{id}/download.
  download_url?: string;
};

const HUB_DOWNLOAD_PATH = /^\/recordings\/[^/?#]+\/download$/;

/**
 * The browser reaches Hub routes through the console's /api proxy, which adds the signed-in person's credentials.
 * Only a Hub path of the expected shape is linked.
 */
export function recordingDownloadHref(recording: Recording): string | null {
  const path = recording.download_url?.trim() ?? "";
  return HUB_DOWNLOAD_PATH.test(path) ? `/api${path}` : null;
}

/** "6 min 12 s", "45 s"; whole seconds. */
export function durationText(ms: number, locale = "en"): string {
  const total = Math.max(0, Math.round((Number.isFinite(ms) ? ms : 0) / 1000));
  const unit = (value: number, u: "minute" | "second") => new Intl.NumberFormat(locale, { style: "unit", unit: u, unitDisplay: "short" }).format(value);
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  if (minutes === 0) return unit(seconds, "second");
  return seconds ? `${unit(minutes, "minute")} ${unit(seconds, "second")}` : unit(minutes, "minute");
}

/** "41 KB", "186 MB", "1.2 GB" (1024-based, as the files are). */
export function sizeText(bytes: number, locale = "en"): string {
  const value = Number.isFinite(bytes) ? Math.max(0, bytes) : 0;
  const steps: Array<["kilobyte" | "megabyte" | "gigabyte", number]> = [["gigabyte", 1024 ** 3], ["megabyte", 1024 ** 2], ["kilobyte", 1024]];
  const [u, size] = steps.find(([, s]) => value >= s) ?? ["kilobyte", 1024];
  const n = value / size;
  return new Intl.NumberFormat(locale, { style: "unit", unit: u, unitDisplay: "short", maximumFractionDigits: n < 10 && u === "gigabyte" ? 1 : 0 }).format(u === "kilobyte" ? Math.max(1, n) : n);
}

export type RecordingSort = "when:desc" | "when:asc" | "dev:asc" | "dev:desc" | "dur:desc" | "dur:asc" | "size:desc" | "size:asc";

const ms = (r: Recording) => {
  const v = new Date(r.created_at).getTime();
  return Number.isNaN(v) ? 0 : v;
};

/** The recordings the filter and protocol chip show, in the chosen order (newest first by default). */
export function recordingRows(
  list: Recording[],
  opts: { query: string; protocol: string; sort: RecordingSort; nameOf: (assetID: string) => string },
): Recording[] {
  const q = opts.query.trim().toLowerCase();
  const hit = list.filter(
    (r) =>
      (opts.protocol === "all" || r.protocol.toLowerCase() === opts.protocol) &&
      (!q || `${opts.nameOf(r.asset_id)} ${r.asset_id} ${r.protocol}`.toLowerCase().includes(q)),
  );
  const [key, dir] = opts.sort.split(":") as ["when" | "dev" | "dur" | "size", "asc" | "desc"];
  const by = {
    when: (a: Recording, b: Recording) => ms(a) - ms(b),
    dev: (a: Recording, b: Recording) => opts.nameOf(a.asset_id).localeCompare(opts.nameOf(b.asset_id)),
    dur: (a: Recording, b: Recording) => a.duration_ms - b.duration_ms,
    size: (a: Recording, b: Recording) => a.file_size - b.file_size,
  }[key];
  const sign = dir === "asc" ? 1 : -1;
  return [...hit].sort((a, b) => sign * by(a, b) || ms(b) - ms(a) || a.id.localeCompare(b.id));
}
