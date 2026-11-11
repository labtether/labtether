import type { AuditEvent } from "../../../console/models";

/** An audit entry as the hub sends it (the full status carries the newest 20 this person may see). */
export type AuditEntry = AuditEvent & {
  actor_id?: string;
  reason?: string;
  session_id?: string;
  command_id?: string;
  details?: Record<string, unknown>;
};

/** Event types with a sentence of their own in audit-log.json (actions.<type with _ for .>). */
export const KNOWN_ACTIONS = new Set([
  "terminal.session.created", "terminal.quick_session.created", "terminal.command.queued", "terminal.command.executed",
  "terminal.command.completed", "terminal.command.deleted", "actions.run.queued", "actions.run.executed",
  "actions.run.completed", "action.failed", "updates.run.queued", "updates.run.executed", "updates.run.completed",
  "schedules.run.completed", "auth.oidc.login", "api_key.created", "api_key.deleted", "webhook.created",
  "webhook.updated", "webhook.deleted", "asset.manual.created", "api.asset.deleted", "asset.power",
  "enrollment.token.created", "agent.token.revoked", "credential.profile.created", "credential.profile.deleted",
  "credential.profile.rotated", "credential.desktop.retrieved", "notification.channel.created",
  "notification.channel.updated", "notification.channel.deleted", "notification.channel.tested",
  "settings.runtime.updated", "settings.runtime.reset", "settings.retention.updated", "settings.tls.updated", "settings.tls.cleared",
  "hub.tls.renewed", "settings.restart.requested", "admin.reset", "protocol.config.created",
  "protocol.config.updated", "protocol.config.deleted", "protocol.test.run", "protocol.ssh.hub_key_pushed",
  "api.exec", "mcp.tool", "api.bulk.file_push", "api.bulk.service_action", "api.request", "terminal.command.policy_checked",
  "actions.run.policy_checked", "api.exec.policy_checked", "queue.dead_letter", "asset.hub_ssh_key.updated",
  "settings.ssh_hub_key.rotation", "settings.managed_database.password_revealed", "api_key.updated",
  "schedule.created", "schedule.updated", "schedule.deleted", "alert.rule.created", "alert.rule.updated",
  "alert.rule.deleted", "group.created", "group.updated", "group.deleted", "asset.updated", "asset.wake.requested",
  "asset.wake.outcome", "asset.wake.relay_dispatch", "asset.wake.relay_result", "maintenance_window.created",
  "maintenance_window.updated", "maintenance_window.deleted", "alert.acknowledged", "alert.resolved", "alert.deleted",
  "alert.silence.created", "alert.silence.deleted", "alert.route.created", "alert.route.updated", "alert.route.deleted",
  "incident.created", "incident.updated", "incident.deleted", "updates.plan.created", "updates.plan.deleted",
  "terminal.bookmark.created", "terminal.bookmark.updated", "terminal.bookmark.deleted", "auth.user.created",
  "auth.user.deleted", "auth.user.role_changed", "auth.user.password_reset", "auth.user.sessions_revoked",
  "auth.password.changed", "auth.2fa.enabled", "auth.2fa.disabled", "saved_action.created", "saved_action.deleted",
  "saved_action.run.completed", "settings.oidc.updated", "settings.oidc.applied",
]);

/** Every API change the Hub stores beside its named event; shown only under "All requests". */
export const REQUEST_TYPE = "api.request";

/** The type filter value that also shows API requests. */
export const ALL_REQUESTS = "requests";

/**
 * A Hub-made ID such as usr_<nanoseconds>_<counter> or dlq_<nanoseconds> (internal/idgen: a prefix, the time
 * in nanoseconds, a counter). It tells a person nothing, so the normal row view leaves it out; the row's details
 * keep it.
 */
export function isGeneratedID(value: string): boolean {
  return /^[a-z][a-z0-9-]{0,39}_[0-9]{10,}(_[0-9]+)?$/.test(value.trim());
}

/**
 * The object's name the Hub stored with the row (details.target_name). Without one, its own ID when a person chose
 * it (a device's ID is its host name), and nothing when the Hub made the ID up.
 */
export function targetText(entry: Pick<AuditEntry, "target" | "details">): string {
  const name = entry.details?.target_name;
  if (typeof name === "string" && name.trim()) return name.trim();
  const id = (entry.target ?? "").trim();
  return isGeneratedID(id) ? "" : id;
}

/** Who did it: a name, or a kind of actor the row reads in words ("Hub", "API key", "Unknown"). */
export type AuditActor = { name: string } | { kind: "hub" | "apiKey" | "unknown" };

/**
 * The person (or key) behind a row. A row carries an ID only, so names come from the Hub's user list (admins see
 * it) and the signed-in person. With no name, an API key reads "API key" and a Hub-made user ID "Unknown"; any
 * other ID (a device acting for itself) is its own name.
 */
