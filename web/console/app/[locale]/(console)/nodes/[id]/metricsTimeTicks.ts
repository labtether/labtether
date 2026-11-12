import type { ChartPoint } from "./nodeMetricsChartGeometry";
import { formatDay, formatTs } from "./nodeMetricsPresentationModel";

export type TimeTick = { x: number; label: string };

const HOUR = 3600;
const DAY = 24 * HOUR;
const CLOCK_STEPS = [5, 10, 15, 30, 60, 120, 180, 240, 360, 720, 1440].map((m) => m * 60);
const DAY_STEPS = [1, 2, 5, 10];

/**
 * Up to four labels under a chart, placed where those times fall on it. Points within a day get round local clock
 * times (00:00, 06:00 …). A longer chart gets dates at local midnight ("7 Nov"): a clock time repeats every day.
 */
export function timeTicks(points: ChartPoint[], most = 4): TimeTick[] {
  if (points.length < 2) return [];
  const a = points[0];
  const b = points[points.length - 1];
  const span = b.ts - a.ts;
  if (span <= 0) return [];
  const xAt = (ts: number) => a.x + ((ts - a.ts) / span) * (b.x - a.x);
  const ticks: TimeTick[] = [];

  // A 24h range can span a little over a day once its samples are grouped into steps.
  if (span <= DAY + HOUR) {
    const step = CLOCK_STEPS.find((s) => span / s <= most) ?? CLOCK_STEPS[CLOCK_STEPS.length - 1];
    const offset = new Date(a.ts * 1000).getTimezoneOffset() * 60;
    for (let t = Math.ceil((a.ts - offset) / step) * step + offset; t <= b.ts; t += step) {
      ticks.push({ x: xAt(t), label: formatTs(t) });
    }
    return ticks;
  }

  const days = DAY_STEPS.find((d) => span / (d * DAY) <= most) ?? DAY_STEPS[DAY_STEPS.length - 1];
  const day = new Date(a.ts * 1000);
  day.setHours(0, 0, 0, 0);
  if (day.getTime() / 1000 < a.ts) day.setDate(day.getDate() + 1);
  // Step by calendar day, so a tick stays on midnight across a daylight-saving change.
  for (; day.getTime() / 1000 <= b.ts; day.setDate(day.getDate() + days)) {
    const t = day.getTime() / 1000;
    ticks.push({ x: xAt(t), label: formatDay(t) });
  }
  return ticks;
}
