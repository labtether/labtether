import { describe, expect, it } from "vitest";
import en from "../../../../../messages/en/audit-log.json";
import { actionKey, actionSentence, alertDevices, ALL_REQUESTS, auditRows, detailRows, humanizeType, KNOWN_ACTIONS, outcome, peopleByActivity, targetText, visibleAuditRows, type AuditEntry } from "../auditModel";

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

  // Risk: a rename or a resolved issue reads only "Changed a device" / "Changed an issue", or a detail-specific
  // sentence has no text.
  it("picks a closer sentence when the row's details say what changed", () => {
    const sentences = en.actions as Record<string, string>;
    const of = (type: string, details: Record<string, unknown>) => actionSentence({ type, details });
    expect(of("asset.updated", { changed: ["name"], previous_name: "nas01" })).toEqual({ key: "asset_updated_renamed", values: { previous: "nas01" } });
    expect(of("asset.updated", { changed: ["group"], group_name: "Rack 2" })).toEqual({ key: "asset_updated_moved", values: { group: "Rack 2" } });
    expect(of("asset.updated", { changed: ["group"], group_name: "" })).toEqual({ key: "asset_updated_ungrouped" });
    expect(of("asset.updated", { changed: ["tags"], tags: ["backup"] })).toEqual({ key: "asset_updated_tags" });
    expect(of("asset.updated", { changed: ["name", "tags"], previous_name: "nas01" })).toEqual({ key: "asset_updated" });
    expect(of("incident.updated", { changed: ["status"], status: "resolved" })).toEqual({ key: "incident_updated_resolved" });
    expect(of("incident.updated", { changed: ["assignee"], assignee: "sam" })).toEqual({ key: "incident_updated_assigned", values: { assignee: "sam" } });
    expect(of("incident.updated", { changed: ["assignee"], assignee: "" })).toEqual({ key: "incident_updated_unassigned" });
    expect(of("incident.updated", { changed: ["summary"], status: "open" })).toEqual({ key: "incident_updated" });
    expect(actionSentence({ type: "backup.verify_job.started" })).toEqual({ key: "" });
    for (const key of ["asset_updated_renamed", "asset_updated_moved", "asset_updated_ungrouped", "asset_updated_tags", "incident_updated_resolved", "incident_updated_assigned", "incident_updated_unassigned"]) {
      expect(sentences[key], key).toBeTruthy();
    }
  });

  // Risk: an alert row does not say which device it was about, names a device for a rule that has none, or shows
  // the names of several devices as raw JSON in the row's details.
  it("says which device an alert row is about: one name, or a count with the names", () => {
    const row = (details: Record<string, unknown>, type = "alert.acknowledged") => ({ type, details });
    expect(alertDevices(row({ target_name: "Disk above 85%", device_name: "nas01", device_id: "dev-1" }))).toEqual({ name: "nas01" });
    expect(alertDevices(row({ device_count: 3, device_ids: ["a", "b", "c"], device_names: ["nas01", "", "pve01"] }, "alert.resolved")))
      .toEqual({ count: 3, names: ["nas01", "pve01"] });
    expect(alertDevices(row({ device_count: 25, device_names: ["nas01", "nas02"] }, "alert.deleted"))).toEqual({ count: 25, names: ["nas01", "nas02"] });
    // No device: a rule that is not about one, or a device that is gone (only its ID is left).
    expect(alertDevices(row({ target_name: "Hub backup late" }))).toBeNull();
    expect(alertDevices(row({ device_id: "dev-gone" }))).toBeNull();
    // Only the three alert rows carry these fields.
    expect(alertDevices(row({ device_name: "nas01" }, "asset.updated"))).toBeNull();

    const where = en.where as Record<string, string>;
    expect(where.device).toContain("{name}");
    expect(where.devices).toContain("{count");

    const details = Object.fromEntries(detailRows({ ...entry("x", "usr-1", "alert.resolved", 5), details: { device_count: 3, device_names: ["nas01", "", "pve01"], severity: "high" } }));
    expect(details.device_names).toBe("nas01, pve01");
    expect(details.severity).toBe("high");
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
