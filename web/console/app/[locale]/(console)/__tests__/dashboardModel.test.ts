import { describe, expect, it } from "vitest";
import type { AlertInstance, Asset } from "../../../console/models";
import { attentionItems } from "../dashboardModel";
import type { DeviceCardData, Freshness } from "../nodes/nodesPageUtils";

const asset = (id: string, extra: Partial<Asset> = {}) =>
  ({ id, name: id, type: "host", source: "agent", status: "online", last_seen_at: "2026-01-01T00:00:00Z", ...extra }) as Asset;
const card = (id: string, freshness: Freshness): DeviceCardData => ({
  asset: asset(id), freshness, cpu: null, mem: null, disk: null,
  workloads: { vms: 0, containers: 0, stacks: 0, datastores: 0, other: 0 }, hostedOn: null, dockerHost: null, haHub: null,
});
const alert = (id: string, severity: AlertInstance["severity"], assetID?: string, started = "2026-01-01T10:00:00Z") =>
  ({ id, rule_id: `rule-${id}`, fingerprint: id, status: "firing", severity, labels: assetID ? { asset_id: assetID } : {},
    started_at: started, last_fired_at: started, created_at: started, updated_at: started }) as AlertInstance;

// Risk: Needs attention is the Dashboard's one promise. A device that stops answering must show there even without an
// alert rule, but not twice when an alert already names it; the most severe alert comes first.
describe("Dashboard Needs attention", () => {
  const base = { ruleNames: new Map<string, string>(), problemWorkloads: [], allAssets: [] };

  it("lists a silent device once, under its alert when one is open", () => {
    const items = attentionItems({ ...base, alerts: [alert("a1", "high", "nas")], cards: [card("nas", "offline"), card("pc", "unresponsive"), card("ok", "online")] });
    expect(items.map((i) => i.key)).toEqual(["alert:a1", "device:pc"]);
  });

  // The Hub names a metric or heartbeat alert's devices in firing_scope, not in an asset_id label.
  it("treats a device in an alert's firing scope as named by that alert", () => {
    const scoped = { ...alert("a2", "high"), firing_scope: { version: 1, asset_ids: ["nas"] } };
    const items = attentionItems({ ...base, alerts: [scoped], cards: [card("nas", "offline")] });
    expect(items.map((i) => i.key)).toEqual(["alert:a2"]);
  });

  // "Device offline" alone does not say which device: the row must name the devices the alert fired for.
  it("names an alert's devices under its title", () => {
    const scoped = (ids: string[]) => ({ ...alert("a3", "critical"), firing_scope: { version: 1, asset_ids: ids } });
    const allAssets = [asset("nas", { name: "nas01" }), asset("pve", { name: "pve01" })];
    const one = attentionItems({ ...base, allAssets, alerts: [scoped(["nas"])], cards: [] });
    expect(one[0].sub).toEqual({ key: "onHost", values: { host: "nas01" } });
    const many = attentionItems({ ...base, allAssets, alerts: [scoped(["nas", "pve", "x1", "x2"])], cards: [] });
    expect(many[0].sub).toEqual({ key: "onHost", values: { host: "nas01, pve01 +2" } });
    expect(attentionItems({ ...base, alerts: [alert("a4", "high")], cards: [] })[0].sub).toBeNull();
  });

  it("puts critical alerts before lower ones and gives medium no dot", () => {
    const items = attentionItems({ ...base, alerts: [alert("m", "medium"), alert("c", "critical")], cards: [] });
    expect(items.map((i) => [i.key, i.tone])).toEqual([["alert:c", "bad"], ["alert:m", ""]]);
  });

  it("is empty when everything answers and nothing fires", () => {
    expect(attentionItems({ ...base, alerts: [], cards: [card("ok", "online")] })).toEqual([]);
  });
});
