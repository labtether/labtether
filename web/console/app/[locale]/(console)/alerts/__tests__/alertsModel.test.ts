import { describe, expect, it } from "vitest";
import type { AlertInstance, AlertSilence, Incident } from "../../../../console/models";
import { alertTimeline, deviceSilences, groupAlerts, pickAlert, silenceFor, silenceMatchers, silencesFor } from "../alertsModel";
import { DEFAULT_FILTER, type DeliveryRecord } from "../alertsPageTypes";
import { candidateIssues } from "../useAlertIssue";

const NOW = Date.parse("2026-06-01T12:00:00Z");
const at = (min: number) => new Date(NOW - min * 60_000).toISOString();
const alert = (id: string, extra: Partial<AlertInstance> = {}) =>
  ({ id, rule_id: `r-${id}`, fingerprint: id, status: "firing", severity: "medium", labels: {}, started_at: at(30),
    last_fired_at: at(30), created_at: at(30), updated_at: at(30), ...extra }) as AlertInstance;
const silence = (matchers: Record<string, string>, from: number, to: number) =>
  ({ id: "s", matchers, created_by: "owner", starts_at: at(from), ends_at: at(to), created_at: at(from) }) as AlertSilence;

// Risk: the inbox is the page's promise. The most severe open alert must come first and be the one shown beside the
// list; an alert a silence covers must leave the open list; resolved ones sit last, latest first.
describe("Alerts inbox", () => {
  it("orders open alerts most severe first, then newest, and shows the first", () => {
    const list = [alert("low", { severity: "low" }), alert("old-crit", { severity: "critical", started_at: at(90) }),
      alert("new-crit", { severity: "critical", started_at: at(5) }), alert("high", { severity: "high" })];
    const groups = groupAlerts(list, [], DEFAULT_FILTER, NOW);
    expect(groups.open.map((a) => a.id)).toEqual(["new-crit", "old-crit", "high", "low"]);
    expect(pickAlert(groups, null)?.id).toBe("new-crit");
    expect(pickAlert(groups, "high")?.id).toBe("high");
  });

  it("moves silenced alerts out of the open list and resolved ones to the end, latest first", () => {
    const list = [
      alert("quiet", { labels: { asset_id: "nas01", team: "lab" } }),
      alert("loud", { labels: { asset_id: "pve-01" } }),
      alert("done-early", { status: "resolved", resolved_at: at(50) }),
      alert("done-late", { status: "resolved", resolved_at: at(10) }),
    ];
    const groups = groupAlerts(list, [silence({ asset_id: "nas01" }, 10, -50)], DEFAULT_FILTER, NOW);
    expect(groups.open.map((a) => a.id)).toEqual(["loud"]);
    expect(groups.silenced.map((a) => a.id)).toEqual(["quiet"]);
    expect(groups.resolved.map((a) => a.id)).toEqual(["done-late", "done-early"]);
  });

  it("applies the severity and state filters", () => {
    const list = [alert("a", { severity: "critical" }), alert("b", { severity: "critical", status: "acknowledged" }), alert("c")];
    const groups = groupAlerts(list, [], { severity: "critical", state: "acknowledged", sort: "severity" }, NOW);
    expect(groups.open.map((a) => a.id)).toEqual(["b"]);
    expect([groups.shown, groups.total]).toEqual([1, 3]);
  });
});

// Risk: a silence must cover an alert only while it runs and only when every matcher equals one of the alert's labels,
// as the hub matches it; anything looser would show an alert as silenced while it still notifies.
describe("silenceFor", () => {
  const a = alert("x", { labels: { asset_id: "nas01", severity: "high" } });
  it("matches when every matcher equals a label and the silence is running", () => {
    expect(silenceFor(a, [silence({ asset_id: "nas01", severity: "high" }, 10, -10)], NOW)).not.toBeNull();
  });
  it("ignores ended, future, partial and empty silences", () => {
    expect(silenceFor(a, [silence({ asset_id: "nas01" }, 60, 30)], NOW)).toBeNull();
    expect(silenceFor(a, [silence({ asset_id: "nas01" }, -10, -60)], NOW)).toBeNull();
    expect(silenceFor(a, [silence({ asset_id: "nas01", team: "lab" }, 10, -10)], NOW)).toBeNull();
    expect(silenceFor(a, [silence({}, 10, -10)], NOW)).toBeNull();
  });
});

