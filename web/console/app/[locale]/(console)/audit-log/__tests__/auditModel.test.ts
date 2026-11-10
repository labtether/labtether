import { describe, expect, it } from "vitest";
import en from "../../../../../messages/en/audit-log.json";
import { actionKey, ALL_REQUESTS, auditRows, detailRows, humanizeType, KNOWN_ACTIONS, outcome, peopleByActivity, targetText, visibleAuditRows, type AuditEntry } from "../auditModel";

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
  // Risk: rows of other people that are already in the page show to a person who is not an admin (second check;
  // the Hub is what limits the rows it sends).
  it("shows the whole log to the owner and admins, and only their own rows to everyone else", () => {
    const log = [entry("a", "usr-1", "terminal.session.created", 1), entry("b", "usr-2", "api_key.created", 2), entry("c", "usr-1", "asset.power", 3)];
    expect(visibleAuditRows(log, true, "usr-2").map((e) => e.id)).toEqual(["a", "b", "c"]);
    expect(visibleAuditRows(log, false, "usr-1").map((e) => e.id)).toEqual(["a", "c"]);
    expect(visibleAuditRows(log, false, undefined)).toEqual([]);
  });

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

  // Risk: the log fills with "API request POST /…" rows that repeat each change, or shows IDs where the Hub has a name.
  it("keeps API requests out until All requests is picked", () => {
    const list = [entry("1", "usr-1", "webhook.deleted", 1), entry("2", "usr-1", "api.request", 1)];
    const base = { query: "", who: "all", type: "all", sort: "time:desc" as const, nameOf, textOf };
    expect(auditRows(list, base).map((e) => e.id)).toEqual(["1"]);
    expect(auditRows(list, { ...base, type: ALL_REQUESTS }).map((e) => e.id).sort()).toEqual(["1", "2"]);
    expect(auditRows(list, { ...base, type: "api.request" }).map((e) => e.id)).toEqual(["2"]);
  });

  it("shows the name the Hub stored, else the ID, and does not repeat the name in the details", () => {
    const named = { ...entry("1", "usr-1", "webhook.deleted", 1), target: "wh_1793", details: { target_name: "ltqa-hook", url: "https://hooks.example.com" } };
    expect(targetText(named)).toBe("ltqa-hook");
    expect(targetText({ ...named, details: { target_name: "  " } })).toBe("wh_1793");
    expect(targetText({ target: "wh_1793" })).toBe("wh_1793");
    expect(detailRows(named)).toEqual([["url", "https://hooks.example.com"]]);
  });
});
