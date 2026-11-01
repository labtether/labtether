// Schedule timing. The hub reads five-field cron (minute hour day month weekday) or an @descriptor, in UTC unless the
// line starts with CRON_TZ=Area/City (robfig/cron, hub internal/schedules/cron.go). The presets save their cron line in
// the browser's zone, so "Every day at 02:00" means 02:00 where the person is. nextRun() previews the next time.

export const CRON_PRESETS = [
  { id: "hourly", cron: "0 * * * *" },
  { id: "every6h", cron: "0 */6 * * *" },
  { id: "daily", cron: "0 2 * * *" },
  { id: "weekly", cron: "0 3 * * 0" },
  { id: "monthly", cron: "0 4 1 * *" },
] as const;

export type CronPresetID = (typeof CRON_PRESETS)[number]["id"];

const DESCRIPTORS: Record<string, string> = {
  "@yearly": "0 0 1 1 *",
  "@annually": "0 0 1 1 *",
  "@monthly": "0 0 1 * *",
  "@weekly": "0 0 * * 0",
  "@daily": "0 0 * * *",
  "@midnight": "0 0 * * *",
  "@hourly": "0 * * * *",
};
const MONTHS = ["JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"];
const WEEKDAYS = ["SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"];

/** The browser's IANA zone, or "UTC" when it has none. */
export function browserZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

/** Splits a CRON_TZ= (or TZ=) prefix from the schedule line. zone is "UTC" when there is none. */
export function splitZone(expr: string): { zone: string; spec: string } {
  const text = expr.trim().replace(/\s+/g, " ");
  const match = /^(?:CRON_TZ|TZ)=(\S+) (.+)$/.exec(text);
  return match ? { zone: match[1]!, spec: match[2]! } : { zone: "UTC", spec: text };
}

/** The line a preset saves: its cron in the given zone (no prefix for UTC). */
export function presetExpr(id: CronPresetID, zone: string): string {
  const cron = CRON_PRESETS.find((p) => p.id === id)?.cron ?? CRON_PRESETS[2].cron;
  return zone && zone !== "UTC" ? `CRON_TZ=${zone} ${cron}` : cron;
}

/** The preset a saved line is in this zone, or "" for any other line. */
export function presetOf(expr: string, zone: string): CronPresetID | "" {
  const { zone: own, spec } = splitZone(expr);
  if (own !== (zone || "UTC")) return "";
  return CRON_PRESETS.find((p) => p.cron === spec)?.id ?? "";
}

type Field = { values: Set<number>; star: boolean };

function value(text: string, names: string[] | null, offset: number): number | null {
  if (/^\d+$/.test(text)) return Number(text);
  const index = names ? names.indexOf(text.toUpperCase()) : -1;
  return index < 0 ? null : index + offset;
}

// One field: a star, ?, a, a-b, star/n, a/n, a-b/n, names and comma lists. star is set as robfig sets it (no step
// over 1).
function field(text: string, lo: number, hi: number, names: string[] | null = null, offset = 0): Field | null {
  const values = new Set<number>();
  let star = false;
  for (const part of text.split(",")) {
    const [range, stepText, extra] = part.split("/");
    if (extra !== undefined || !range) return null;
    const step = stepText === undefined ? 1 : /^\d+$/.test(stepText) ? Number(stepText) : 0;
    if (step < 1) return null;
    let from: number | null;
    let to: number | null;
    if (range === "*" || range === "?") {
      from = lo;
      to = hi;
      if (step === 1) star = true;
    } else {
      const [a, b, more] = range.split("-");
      if (more !== undefined || a === undefined) return null;
      from = value(a, names, offset);
      to = b === undefined ? (stepText === undefined ? from : hi) : value(b, names, offset);
    }
    if (from === null || to === null || from < lo || to > hi || from > to) return null;
    for (let x = from; x <= to; x += step) values.add(x);
  }
  return { values, star };
}

type Cron = { minutes: number[]; hours: number[]; dom: Field; month: Field; dow: Field };

/** Parses the schedule part (no zone prefix). null when the hub would refuse it. */
export function parseCron(spec: string): Cron | null {
  const text = DESCRIPTORS[spec.trim().toLowerCase()] ?? spec.trim();
  const fields = text.split(/\s+/);
  if (fields.length !== 5) return null;
  const minute = field(fields[0]!, 0, 59);
  const hour = field(fields[1]!, 0, 23);
  const dom = field(fields[2]!, 1, 31);
  const month = field(fields[3]!, 1, 12, MONTHS, 1);
  const dow = field(fields[4]!, 0, 6, WEEKDAYS, 0);
  if (!minute || !hour || !dom || !month || !dow) return null;
  const sorted = (f: Field) => [...f.values].sort((a, b) => a - b);
  return { minutes: sorted(minute), hours: sorted(hour), dom, month, dow };
}

function zoneFormat(zone: string): Intl.DateTimeFormat {
  return new Intl.DateTimeFormat("en-US", {
    timeZone: zone, hourCycle: "h23", year: "numeric", month: "numeric", day: "numeric", hour: "numeric", minute: "numeric",
  });
}

/** The wall clock in the zone at an instant, as a UTC-based millisecond count. */
function wallClock(format: Intl.DateTimeFormat, at: number): number {
  const p: Record<string, number> = {};
  for (const part of format.formatToParts(at)) if (part.type !== "literal") p[part.type] = Number(part.value);
  return Date.UTC(p.year!, p.month! - 1, p.day!, p.hour! % 24, p.minute!);
}

/** The instant a wall-clock time in the zone happens. */
function fromWall(format: Intl.DateTimeFormat, wall: number): number {
  const first = wall - (wallClock(format, wall) - wall);
  return wall - (wallClock(format, first) - first);
}

function dayMatches(cron: Cron, date: Date): boolean {
  if (!cron.month.values.has(date.getUTCMonth() + 1)) return false;
  const domHit = cron.dom.values.has(date.getUTCDate());
  const dowHit = cron.dow.values.has(date.getUTCDay());
  return cron.dom.star || cron.dow.star ? domHit && dowHit : domHit || dowHit;
}

/** The next run strictly after `after`, in the line's zone; null when the line is not valid or never runs. */
export function nextRun(expr: string, after: Date = new Date()): Date | null {
  const { zone, spec } = splitZone(expr);
  const cron = parseCron(spec);
  if (!cron) return null;
  let format: Intl.DateTimeFormat;
  try {
    format = zoneFormat(zone);
  } catch {
    return null;
  }
  const start = new Date(wallClock(format, after.getTime()));
  // Five years covers 29 February lines, as robfig does.
  for (let d = 0; d < 366 * 5; d++) {
    const day = new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth(), start.getUTCDate() + d));
    if (!dayMatches(cron, day)) continue;
    for (const hour of cron.hours) {
      for (const minute of cron.minutes) {
        const wall = Date.UTC(day.getUTCFullYear(), day.getUTCMonth(), day.getUTCDate(), hour, minute);
        const at = fromWall(format, wall);
        if (at > after.getTime()) return new Date(at);
      }
    }
  }
  return null;
}
