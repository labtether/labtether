import type { Incident, IncidentEvent } from "../../../console/models";

/** What the hub sends for an issue beyond the console's Incident type: the device it is about and when it opened. */
export type Issue = Incident & { primary_asset_id?: string; opened_at?: string };
export type IssueStatus = Incident["status"];
export type IssueShow = "active" | "resolved" | "all";
/** The list shows a group of states (Active, Resolved, All) or one exact state. */
export type IssueFilter = IssueShow | IssueStatus;
export type IssueSortKey = "title" | "state" | "owner" | "opened";
export type IssueSort = { key: IssueSortKey; dir: "asc" | "desc" };
export type Tone = "bad" | "warn" | "ok" | "act" | "";

export const STATES: IssueStatus[] = ["open", "investigating", "mitigated", "resolved", "closed"];
export const SHOWS: IssueShow[] = ["active", "resolved", "all"];

export const isActive = (issue: Pick<Incident, "status">) =>
  issue.status !== "resolved" && issue.status !== "closed";

const validTime = (iso?: string) => Boolean(iso) && Number.isFinite(Date.parse(iso as string)) && !(iso as string).startsWith("0001-");

/** When the issue opened: the hub's opened_at, or its creation time from older hubs. */
export const openedAt = (issue: Issue) => (validTime(issue.opened_at) ? (issue.opened_at as string) : issue.created_at);

export function filterIssues<T extends Issue>(issues: T[], filter: IssueFilter): T[] {
  if (filter === "all") return issues;
  if (filter === "active") return issues.filter(isActive);
  if (filter === "resolved") return issues.filter((issue) => !isActive(issue));
  return issues.filter((issue) => issue.status === filter);
}

const stateRank = (status: string) => {
  const at = STATES.indexOf(status as IssueStatus);
  return at < 0 ? STATES.length : at;
};

const COMPARE: Record<IssueSortKey, (a: Issue, b: Issue) => number> = {
  title: (a, b) => a.title.localeCompare(b.title),
  state: (a, b) => stateRank(a.status) - stateRank(b.status),
  // Unassigned issues sort after every owner.
  owner: (a, b) => (a.assignee || "￿").localeCompare(b.assignee || "￿"),
  opened: (a, b) => Date.parse(openedAt(a)) - Date.parse(openedAt(b)),
};

/** Sorts a copy; ties fall back to the newest first, so the order is stable between polls. */
export function sortIssues<T extends Issue>(issues: T[], sort: IssueSort): T[] {
  const sign = sort.dir === "asc" ? 1 : -1;
  return [...issues].sort((a, b) => sign * COMPARE[sort.key](a, b) || COMPARE.opened(b, a) || a.id.localeCompare(b.id));
}

/** Clicking a column sorts by it; clicking it again flips the order. Times start newest first, words A to Z. */
export function nextSort(current: IssueSort, key: IssueSortKey): IssueSort {
  if (current.key === key) return { key, dir: current.dir === "asc" ? "desc" : "asc" };
  return { key, dir: key === "opened" ? "desc" : "asc" };
}

/**
 * The states the hub accepts from each state (incidents.CanTransitionStatus), in menu order. Offering any other
 * move would only fail on save.
 */
const MOVES: Record<IssueStatus, IssueStatus[]> = {
  open: ["investigating", "resolved", "closed"],
  investigating: ["mitigated", "resolved", "closed"],
  mitigated: ["investigating", "resolved", "closed"],
  resolved: ["investigating", "closed"],
  closed: ["investigating"],
};
export const nextStates = (status: IssueStatus): IssueStatus[] => MOVES[status] ?? [];

/** Critical issues read red and high ones amber; medium and low get no colour. */
export const severityTone = (severity: string): Tone => (severity === "critical" ? "bad" : severity === "high" ? "warn" : "");
/** A list row's dot: only an active critical or high issue has one. */
export const listTone = (issue: Issue): Tone => (isActive(issue) ? severityTone(issue.severity) : "");

/** A timeline dot: an alert firing is red, a recovery green, something someone ran solid; the rest a ring. */
export function eventTone(event: Pick<IncidentEvent, "event_type">): Tone {
  switch (event.event_type) {
    case "alert_fired": return "bad";
    case "alert_resolved": return "ok";
    case "action_run":
    case "update_run": return "act";
    default: return "";
  }
}

/** Oldest first, so the timeline reads down; the hub pages it newest first. */
export const chronological = (events: IncidentEvent[]) =>
  [...events].sort((a, b) => Date.parse(a.occurred_at) - Date.parse(b.occurred_at) || a.id.localeCompare(b.id));

/** "21:12" today, "18 Oct" earlier this year, "18 Oct 2025" before that. */
export function shortWhen(iso: string, locale: string, now: Date = new Date()): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  if (d.toDateString() === now.toDateString()) return d.toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  const sameYear = d.getFullYear() === now.getFullYear();
  return d.toLocaleDateString(locale, { day: "numeric", month: "short", ...(sameYear ? {} : { year: "numeric" }) });
}

/** The full date and time, for a tooltip on a short time. */
export const fullWhen = (iso: string, locale: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" });
};

/** The time between two moments as whole days, hours and minutes (at least one minute), or null if it runs backwards. */
export function durationParts(fromIso: string, toIso: string): { days: number; hours: number; minutes: number } | null {
  const ms = Date.parse(toIso) - Date.parse(fromIso);
  if (!Number.isFinite(ms) || ms < 0) return null;
  const total = Math.max(1, Math.floor(ms / 60_000));
  return { days: Math.floor(total / 1440), hours: Math.floor((total % 1440) / 60), minutes: total % 60 };
}

export const hasWriteup = (issue: Incident) =>
  Boolean(issue.root_cause?.trim() || issue.action_items?.some((item) => item.trim()) || issue.lessons_learned?.trim());
