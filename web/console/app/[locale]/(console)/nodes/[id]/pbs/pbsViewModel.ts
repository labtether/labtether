// Decisions behind the PBS views: which datastores the server reports, backup coverage ordered by risk, task kinds
// and states, the newest task of a kind, and plain schedule words.

import {
  backupStaleness,
  pbsTaskStatusBadge,
  type PBSAssetDetailsResponse,
  type PBSDatastoreSummary,
  type PBSGroupsResponse,
  type PBSTask,
} from "../pbsTabModel";

/** The datastores of a server (or the single one of a datastore asset), A–Z. */
export function pbsStores(details: PBSAssetDetailsResponse | null): PBSDatastoreSummary[] {
  if (!details) return [];
  const list = details.kind === "server" ? (details.datastores ?? []) : details.datastore ? [details.datastore] : [];
  return [...list].sort((a, b) => a.store.localeCompare(b.store));
}

/** Datastore names for pickers: the server's stores, or the single store of a datastore asset. */
export function pbsStoreNames(details: PBSAssetDetailsResponse | null): string[] {
  if (!details) return [];
  const stores = details.kind === "server"
    ? (details.datastores ?? []).map((datastore) => datastore.store)
    : [details.datastore?.store || details.store || ""];
  return Array.from(new Set(stores.map((store) => store.trim()).filter(Boolean))).sort();
}

export type CoverageRow = {
  key: string;
  store: string;
  /** Namespace; empty for the root. The same type/id can exist in two namespaces. */
  ns: string;
  type: string;
  id: string;
  comment?: string;
  owner?: string;
  count: number;
  last?: number;
  tone: "ok" | "warn" | "bad";
};

export type CoverageSortKey = "name" | "last" | "snaps";

/** Every backup group across the datastores with its staleness tone. */
export function coverageRows(groups: PBSGroupsResponse | null): CoverageRow[] {
  if (!groups) return [];
  return groups.datastores.flatMap((datastore) =>
    datastore.groups.map((group) => ({
      key: `${datastore.store}/${group.ns ? `${group.ns}|` : ""}${group.backup_type}/${group.backup_id}`,
      store: datastore.store,
      ns: group.ns ?? "",
      type: group.backup_type,
      id: group.backup_id,
      comment: group.comment?.trim() || undefined,
      owner: group.owner,
      count: group.backup_count,
      last: group.last_backup,
      tone: backupStaleness(group.last_backup),
    })),
  );
}

/** Where a group lives: its datastore, then its namespace when it is not the root. */
export function coverageWhere(row: Pick<CoverageRow, "store" | "ns">): string {
  return row.ns ? `${row.store}/${row.ns}` : row.store;
}

/** The group's name as people read it: its comment when PBS has one, else type/id. */
export function coverageName(row: Pick<CoverageRow, "comment" | "type" | "id">): string {
  return row.comment || `${row.type}/${row.id}`;
}

/** Coverage in the chosen order. "last" ascending puts groups with no backup first, then the oldest. */
export function sortCoverage(rows: CoverageRow[], key: CoverageSortKey, dir: "asc" | "desc"): CoverageRow[] {
  const sign = dir === "asc" ? 1 : -1;
  const byName = (a: CoverageRow, b: CoverageRow) => coverageName(a).localeCompare(coverageName(b)) || a.store.localeCompare(b.store);
  return [...rows].sort((a, b) => {
    let diff = 0;
    if (key === "last") diff = (a.last ?? 0) - (b.last ?? 0);
    else if (key === "snaps") diff = a.count - b.count;
    else diff = byName(a, b);
    return diff * sign || byName(a, b);
  });
}

export type TaskKind = "backup" | "verify" | "prune" | "gc" | "sync" | "restore";