export function auditActor(actorID: string | undefined, names: ReadonlyMap<string, string>): AuditActor {
  const id = (actorID ?? "").trim();
  if (!id) return { kind: "unknown" };
  if (id === "system") return { kind: "hub" };
  const name = names.get(id)?.trim();
  if (name) return { name };
  if (id.startsWith("apikey:")) return { kind: "apiKey" };
  return isGeneratedID(id) ? { kind: "unknown" } : { name: id };
}

/**
 * The IDs the normal row view left out, for the row's details, where a sysadmin looks on purpose: the actor's when
 * the row reads "API key" or "Unknown", the object's when the row shows no name for it.
 */
export function hiddenIDs(entry: Pick<AuditEntry, "actor_id" | "target" | "details">, actor: AuditActor): Array<[string, string]> {
  const rows: Array<[string, string]> = [];
  const actorID = (entry.actor_id ?? "").trim();
  if (actorID && "kind" in actor && actor.kind !== "hub") {
    rows.push(actor.kind === "apiKey" ? [ID_KEYS.apiKey, actorID.slice("apikey:".length)] : [ID_KEYS.actor, actorID]);
  }
  const target = (entry.target ?? "").trim();
  if (target && !targetText(entry)) rows.push([ID_KEYS.target, target]);
  return rows;
}

/** Keys of the hiddenIDs rows; they cannot clash with the Hub's own detail keys. */
export const ID_KEYS = { actor: "@actor", apiKey: "@api_key", target: "@target" } as const;

/**
 * The rows this person may see: the whole log for the owner and admins, their own rows for everyone else. Not access
 * control: the Hub decides which rows it sends. This only keeps rows already in the page from showing to a person
 * who may not see them (a role changed while the page was open, or a Hub that sent more than it should).
 */
export function visibleAuditRows(entries: AuditEntry[], wholeLog: boolean, actorID: string | undefined): AuditEntry[] {
  if (wholeLog) return entries;
  return actorID ? entries.filter((entry) => entry.actor_id === actorID) : [];
}

/** The message key for a known type ("terminal.session.created" → "terminal_session_created"), or "". */
export function actionKey(type: string): string {
  return KNOWN_ACTIONS.has(type) ? type.replace(/\./g, "_") : "";
}

/**
 * The sentence for a row: its type's own (actions.<key>), or a closer one when the row's details say what changed.
 * A renamed device reads "Renamed nas01 to" before its new name; an issue someone resolved reads "Resolved an
 * issue", not "Changed an issue". key is "" for a type without a sentence.
 */
export function actionSentence(entry: Pick<AuditEntry, "type" | "details">): { key: string; values?: Record<string, string> } {
  const key = actionKey(entry.type);
  const details = entry.details ?? {};
  const changed = Array.isArray(details.changed) ? details.changed.filter((field): field is string => typeof field === "string") : [];
  const only = (field: string) => changed.length === 1 && changed[0] === field;
  const word = (value: unknown) => (typeof value === "string" ? value.trim() : "");
  if (entry.type === "asset.updated") {
    const previous = word(details.previous_name);
    const group = word(details.group_name);
    if (only("name") && previous) return { key: `${key}_renamed`, values: { previous } };
    if (only("group")) return group ? { key: `${key}_moved`, values: { group } } : { key: `${key}_ungrouped` };
    if (only("tags")) return { key: `${key}_tags` };
  }
  if (entry.type === "incident.updated") {
    const assignee = word(details.assignee);
    if (changed.includes("status") && word(details.status) === "resolved") return { key: `${key}_resolved` };
    if (only("assignee")) return assignee ? { key: `${key}_assigned`, values: { assignee } } : { key: `${key}_unassigned` };
  }
  return { key };
}

/** The alert rows the Hub writes with the alert's devices in their details. */
const ALERT_DEVICE_TYPES = new Set(["alert.acknowledged", "alert.resolved", "alert.deleted"]);

/**
 * The devices an alert row is about: one name ("on nas01"), or how many with the names the row carries ("on 3
 * devices"; the Hub sends the first 20 names, "" for a device that is gone). null when the row names no device:
 * a rule that is not about a device, or one device that no longer exists.
 */
export function alertDevices(entry: Pick<AuditEntry, "type" | "details">): { name: string } | { count: number; names: string[] } | null {
  if (!ALERT_DEVICE_TYPES.has(entry.type)) return null;
  const details = entry.details ?? {};
  const name = typeof details.device_name === "string" ? details.device_name.trim() : "";
  if (name) return { name };
  const names = (Array.isArray(details.device_names) ? details.device_names : [])
    .filter((value): value is string => typeof value === "string" && value.trim() !== "")
    .map((value) => value.trim());
  const count = typeof details.device_count === "number" && Number.isFinite(details.device_count) ? Math.floor(details.device_count) : names.length;
  if (count === 1 && names.length === 1) return { name: names[0] };
  return count > 1 ? { count, names } : null;
}

