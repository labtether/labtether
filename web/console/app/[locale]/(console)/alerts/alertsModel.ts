import type { AlertInstance, AlertSilence } from "../../../console/models";
import { SEVERITIES, type AlertSort, type DeliveryRecord, type InboxFilter } from "./alertsPageTypes";

/** Severity order as the hub ranks it: critical, high, medium, low; anything else sorts last. */
export const sevRank = (severity: string) => {
  const i = SEVERITIES.indexOf(severity as (typeof SEVERITIES)[number]);
  return i < 0 ? SEVERITIES.length : i;
};

/** Critical reads red and high amber; medium and low carry no dot (their word is enough). */
export const sevTone = (severity: string): "bad" | "warn" | "" =>
  severity === "critical" ? "bad" : severity === "high" ? "warn" : "";

const ts = (iso: string | null | undefined) => {
  const n = iso ? Date.parse(iso) : NaN;
  return Number.isFinite(n) ? n : 0;
};

/** The alert's title: the rule name it carries, else the rule's current name; '' when neither is known. */
export const alertTitle = (alert: AlertInstance, ruleNames: Map<string, string>) =>
  alert.labels?.rule_name || ruleNames.get(alert.rule_id) || "";

/**
 * The active silence that covers this alert, if any. The hub matches a silence against the rule's labels (every
 * matcher must equal one), and an alert carries its rule's labels.
 */
export function silenceFor(alert: AlertInstance, silences: AlertSilence[], now: number): AlertSilence | null {
  const labels = alert.labels ?? {};
  return silences.find((silence) => {
    const matchers = Object.entries(silence.matchers ?? {});
    if (matchers.length === 0) return false;
    if (!(ts(silence.starts_at) <= now && now < ts(silence.ends_at))) return false;
    return matchers.every(([key, value]) => labels[key] === value);
  }) ?? null;
}

function compareAlerts(sort: AlertSort) {
  const newest = (a: AlertInstance, b: AlertInstance) => ts(b.started_at) - ts(a.started_at);
  switch (sort) {
    case "severity": return (a: AlertInstance, b: AlertInstance) => sevRank(a.severity) - sevRank(b.severity) || newest(a, b);
    case "severityAsc": return (a: AlertInstance, b: AlertInstance) => sevRank(b.severity) - sevRank(a.severity) || newest(a, b);
    case "oldest": return (a: AlertInstance, b: AlertInstance) => -newest(a, b);
    default: return newest;
  }
}

export type InboxGroups = {
  /** Firing, acknowledged and pending alerts no silence covers, in the chosen order (most severe first by default). */
  open: AlertInstance[];
  /** Alerts still open that a silence covers. */
  silenced: AlertInstance[];
  /** Resolved alerts, the latest first (oldest first when that is the chosen order). */
  resolved: AlertInstance[];
  shown: number;
  total: number;
};

/** The inbox: the filter applied, then open alerts, silenced ones and resolved ones. */
export function groupAlerts(instances: AlertInstance[], silences: AlertSilence[], filter: InboxFilter, now: number): InboxGroups {
  const hit = instances.filter((a) =>
    (filter.severity === "all" || a.severity === filter.severity) && (filter.state === "all" || a.status === filter.state));
  const sorted = [...hit].sort(compareAlerts(filter.sort));
  const groups: InboxGroups = { open: [], silenced: [], resolved: [], shown: hit.length, total: instances.length };
  for (const alert of sorted) {
    if (alert.status === "resolved") groups.resolved.push(alert);
    else if (alert.suppressed_by || silenceFor(alert, silences, now)) groups.silenced.push(alert);
    else groups.open.push(alert);
  }
  const ended = (a: AlertInstance) => ts(a.resolved_at ?? a.updated_at);
  groups.resolved.sort((a, b) => (filter.sort === "oldest" ? ended(a) - ended(b) : ended(b) - ended(a)));
  return groups;
}

/** The alert to show beside the list: the chosen one while it is listed, else the first listed. */
export function pickAlert(groups: InboxGroups, selectedId: string | null): AlertInstance | null {
  const listed = [...groups.open, ...groups.silenced, ...groups.resolved];
  return listed.find((a) => a.id === selectedId) ?? listed[0] ?? null;
}

export type TimelineEntry = {
  at: string;
  kind: "opened" | "fired" | "acknowledged" | "resolved" | "sent" | "failed" | "pending";
  tone: "bad" | "warn" | "";
  channelId?: string;
};

/**
 * What the hub recorded for this alert, oldest first: opened, last fired (when it fired again later), acknowledged,
 * resolved, and each notification sent for it.
 */
export function alertTimeline(alert: AlertInstance, deliveries: DeliveryRecord[]): TimelineEntry[] {
  const entries: TimelineEntry[] = [{ at: alert.started_at, kind: "opened", tone: sevTone(alert.severity) }];
  if (ts(alert.last_fired_at) - ts(alert.started_at) > 60_000) entries.push({ at: alert.last_fired_at, kind: "fired", tone: "" });
  if (alert.status === "acknowledged") entries.push({ at: alert.updated_at, kind: "acknowledged", tone: "" });
  if (alert.resolved_at) entries.push({ at: alert.resolved_at, kind: "resolved", tone: "" });
  for (const d of deliveries) {
    if (d.alert_instance_id !== alert.id) continue;
    entries.push({ at: d.sent_at ?? d.created_at, kind: d.status === "sent" ? "sent" : d.status === "failed" ? "failed" : "pending", tone: d.status === "failed" ? "bad" : "", channelId: d.channel_id });
  }
  return entries.sort((a, b) => ts(a.at) - ts(b.at));
}

/** "21:12" (24-hour, as the mockup) for today, else "3 Nov" (with the year when it is not this year). */
export function shortTime(iso: string | null | undefined, locale: string, now: number): string {
  const n = ts(iso);
  if (!n) return "";
  const d = new Date(n);
  const today = new Date(now);
  if (d.toDateString() === today.toDateString()) return d.toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  const year = d.getFullYear() === today.getFullYear() ? undefined : "numeric";
  return d.toLocaleDateString(locale, { day: "numeric", month: "short", year });
}

/** "3 Nov 2026, 21:12": a full 24-hour time for the facts list. */
export function fullTime(iso: string | null | undefined, locale: string): string {
  const n = ts(iso);
  return n ? new Date(n).toLocaleString(locale, { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }) : "";
}

/** A count of seconds in its largest whole unit: 7200 → 2 h, 300 → 5 min, 90 → 90 s. */
export function splitSeconds(seconds: number): { unit: "h" | "min" | "s"; n: number } {
  if (seconds >= 3600 && seconds % 3600 === 0) return { unit: "h", n: seconds / 3600 };
  if (seconds >= 60 && seconds % 60 === 0) return { unit: "min", n: seconds / 60 };
  return { unit: "s", n: seconds };
}
