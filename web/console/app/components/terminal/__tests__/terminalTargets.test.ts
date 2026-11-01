// Guards the Terminal picker's promises: offline devices fold away instead of looking openable, containers and guests
// name their host, Recent never offers an offline device, the address form reads user@host:port, and tabs named by the
// workspace follow their device while a renamed tab keeps its name.
import { describe, expect, it } from "vitest";
import type { Asset } from "../../../console/models";
import { buildTargetLists, formatAddress, looksLikeAddress, parseAddress, tabLabels } from "../terminalTargets";

const now = new Date().toISOString();
const old = new Date(Date.now() - 60 * 60_000).toISOString();
const asset = (id: string, extra: Partial<Asset> = {}): Asset => ({
  id, name: id, type: "host", source: "agent", status: "online", last_seen_at: now, metadata: {}, ...extra,
});

const lab: Asset[] = [
  asset("pve-01", { type: "hypervisor-node", source: "proxmox" }),
  asset("nuc-01"),
  asset("docker-01", { type: "vm", source: "proxmox", metadata: { node: "pve-01", vmid: "110", os_pretty_name: "Debian 13" } }),
  asset("old-box", { last_seen_at: old }),
  asset("grafana", { type: "docker-container", source: "docker", metadata: { node: "docker-01" } }),
];

describe("buildTargetLists", () => {
  it("sorts devices by name, puts the current device first and folds offline ones away", () => {
    const lists = buildTargetLists({ assets: lab, connectedAgentIds: new Set(["nuc-01"]), firstId: "pve-01" });
    expect(lists.devices.map((t) => t.asset.id)).toEqual(["pve-01", "docker-01", "nuc-01"]);
    expect(lists.offline.map((t) => t.asset.id)).toEqual(["old-box"]);
    expect(lists.containers.map((t) => t.asset.id)).toEqual(["grafana"]);
  });

  it("says how each target is reached and where a guest runs", () => {
    const lists = buildTargetLists({ assets: lab, connectedAgentIds: new Set(["nuc-01"]) });
    const via = Object.fromEntries(lists.devices.map((t) => [t.asset.id, t.via]));
    expect(via).toEqual({ "docker-01": "proxmox", "nuc-01": "agent", "pve-01": "proxmox" });
    const guest = lists.devices.find((t) => t.asset.id === "docker-01");
    expect(guest).toMatchObject({ host: "pve-01", vmid: "110", os: "Debian 13" });
    expect(lists.containers[0].host).toBe("docker-01");
  });

  it("keeps Recent to three reachable targets in recent order, and hides it while filtering", () => {
    const recentIds = ["old-box", "nuc-01", "missing", "grafana", "pve-01", "docker-01"];
    const lists = buildTargetLists({ assets: lab, connectedAgentIds: new Set(), recentIds });
    expect(lists.recent.map((t) => t.asset.id)).toEqual(["nuc-01", "grafana", "pve-01"]);
    expect(buildTargetLists({ assets: lab, connectedAgentIds: new Set(), recentIds, query: "nuc" }).recent).toEqual([]);
  });

  it("matches every word of the query against name, OS and host", () => {
    const lists = buildTargetLists({ assets: lab, connectedAgentIds: new Set(), query: "debian pve" });
    expect(lists.devices.map((t) => t.asset.id)).toEqual(["docker-01"]);
  });
});

describe("addresses", () => {
  it("reads user, host and port, with 22 by default and brackets for IPv6", () => {
    expect(parseAddress("root@10.20.0.50")).toEqual({ user: "root", host: "10.20.0.50", port: 22 });
    expect(parseAddress("admin@nas.example.com:2222")).toEqual({ user: "admin", host: "nas.example.com", port: 2222 });
    expect(parseAddress("root@[2001:db8::1]:22")).toEqual({ user: "root", host: "2001:db8::1", port: 22 });
    expect(parseAddress("not an address")).toBeNull();
    expect(formatAddress({ user: "root", host: "2001:db8::1", port: 2222 })).toBe("root@[2001:db8::1]:2222");
  });

  it("treats a bare device name as a search, not an address", () => {
    expect(looksLikeAddress("pve-01")).toBe(false);
    expect(looksLikeAddress("root@pve-01")).toBe(true);
    expect(looksLikeAddress("10.20.0.50")).toBe(true);
    expect(looksLikeAddress("nas01:2222")).toBe(true);
  });
});

describe("tabLabels", () => {
  it("names workspace tabs after their device, numbers repeats and keeps renamed tabs", () => {
    const tabs = [
      { name: "Default", panes: [{ targetNodeId: "pve-01" }] },
      { name: "Tab 2", panes: [{ targetNodeId: "" }, { targetNodeId: "pve-01" }] },
      { name: "Ops", panes: [{ targetNodeId: "nuc-01" }] },
      { name: "Tab 4 Copy", panes: [] },
    ];
    expect(tabLabels(tabs, (id) => id.toUpperCase())).toEqual(["PVE-01", "PVE-01 (2)", "Ops", null]);
  });
});
