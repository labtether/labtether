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
  "settings.runtime.updated", "settings.runtime.reset", "settings.tls.updated", "settings.tls.cleared",
  "hub.tls.renewed", "settings.restart.requested", "admin.reset", "protocol.config.created",
  "protocol.config.updated", "protocol.config.deleted", "protocol.test.run", "protocol.ssh.hub_key_pushed",
  "api.exec", "mcp.tool", "api.bulk.file_push", "api.bulk.service_action", "api.request", "terminal.command.policy_checked",
  "actions.run.policy_checked", "api.exec.policy_checked", "queue.dead_letter", "asset.hub_ssh_key.updated",
  "settings.ssh_hub_key.rotation", "settings.managed_database.password_revealed",
]);

/** Every API change the Hub stores beside its named event; shown only under "All requests". */
export const REQUEST_TYPE = "api.request";

/** The type filter value that also shows API requests. */
export const ALL_REQUESTS = "requests";

/** The object's name the Hub stored with the row (details.target_name), else its ID. */
export function targetText(entry: Pick<AuditEntry, "target" | "details">): string {
  const name = entry.details?.target_name;
  return typeof name === "string" && name.trim() ? name.trim() : entry.target ?? "";
}

/** The message key for a known type ("terminal.session.created" → "terminal_session_created"), or "". */
export function actionKey(type: string): string {
  return KNOWN_ACTIONS.has(type) ? type.replace(/\./g, "_") : "";
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

/** Details worth showing when a row opens: the reason first, then each field as text. */
export function detailRows(entry: AuditEntry): Array<[string, string]> {
  const rows: Array<[string, string]> = [];
  if (entry.reason) rows.push(["reason", entry.reason]);
  for (const [k, v] of Object.entries(entry.details ?? {})) {
    if (v === null || v === undefined || v === "" || k === "target_name") continue;
    rows.push([k, typeof v === "string" ? v : JSON.stringify(v)]);
  }
  return rows;
}
