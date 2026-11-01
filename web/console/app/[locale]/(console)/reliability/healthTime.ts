/**
 * Times as the 2.0 lists show them: "21:12" today, "Fri 21:12" within a week either way, "12 Oct" (with the year when
 * it differs) further out. Spans drop the second date when both ends fall on the same day.
 */

const valid = (iso?: string) => {
  if (!iso) return null;
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? null : d;
};

const sameDay = (a: Date, b: Date) =>
  a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();

export function clock(iso: string | undefined, locale: string): string {
  const d = valid(iso);
  return d ? d.toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }) : "";
}

/** full keeps the time on dates further out too: "12 Oct 23:00". */
export function when(iso: string | undefined, now: number, locale: string, full = false): string {
  const d = valid(iso);
  if (!d) return "";
  const today = new Date(now);
  const time = d.toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  if (sameDay(d, today)) return time;
  if (Math.abs(d.getTime() - now) < 6 * 86_400_000) return `${d.toLocaleDateString(locale, { weekday: "short" })} ${time}`;
  const date = d.toLocaleDateString(locale, {
    day: "numeric",
    month: "short",
    ...(d.getFullYear() === today.getFullYear() ? {} : { year: "numeric" }),
  });
  return full ? `${date} ${time}` : date;
}

/** "Tue 23:00 – 01:00" when both ends share a day or the end is the next morning; both dates otherwise. */
export function span(startIso: string, endIso: string, now: number, locale: string): string {
  const start = valid(startIso);
  const end = valid(endIso);
  if (!start || !end) return "";
  const short = sameDay(start, end) || end.getTime() - start.getTime() < 86_400_000;
  return `${when(startIso, now, locale, true)} – ${short ? clock(endIso, locale) : when(endIso, now, locale, true)}`;
}
