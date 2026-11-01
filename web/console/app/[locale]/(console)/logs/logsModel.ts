import type { LogEvent } from "../../../console/models";

/** The level word a line shows: warn and error in colour, debug grey, info none. Agents send a few spellings. */
export type LevelWord = "error" | "warn" | "debug" | "info";

export function levelWord(level: string | undefined): LevelWord {
  const value = (level ?? "").trim().toLowerCase();
  if (value === "error" || value === "err" || value === "critical" || value === "crit" || value === "fatal" || value === "alert" || value === "emerg") {
    return "error";
  }
  if (value === "warn" || value === "warning") return "warn";
  if (value === "debug" || value === "trace") return "debug";
  return "info";
}

const pad = (n: number) => String(n).padStart(2, "0");

/**
 * The time cell: "22:40:12" today, "Mon 22:40" in the last six days, "28 Oct 22:40" before that.
 * Uses the viewer's clock; an unreadable time comes back as it was sent.
 */
export function logTime(timestamp: string, now: Date, locale = "en"): string {
  const at = new Date(timestamp);
  if (Number.isNaN(at.getTime())) return timestamp;
  const hm = `${pad(at.getHours())}:${pad(at.getMinutes())}`;
  const sameDay = at.getFullYear() === now.getFullYear() && at.getMonth() === now.getMonth() && at.getDate() === now.getDate();
  if (sameDay) return `${hm}:${pad(at.getSeconds())}`;
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  if (at.getTime() >= startOfToday - 6 * 86_400_000 && at.getTime() < startOfToday) {
    return `${new Intl.DateTimeFormat(locale, { weekday: "short" }).format(at)} ${hm}`;
  }
  return `${new Intl.DateTimeFormat(locale, { day: "numeric", month: "short" }).format(at)} ${hm}`;
}

const timeOf = (event: LogEvent) => {
  const ms = new Date(event.timestamp).getTime();
  return Number.isNaN(ms) ? 0 : ms;
};

/** Newest first; the stream can arrive slightly out of order. Equal times keep the order the hub sent. */
export function newestFirst(events: LogEvent[]): LogEvent[] {
  return events
    .map((event, index) => ({ event, index }))
    .sort((a, b) => timeOf(b.event) - timeOf(a.event) || a.index - b.index)
    .map(({ event }) => event);
}

/** The writer of a line for context: its device, or its source when the hub wrote it. */
const writer = (event: LogEvent) => event.asset_id || `source:${event.source}`;

/** Up to `n` lines either side of `event` from the same device, out of a newest-first list. */
export function contextLines(events: LogEvent[], event: LogEvent, n = 2): { newer: LogEvent[]; older: LogEvent[] } {
  const same = events.filter((candidate) => writer(candidate) === writer(event));
  const index = same.findIndex((candidate) => candidate.id === event.id);
  if (index < 0) return { newer: [], older: [] };
  return { newer: same.slice(Math.max(0, index - n), index), older: same.slice(index + 1, index + 1 + n) };
}

/** A line as it reads in a log file: time, device, level, source, message. */
export function lineText(event: LogEvent, time: string, device: string): string {
  return [time, device, levelWord(event.level), event.source, event.message].filter(Boolean).join(" ");
}

/** Lines in the order a log file reads (oldest first), for copying a selection. */
export function selectedText(events: LogEvent[], selected: ReadonlySet<string>, text: (event: LogEvent) => string): string {
  return events.filter((event) => selected.has(event.id)).reverse().map(text).join("\n");
}
