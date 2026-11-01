import type { DeadLetterAnalytics, DeadLetterEvent, EndpointStatus, GroupReliability, MaintenanceWindow } from "../../../console/models";

export type Tone = "ok" | "warn" | "bad" | "busy" | "none";

/** A row of the Hub list: a name (a message key, or the hub's own name), a status word and the facts beside it. */
export type HubCheck = {
  key: string;
  name: { key: string } | { text: string };
  tone: Tone;
  word: string;
  values?: Record<string, string | number>;
  sub?: { key: string; values?: Record<string, string | number> } | { text: string };
  at?: string;
  latencyMs?: number;
};

const ENDPOINT_NAMES: Record<string, string> = { LabTether: "api", "Node Agent": "nodeAgent" };

/**
 * The hub's own parts and checks, worst news included as it is: the hub process (version, running since), each
 * endpoint it probes, failed background jobs and the data cleanup loop.
 */
export function hubChecks(input: {
  version?: { version?: string; started_at?: string } | null;
  endpoints: EndpointStatus[];
  analytics: DeadLetterAnalytics;
  deadLetters: DeadLetterEvent[];
  retentionError?: string;
}): HubCheck[] {
  const rows: HubCheck[] = [];
  if (input.version?.version || input.version?.started_at) {
    rows.push({
      key: "hub",
      name: { key: "hub" },
      tone: "ok",
      word: "running",
      sub: input.version.version ? { key: "version", values: { version: input.version.version } } : undefined,
      at: input.version.started_at,
    });
  }
  for (const endpoint of input.endpoints) {
    const known = ENDPOINT_NAMES[endpoint.name];
    rows.push({
      key: `endpoint:${endpoint.name}`,
      name: known ? { key: known } : { text: endpoint.name },
      tone: endpoint.ok ? "ok" : "bad",
      word: endpoint.ok ? "up" : endpoint.url ? "down" : "notSet",
      sub: endpoint.url ? { text: endpoint.url } : undefined,
      latencyMs: endpoint.ok ? endpoint.latencyMs : undefined,
    });
  }
  const failed = input.analytics.total;
  rows.push({
    key: "jobs",
    name: { key: "jobs" },
    tone: failed > 0 ? "bad" : "ok",
    word: failed > 0 ? "failedCount" : "noFailures",
    values: { count: failed, window: input.analytics.window || "24h" },
    at: failed > 0 ? latest(input.deadLetters) : undefined,
  });
  const cleanup = input.retentionError?.trim();
  rows.push({
    key: "cleanup",
    name: { key: "cleanup" },
    tone: cleanup ? "bad" : "ok",
    word: cleanup ? "failed" : "ok",
    sub: cleanup ? { text: cleanup } : undefined,
  });
  return rows;
}

/** The newest failure's time. */
function latest(events: DeadLetterEvent[]): string | undefined {
  let best: string | undefined;
  for (const e of events) if (!best || Date.parse(e.created_at) > Date.parse(best)) best = e.created_at;
  return best;
}

/** A group's health from its score, on the hub's grade lines: A (90+) good, B (75+) fair, below that poor. */
export function groupTone(row: Pick<GroupReliability, "score" | "maintenance_active">): { tone: Tone; word: string } {
  if (row.maintenance_active) return { tone: "busy", word: "downtime" };
  if (row.score >= 90) return { tone: "ok", word: "good" };
  if (row.score >= 75) return { tone: "warn", word: "fair" };
  return { tone: "bad", word: "poor" };
}

export type WindowState = "active" | "upcoming" | "ended";

export function windowState(w: Pick<MaintenanceWindow, "start_at" | "end_at">, now: number): WindowState {
  const end = Date.parse(w.end_at);
  if (!Number.isNaN(end) && end < now) return "ended";
  const start = Date.parse(w.start_at);
  return !Number.isNaN(start) && start <= now ? "active" : "upcoming";
}

const ORDER: Record<WindowState, number> = { active: 0, upcoming: 1, ended: 2 };

/** Running windows first, then the next to start, then ended ones with the latest first. */
export function sortWindows<T extends Pick<MaintenanceWindow, "start_at" | "end_at">>(windows: T[], now: number): T[] {
  return [...windows].sort((a, b) => {
    const sa = windowState(a, now);
    const sb = windowState(b, now);
    if (sa !== sb) return ORDER[sa] - ORDER[sb];
    if (sa === "ended") return Date.parse(b.end_at) - Date.parse(a.end_at);
    return Date.parse(a.start_at) - Date.parse(b.start_at);
  });
}

/** What a window does, as message keys: mutes alerts, pauses actions, pauses updates. */
export const windowEffects = (w: Pick<MaintenanceWindow, "suppress_alerts" | "block_actions" | "block_updates">) =>
  [w.suppress_alerts && "mutes", w.block_actions && "pausesActions", w.block_updates && "pausesUpdates"].filter(Boolean) as string[];
