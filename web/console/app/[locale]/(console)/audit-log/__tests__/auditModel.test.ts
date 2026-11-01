import { describe, expect, it } from "vitest";
import en from "../../../../../messages/en/audit-log.json";
import { actionKey, auditRows, humanizeType, KNOWN_ACTIONS, outcome, peopleByActivity, type AuditEntry } from "../auditModel";

// Risks: a refused attempt does not read "Denied", the person chips miss or misorder people, a filter hides the
// entries it should keep, or an unknown event shows as a raw code.

const entry = (id: string, actor: string, type: string, minute: number, decision?: string): AuditEntry => ({
  id,
  actor_id: actor,
  type,
  decision,
  target: "pve-01",
  timestamp: new Date(Date.UTC(2026, 9, 27, 22, minute)).toISOString(),
});

const names: Record<string, string> = { "usr-1": "alex", "usr-2": "sam" };
const nameOf = (id: string) => names[id] ?? id;
const textOf = (e: AuditEntry) => `${nameOf(e.actor_id ?? "")} ${e.type} ${e.target ?? ""}`;

describe("auditModel", () => {
  it("reads refused attempts as Denied and failures as Failed", () => {
    expect(outcome("denied")).toBe("denied");
    expect(outcome("DENY")).toBe("denied");
    expect(outcome("dead_lettered")).toBe("failed");
    expect(outcome("allowed")).toBe("");
    expect(outcome(undefined)).toBe("");
  });

  it("gives known types a message key and reads unknown ones as words", () => {
    expect(actionKey("terminal.session.created")).toBe("terminal_session_created");
    expect(actionKey("backup.verify_job.started")).toBe("");
    expect(humanizeType("backup.verify_job.started")).toBe("Backup verify job started");
    const sentences = en.actions as Record<string, string>;
    expect([...KNOWN_ACTIONS].filter((type) => !sentences[actionKey(type)])).toEqual([]);
  });

  it("lists people most active first, then A-Z", () => {
    const list = [entry("1", "usr-2", "a", 1), entry("2", "usr-1", "a", 2), entry("3", "usr-1", "a", 3), entry("4", "system", "a", 4)];
    expect(peopleByActivity(list, nameOf)).toEqual(["alex", "sam", "system"]);
  });

  it("filters by person, type and words, newest first by default", () => {
    const list = [
      entry("1", "usr-1", "terminal.session.created", 1),
      entry("2", "usr-2", "terminal.session.created", 2, "denied"),
      entry("3", "usr-1", "webhook.created", 3),
    ];
    const base = { query: "", who: "all", type: "all", sort: "time:desc" as const, nameOf, textOf };
    expect(auditRows(list, base).map((e) => e.id)).toEqual(["3", "2", "1"]);
    expect(auditRows(list, { ...base, who: "alex" }).map((e) => e.id)).toEqual(["3", "1"]);
    expect(auditRows(list, { ...base, type: "terminal.session.created" }).map((e) => e.id)).toEqual(["2", "1"]);
    expect(auditRows(list, { ...base, query: "WEBHOOK" }).map((e) => e.id)).toEqual(["3"]);
    expect(auditRows(list, { ...base, sort: "who:asc" }).map((e) => e.id)).toEqual(["3", "1", "2"]);
  });
});
