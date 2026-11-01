import { describe, expect, it } from "vitest";
import type { Asset } from "../../../../console/models";
import { buildSections } from "../deviceListModel";
import type { DeviceCardData, Freshness } from "../nodesPageUtils";

const card = (id: string, type: string, extra: Partial<DeviceCardData> = {}, freshness: Freshness = "online"): DeviceCardData => ({
  asset: { id, name: id, type, source: "proxmox", status: "online", last_seen_at: "2026-01-01T00:00:00Z" } as Asset,
  freshness,
  cpu: null,
  mem: null,
  disk: null,
  workloads: { vms: 0, containers: 0, stacks: 0, datastores: 0, other: 0 },
  hostedOn: null,
  dockerHost: null,
  haHub: null,
  ...extra,
});

const host = card("pve-01", "hypervisor-node");
const vm = card("docker-01", "vm", { hostedOn: { id: "pve-01", name: "pve-01" } });
const base = { cards: [host, vm], grouping: "type" as const, groups: [], query: "", problemsOnly: false };

// Risk: the Type view hides a VM behind its host. It must fold under the host when both show, and stand on its own
// when a filter leaves the host out, so a search for the VM never comes back empty.
describe("Devices list sections", () => {
  it("folds a VM under its host in the Type view", () => {
    const sections = buildSections(base);
    expect(sections.map((s) => s.key)).toEqual(["type:servers"]);
    expect(sections[0].rows[0].card.asset.id).toBe("pve-01");
    expect(sections[0].rows[0].guests.map((g) => g.asset.id)).toEqual(["docker-01"]);
  });

  it("shows a VM on its own when the filter leaves its host out", () => {
    const sections = buildSections({ ...base, query: "docker" });
    expect(sections.map((s) => s.key)).toEqual(["type:vms"]);
    expect(sections[0].rows.map((r) => r.card.asset.id)).toEqual(["docker-01"]);
  });

  it("puts devices that need attention first and can show only them", () => {
    const quiet = card("a-quiet", "host");
    const down = card("z-down", "host", {}, "offline");
    const all = buildSections({ ...base, cards: [quiet, down] });
    expect(all[0].rows.map((r) => r.card.asset.id)).toEqual(["z-down", "a-quiet"]);
    const problems = buildSections({ ...base, cards: [quiet, down], problemsOnly: true });
    expect(problems[0].rows.map((r) => r.card.asset.id)).toEqual(["z-down"]);
  });
});
