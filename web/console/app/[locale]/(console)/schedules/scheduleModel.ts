import { timeValue } from "../actions/_parts/autoTime";

/** A schedule as the hub sends it (internal/schedules ScheduledTask). */
export interface Schedule {
  id: string;
  name: string;
  cron_expr: string;
  command: string;
  targets: string[];
  group_id?: string;
  enabled: boolean;
  last_run_at?: string;
  next_run_at?: string;
  last_run_status?: string;
  last_error?: string;
}

export type ScheduleInput = {
  name: string;
  cron_expr: string;
  command: string;
  targets: string[];
  group_id: string;
  enabled: boolean;
};

/**
 * The last run as a word and a tone. The hub's states: running, succeeded, failed, partial (some devices failed),
 * blocked (policy or downtime stopped it), skipped, cancelled; empty before the first run.
 */
export type LastRun = { word: "running" | "queued" | "failed" | "partial" | "blocked" | "skipped" | "cancelled" | "ok" | "never"; tone: "busy" | "bad" | "warn" | "plain" };

export function lastRun(schedule: Pick<Schedule, "last_run_status" | "last_run_at">): LastRun {
  const status = (schedule.last_run_status ?? "").toLowerCase();
  switch (status) {
    case "running": return { word: "running", tone: "busy" };
    case "queued": return { word: "queued", tone: "busy" };
    case "failed": return { word: "failed", tone: "bad" };
    case "partial": return { word: "partial", tone: "bad" };
    case "blocked": return { word: "blocked", tone: "warn" };
    case "skipped": return { word: "skipped", tone: "plain" };
    case "cancelled": return { word: "cancelled", tone: "plain" };
    default: return schedule.last_run_at || status ? { word: "ok", tone: "plain" } : { word: "never", tone: "plain" };
  }
}

export type ScheduleSort = { key: "name" | "next" | "last"; dir: "asc" | "desc" };

/** Sorted for the list. Next run puts paused schedules (no next run) last; last run puts never-run ones last. */
export function sortSchedules(list: Schedule[], sort: ScheduleSort): Schedule[] {
  const sign = sort.dir === "asc" ? 1 : -1;
  const byName = (a: Schedule, b: Schedule) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
  const next = (s: Schedule) => (s.enabled ? timeValue(s.next_run_at, Number.NaN) : Number.NaN);
  const last = (s: Schedule) => timeValue(s.last_run_at, Number.NaN);
  const by = (value: (s: Schedule) => number) => (a: Schedule, b: Schedule) => {
    const x = value(a);
    const y = value(b);
    if (Number.isNaN(x) || Number.isNaN(y)) return Number.isNaN(x) === Number.isNaN(y) ? byName(a, b) : Number.isNaN(x) ? 1 : -1;
    return sign * (x - y) || byName(a, b);
  };
  const compare = sort.key === "name" ? (a: Schedule, b: Schedule) => sign * byName(a, b) : by(sort.key === "next" ? next : last);
  return [...list].sort(compare);
}