/** A type without a sentence of its own, read as words: "backup.verify_job.started" → "Backup verify job started". */
export function humanizeType(type: string): string {
  const words = type.split(/[._\s-]+/).filter(Boolean).join(" ").toLowerCase();
  return words ? words[0].toUpperCase() + words.slice(1) : type;
}

/** The result word a row leads with: refused attempts read "Denied", failures "Failed"; others none. */
export function outcome(decision: string | undefined): "denied" | "failed" | "" {
  const value = (decision ?? "").trim().toLowerCase();
  if (value === "deny" || value === "denied" || value === "blocked" || value === "rejected") return "denied";
  if (value === "failed" || value === "error" || value === "dead_lettered") return "failed";
  return "";
}

const pad = (n: number) => String(n).padStart(2, "0");

/** "22:41" today, "Sat 10:04" in the last six days, "22 Oct" before that. */
export function auditTime(timestamp: string, now: Date, locale = "en"): string {
  const at = new Date(timestamp);
  if (Number.isNaN(at.getTime())) return timestamp;
  const hm = `${pad(at.getHours())}:${pad(at.getMinutes())}`;
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  if (at.getTime() >= today && at.getTime() < today + 86_400_000) return hm;
  if (at.getTime() >= today - 6 * 86_400_000 && at.getTime() < today) {
    return `${new Intl.DateTimeFormat(locale, { weekday: "short" }).format(at)} ${hm}`;
  }
  return new Intl.DateTimeFormat(locale, { day: "numeric", month: "short" }).format(at);
}

/** The people in the log, most active first (then A-Z). */
export function peopleByActivity(entries: AuditEntry[], nameOf: (id: string) => string): string[] {
  const count = new Map<string, number>();
  for (const e of entries) {
    const who = nameOf(e.actor_id ?? "");
    if (who) count.set(who, (count.get(who) ?? 0) + 1);
  }
  return [...count.keys()].sort((a, b) => (count.get(b) ?? 0) - (count.get(a) ?? 0) || a.localeCompare(b));
}

export type AuditSort = "time:desc" | "time:asc" | "who:asc" | "who:desc";

const ms = (e: AuditEntry) => {
  const v = new Date(e.timestamp).getTime();
  return Number.isNaN(v) ? 0 : v;
};

/**
 * The rows the filter shows. `textOf` gives the searchable words of an entry (time, person, action, target, result);
 * `who` and `type` narrow to one person or one kind of action ("all" for any).
 */
export function auditRows(
  entries: AuditEntry[],
  opts: { query: string; who: string; type: string; sort: AuditSort; nameOf: (id: string) => string; textOf: (e: AuditEntry) => string },
): AuditEntry[] {
  const q = opts.query.trim().toLowerCase();
  // "all" is every action but the API requests behind them; "requests" adds those.
  const typeHit = (e: AuditEntry) =>
    opts.type === ALL_REQUESTS || (opts.type === "all" ? e.type !== REQUEST_TYPE : e.type === opts.type);
  const hit = entries.filter(
    (e) =>
      (opts.who === "all" || opts.nameOf(e.actor_id ?? "") === opts.who) &&
      typeHit(e) &&
      (!q || opts.textOf(e).toLowerCase().includes(q)),
  );
  const [key, dir] = opts.sort.split(":") as ["time" | "who", "asc" | "desc"];
  const sign = dir === "asc" ? 1 : -1;
  return hit
    .map((e, i) => ({ e, i }))
    .sort((a, b) => {
      const primary = key === "who"
        ? opts.nameOf(a.e.actor_id ?? "").localeCompare(opts.nameOf(b.e.actor_id ?? ""))
        : ms(a.e) - ms(b.e);
      return sign * primary || ms(b.e) - ms(a.e) || a.i - b.i;
    })
    .map(({ e }) => e);
}

/** Details worth showing when a row opens: the reason first, then each field as text, then the IDs the row left out. */
export function detailRows(entry: AuditEntry, ids: Array<[string, string]> = []): Array<[string, string]> {
  const rows: Array<[string, string]> = [];
  if (entry.reason) rows.push(["reason", entry.reason]);
  for (const [k, v] of Object.entries(entry.details ?? {})) {
    if (v === null || v === undefined || v === "" || k === "target_name") continue;
    // A list of words (device names, changed fields, tags) reads as words, not as JSON.
    const words = Array.isArray(v) && v.every((item) => typeof item === "string" || typeof item === "number")
      ? v.filter((item) => item !== "").join(", ")
      : null;
    if (words === "") continue;
    rows.push([k, words ?? (typeof v === "string" ? v : JSON.stringify(v))]);
  }
  return [...rows, ...ids];
}
