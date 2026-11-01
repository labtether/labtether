/** Shared helpers for the device Logs tab (stored lines, journal lines and action runs). */

/** A log level folded to the four the list shows: error, warn, info, or the raw word. */
export function normalizeLogLevel(level: string): string {
  const normalized = level.toLowerCase();
  if (normalized === "warning") return "warn";
  if (["err", "crit", "critical", "alert", "emerg"].includes(normalized)) return "error";
  if (normalized === "notice" || normalized === "debug") return "info";
  return normalized;
}

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

/** "14:02:31" today, "30 Oct 14:02" before today; the raw text when it is not a date. */
export function formatLogTime(iso: string, now = new Date()): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  if (sameDay(date, now)) {
    return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23" });
  }
  const day = date.toLocaleDateString([], { day: "numeric", month: "short" });
  return `${day} ${date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hourCycle: "h23" })}`;
}

/** The full local date and time, for a title. */
export function fullTime(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString();
}

/**
 * The part of a load error worth showing: the hub's own words without a trailing status code. The hook's generic
 * fallbacks ("failed to load …") say nothing the translated line does not, so they come back empty.
 */
export function cleanLoadError(message: string | null | undefined): string {
  const text = (message ?? "").trim();
  if (!text || /^failed to load/i.test(text)) return "";
  return text.replace(/\s*\(\d{3}\)\s*$/, "");
}