const TASK_KINDS: Record<string, TaskKind> = {
  backup: "backup",
  verificationjob: "verify",
  verify: "verify",
  verify_group: "verify",
  verify_snapshot: "verify",
  datastore_verify: "verify",
  prune: "prune",
  prunejob: "prune",
  garbage_collection: "gc",
  gc: "gc",
  sync: "sync",
  syncjob: "sync",
  reader: "restore",
};

/** The kind of a PBS worker type, or null for types without a plain name (shown as PBS writes them). */
export function taskKind(workerType?: string): TaskKind | null {
  return TASK_KINDS[(workerType ?? "").toLowerCase()] ?? null;
}

export type TaskState = "running" | "failed" | "ok";

export function taskState(task: Pick<PBSTask, "status">): TaskState {
  const badge = pbsTaskStatusBadge(task.status);
  return badge === "bad" ? "failed" : badge === "pending" ? "running" : "ok";
}

/** Whether a task has ended, from the list entry and (when loaded) its status. */
export function taskFinished(task: PBSTask, status?: string, exitStatus?: string): boolean {
  const normalized = (status || task.status || "").trim().toLowerCase();
  return Boolean(task.endtime && task.endtime > 0)
    || Boolean(exitStatus?.trim())
    || Boolean(normalized && !normalized.includes("run") && !normalized.includes("active"));
}

export type TaskFilter = "all" | "errors" | "running";

export function filterTasks(tasks: PBSTask[], type: string, filter: TaskFilter): PBSTask[] {
  return tasks.filter((task) => {
    if (type !== "" && (task.worker_type || "unknown") !== type) return false;
    if (filter === "errors") return taskState(task) === "failed";
    if (filter === "running") return taskState(task) === "running";
    return true;
  });
}

/** The newest task of a kind whose worker id names the datastore (or starts with it). */
export function newestTask(tasks: PBSTask[], kind: TaskKind, store: string): PBSTask | undefined {
  let best: PBSTask | undefined;
  for (const task of tasks) {
    if (taskKind(task.worker_type) !== kind) continue;
    const target = task.worker_id ?? "";
    if (target !== store && !target.startsWith(`${store}:`)) continue;
    if (!best || (task.starttime ?? 0) > (best.starttime ?? 0)) best = task;
  }
  return best;
}

/** Tasks that started in the last day: how many ended well and how many failed. */
export function tasksLastDay(tasks: PBSTask[], now = Date.now()): { ok: number; failed: number } {
  let ok = 0;
  let failed = 0;
  for (const task of tasks) {
    if (!task.starttime || now / 1000 - task.starttime >= 86_400) continue;
    const state = taskState(task);
    if (state === "ok") ok += 1;
    if (state === "failed") failed += 1;
  }
  return { ok, failed };
}

/** The newest backup across datastores, as the datastore reports it. */
export function newestBackup(stores: PBSDatastoreSummary[]): string | undefined {
  let best: PBSDatastoreSummary | undefined;
  for (const store of stores) {
    if (typeof store.days_since_backup !== "number" || !store.last_backup_at) continue;
    if (!best || store.days_since_backup < (best.days_since_backup ?? Infinity)) best = store;
  }
  return best?.last_backup_at;
}

export type ScheduleWords =
  | { kind: "every"; unit: "hourly" | "daily" | "weekly" | "monthly" | "yearly" }
  | { kind: "dailyAt"; time: string }
  | { kind: "raw"; text: string };

/** A PBS calendar event in plain words where it is a common one; anything else stays as PBS writes it. */
export function scheduleWords(schedule?: string): ScheduleWords | null {
  const value = (schedule ?? "").trim();
  if (!value) return null;
  const lower = value.toLowerCase();
  if (lower === "hourly" || lower === "daily" || lower === "weekly" || lower === "monthly" || lower === "yearly") {
    return { kind: "every", unit: lower };
  }
  const match = /^(\d{1,2}):(\d{2})$/.exec(value);
  if (match) return { kind: "dailyAt", time: `${match[1].padStart(2, "0")}:${match[2]}` };
  return { kind: "raw", text: value };
}