// Risk: Silence on an alert makes rule_id + asset_id silences (a rule without labels must work), and one device's
// silence must not show the alert as silenced while it still alerts for another device.
describe("silenceFor with rule and device silences", () => {
  const two = alert("y", { rule_id: "r-y", labels: {}, firing_scope: { version: 1, asset_ids: ["nas01", "nas02"] } });
  it("covers an alert with no labels when its rule is silenced", () => {
    expect(silenceFor(two, [silence({ rule_id: "r-y" }, 10, -10)], NOW)).not.toBeNull();
    expect(silenceFor(two, [silence({ rule_id: "r-other" }, 10, -10)], NOW)).toBeNull();
  });
  it("covers it only when every device it fired for is silenced", () => {
    const nas01 = silence({ rule_id: "r-y", asset_id: "nas01" }, 10, -10);
    expect(silenceFor(two, [nas01], NOW)).toBeNull();
    expect(silencesFor(two, [nas01, { ...silence({ rule_id: "r-y", asset_id: "nas02" }, 10, -10), id: "s2" }], NOW)).toHaveLength(2);
  });
});

// Risk: Silence on one device of an alert that fired for several must ask the Hub for that device alone (the others
// keep alerting), and End silence on a device must end only the silences that name it, not a neighbour's or the rule's.
describe("one device of a multi-device alert", () => {
  const two = alert("z", { rule_id: "r-z", labels: {}, firing_scope: { version: 1, asset_ids: ["nas01", "nas02"] } });
  it("asks for the named device only, and for every device when none is named", () => {
    expect(silenceMatchers(two, "nas02")).toEqual([{ rule_id: "r-z", asset_id: "nas02" }]);
    expect(silenceMatchers(two)).toEqual([{ rule_id: "r-z", asset_id: "nas01" }, { rule_id: "r-z", asset_id: "nas02" }]);
    expect(silenceMatchers(alert("bare", { rule_id: "r-bare" }))).toEqual([{ rule_id: "r-bare" }]);
  });
  it("finds only the running silences that name that device", () => {
    const list = [
      { ...silence({ rule_id: "r-z", asset_id: "nas01" }, 10, -10), id: "one" },
      { ...silence({ rule_id: "r-z", asset_id: "nas02" }, 10, -10), id: "two" },
      { ...silence({ rule_id: "r-z" }, 10, -10), id: "rule" },
      { ...silence({ rule_id: "r-other", asset_id: "nas01" }, 10, -10), id: "other-rule" },
      { ...silence({ rule_id: "r-z", asset_id: "nas01" }, 60, 30), id: "ended" },
    ];
    expect(deviceSilences(two, "nas01", list, NOW).map((s) => s.id)).toEqual(["one"]);
  });
});

// Risk: the timeline must only say what the hub recorded for this alert, in time order, including its deliveries.
describe("alertTimeline", () => {
  it("lists opened, fired again, deliveries and resolved in order, skipping other alerts' deliveries", () => {
    const a = alert("x", { severity: "critical", status: "resolved", started_at: at(30), last_fired_at: at(20), resolved_at: at(5) });
    const record = (id: string, alertId: string, status: DeliveryRecord["status"], min: number) =>
      ({ id, channel_id: "ch-1", alert_instance_id: alertId, route_id: "", status, sent_at: at(min), error: "", retry_count: 0,
        max_retries: 0, next_retry_at: null, created_at: at(min) }) as DeliveryRecord;
    const entries = alertTimeline(a, [record("d1", "x", "sent", 29), record("d2", "other", "sent", 28), record("d3", "x", "failed", 19)]);
    expect(entries.map((e) => [e.kind, e.tone])).toEqual([["opened", "bad"], ["sent", ""], ["fired", ""], ["failed", "bad"], ["resolved", ""]]);
  });
});

// Risk: the alert's issue is looked up on a few issues' timelines only; an issue still open (or resolved after the
// alert started) must stay a candidate, newest first, and one closed before the alert must not cost a request.
describe("candidateIssues", () => {
  it("keeps issues open while the alert ran, the latest first", () => {
    const issue = (id: string, created: number, resolved?: number) =>
      ({ id, title: id, severity: "high", status: resolved === undefined ? "open" : "resolved", source: "alert_auto",
        created_at: at(created), updated_at: at(created), resolved_at: resolved === undefined ? undefined : at(resolved) }) as Incident;
    const list = candidateIssues([issue("old-closed", 300, 200), issue("open", 60), issue("closed-after", 40, 5)], at(30));
    expect(list.map((i) => i.id)).toEqual(["closed-after", "open"]);
  });
});
