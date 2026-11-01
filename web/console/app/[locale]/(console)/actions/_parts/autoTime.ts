// Times in the automation lists, in the browser's own format (as the Dashboard shows them): a clock time today, a
// weekday and time within a week, a date before or after that.

const DAY_MS = 86_400_000;

export function parseTime(iso?: string | null): Date | null {
  if (!iso) return null;
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? null : date;
}

function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}

/** Whole calendar days from a to b (b later is positive); DST days still count as one. */
function dayGap(a: Date, b: Date): number {
  return Math.round((startOfDay(b) - startOfDay(a)) / DAY_MS);
}

function parts(date: Date, locale?: string) {
  // The 24-hour clock everywhere in the console, as in the 2.0 design ("02:14", "13:27").
  const clock = new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(date);
  const weekday = new Intl.DateTimeFormat(locale, { weekday: "short" }).format(date);
  const day = new Intl.DateTimeFormat(locale, { day: "numeric", month: "short" }).format(date);
  const dayYear = new Intl.DateTimeFormat(locale, { day: "numeric", month: "short", year: "numeric" }).format(date);
  return { clock, weekday, day, dayYear };
}

/** A past time: "14:02" today, "Sat 10:04" in the last week, "20 Oct" before that (with the year when not this year). */
export function pastWhen(iso: string | null | undefined, now: Date = new Date(), locale?: string): string {
  const date = parseTime(iso);
  if (!date) return "";
  const days = dayGap(date, now);
  const p = parts(date, locale);
  if (days <= 0) return p.clock;
  if (days < 7) return `${p.weekday} ${p.clock}`;
  return date.getFullYear() === now.getFullYear() ? p.day : p.dayYear;
}

/** A coming time: "23:00" today, "Wed 02:00" within a week, "24 Nov 20:00" later. */
export function futureWhen(iso: string | null | undefined, now: Date = new Date(), locale?: string): string {
  const date = parseTime(iso);
  if (!date) return "";
  const days = dayGap(now, date);
  const p = parts(date, locale);
  if (days <= 0) return p.clock;
  if (days < 7) return `${p.weekday} ${p.clock}`;
  return `${date.getFullYear() === now.getFullYear() ? p.day : p.dayYear} ${p.clock}`;
}

/** Milliseconds for sorting; missing times sort as `missing`. */
export function timeValue(iso: string | null | undefined, missing: number): number {
  return parseTime(iso)?.getTime() ?? missing;
}
