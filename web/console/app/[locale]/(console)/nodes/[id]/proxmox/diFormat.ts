// Plain number and time words for the integration tabs on a device page (Proxmox, PBS, TrueNAS, Home Assistant,
// storage). Sizes use 1024 steps with the console's usual unit names; times read like the rest of 2.0: "14:05" today,
// "Sat 02:10" this week, a date after that.

const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

function unitIndex(bytes: number): number {
  if (!Number.isFinite(bytes) || bytes < 1024) return 0;
  return Math.min(UNITS.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
}

function scaled(bytes: number, index: number): string {
  const value = bytes / Math.pow(1024, index);
  if (index === 0) return String(Math.round(value));
  if (value >= 100) return String(Math.round(value));
  return String(Number(value.toFixed(1)));
}

/** "612 MB", "3.4 TB"; "" when the value is missing. */
export function sizeText(bytes?: number | null): string {
  if (typeof bytes !== "number" || !Number.isFinite(bytes) || bytes < 0) return "";
  const index = unitIndex(bytes);
  return `${scaled(bytes, index)} ${UNITS[index]}`;
}

/** The number and the unit apart, for a number cell with a grey unit. */
export function sizeParts(bytes?: number | null): { value: string; unit: string } | null {
  const text = sizeText(bytes);
  if (!text) return null;
  const [value, unit] = text.split(" ");
  return { value, unit };
}

/** Used and total in the unit of the total, for "3.3 of 3.7 TB"; null when either is missing. */
export function ofParts(used?: number | null, total?: number | null): { used: string; total: string; unit: string } | null {
  if (typeof used !== "number" || typeof total !== "number" || !Number.isFinite(used) || !Number.isFinite(total) || total <= 0) {
    return null;
  }
  const index = unitIndex(total);
  return { used: scaled(Math.max(0, used), index), total: scaled(total, index), unit: UNITS[index] };
}

/** A 0–100 percent from used and total, or null when either is missing. */
export function percentOf(used?: number | null, total?: number | null): number | null {
  if (typeof used !== "number" || typeof total !== "number" || !Number.isFinite(used) || !Number.isFinite(total) || total <= 0) {
    return null;
  }
  return Math.max(0, Math.min(100, (used / total) * 100));
}

/** Seconds since 1970 or an ISO string to milliseconds; null when unreadable or zero. */
export function toMillis(value?: number | string | null): number | null {
  if (typeof value === "number") return Number.isFinite(value) && value > 0 ? value * 1000 : null;
  if (typeof value === "string" && value.trim() !== "") {
    const ms = new Date(value).getTime();
    return Number.isFinite(ms) && ms > 0 ? ms : null;
  }
  return null;
}

const DAY = 86_400_000;

function startOfDay(ms: number): number {
  const d = new Date(ms);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

/**
 * When something happened, the way 2.0 lists show it: "14:05" today, "Sat 02:10" within the last six days, "18 Oct"
 * this year, "18 Oct 2025" before that. Future times read the same way. Empty when the time is missing.
 */
export function whenText(value: number | string | null | undefined, locale: string, now = Date.now()): string {
  const ms = toMillis(value);
  if (ms == null) return "";
  const time = new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(ms);
  const days = Math.round((startOfDay(now) - startOfDay(ms)) / DAY);
  if (days === 0) return time;
  if (days > 0 && days < 7) {
    const weekday = new Intl.DateTimeFormat(locale, { weekday: "short" }).format(ms);
    return `${weekday} ${time}`;
  }
  const sameYear = new Date(ms).getFullYear() === new Date(now).getFullYear();
  return new Intl.DateTimeFormat(locale, sameYear ? { day: "numeric", month: "short" } : { day: "numeric", month: "short", year: "numeric" }).format(ms);
}

/** "41 s", "14 min", "2 h 5 min", "3 d"; empty for a missing or negative span. */
export function spanText(seconds?: number | null): string {
  if (typeof seconds !== "number" || !Number.isFinite(seconds) || seconds < 0) return "";
  if (seconds < 60) return `${Math.max(1, Math.round(seconds))} s`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) {
    const rest = minutes % 60;
    return rest ? `${hours} h ${rest} min` : `${hours} h`;
  }
  return `${Math.floor(hours / 24)} d`;
}

/** Whole days from now until a time (negative once it has passed); null when missing. */
export function daysUntil(value: number | string | null | undefined, now = Date.now()): number | null {
  const ms = toMillis(value);
  if (ms == null) return null;
  return Math.floor((ms - now) / DAY);
}

/** Status tone of a percent: amber at 85% or more, as the 2.0 meters show it. */
export function usageTone(pct: number | null | undefined): "warn" | "" {
  return typeof pct === "number" && pct >= 85 ? "warn" : "";
}
