import { describe, expect, it } from "vitest";

import type { WebService } from "../../../../hooks/useWebServices";
import {
  filterServices,
  groupServices,
  NO_FILTERS,
  outagesFromChecks,
  serviceLine,
  sortServices,
} from "../servicesListModel";

function svc(id: string, patch: Partial<WebService> = {}): WebService {
  return {
    id,
    service_key: id,
    name: id,
    category: "Other",
    url: `https://${id}.example.com`,
    source: "docker",
    status: "up",
    response_ms: 20,
    host_asset_id: "nas01",
    icon_key: id,
    ...patch,
  };
}

const names = (list: WebService[]) => list.map((s) => s.name);
const hosts = new Map([["nas01", "nas01"], ["pve01", "pve01"]]);

// Risk: a hidden service leaking into the main list, or the Hidden chip showing visible ones.
describe("filterServices", () => {
  const list = [svc("grafana"), svc("jellyfin", { status: "down" }), svc("old", { metadata: { hidden: "true" } })];

  it("shows hidden services only under the Hidden chip", () => {
    expect(names(filterServices(list, NO_FILTERS, hosts))).toEqual(["grafana", "jellyfin"]);
    expect(names(filterServices(list, { ...NO_FILTERS, show: "hidden" }, hosts))).toEqual(["old"]);
    expect(names(filterServices(list, { ...NO_FILTERS, show: "down" }, hosts))).toEqual(["jellyfin"]);
  });

  it("matches every word of the text filter against name, device and address", () => {
    const pve = svc("proxmox", { host_asset_id: "pve01", url: "https://10.20.0.11:8006" });
    expect(names(filterServices([...list, pve], { ...NO_FILTERS, query: "pve01 8006" }, hosts))).toEqual(["proxmox"]);
  });
});

// Risk: a down service sinking out of sight, or the latency sort mixing down services with fast ones.
describe("sortServices", () => {
  const list = [svc("b", { response_ms: 300 }), svc("a", { response_ms: 10 }), svc("z", { status: "down", response_ms: 0 }), svc("c", { response_ms: 0 })];

  it("puts down services first when sorting by name, in either direction", () => {
    expect(names(sortServices(list, { key: "name", dir: "asc" }))).toEqual(["z", "a", "b", "c"]);
    expect(names(sortServices(list, { key: "name", dir: "desc" }))).toEqual(["z", "c", "b", "a"]);
  });

  it("sorts by latency with unmeasured and then down services last", () => {
    expect(names(sortServices(list, { key: "latency", dir: "asc" }))).toEqual(["a", "b", "c", "z"]);
  });
});

// Risk: groups in a random order, or an arranged order lost when grouped by category.
describe("groupServices", () => {
  it("orders categories by the console's category order and keeps services without a device last", () => {
    const list = [svc("x", { category: "Other" }), svc("m", { category: "Media" }), svc("s", { category: "Storage", host_asset_id: "" })];
    expect(groupServices(list, "category", hosts).map((g) => g.title)).toEqual(["Media", "Storage", "Other"]);
    expect(groupServices(list, "host", hosts).map((g) => g.title)).toEqual(["nas01", ""]);
  });

  it("follows an arranged order inside each category", () => {
    const list = [svc("a", { category: "Media" }), svc("b", { category: "Media" }), svc("c", { category: "Media" })];
    const groups = groupServices(list, "category", hosts, { Media: ["nas01::c", "nas01::a"] });
    expect(names(groups[0].services)).toEqual(["c", "a", "b"]);
  });
});

// Risk: the details panel reporting the wrong outages from the recent checks.
describe("outagesFromChecks", () => {
  it("finds each down period, newest first, and leaves a current one open", () => {
    const checks = ["up", "down", "down", "up", "unknown", "down"].map((status, i) => ({ at: `t${i}`, status }));
    expect(outagesFromChecks(checks)).toEqual([{ start: "t5" }, { start: "t1", end: "t3" }]);
  });
});

// Risk: the grey line repeating the device ("docker-01 · docker-01.lan") or dropping it for an IP or proxy domain.
describe("serviceLine", () => {
  it("names the device once and always shows the port", () => {
    expect(serviceLine("http://docker-01.lan:3000", "docker-01")).toBe("docker-01:3000");
    expect(serviceLine("https://10.20.0.1", "nuc-01")).toBe("nuc-01 · 10.20.0.1:443");
    expect(serviceLine("https://grafana.example.com", "")).toBe("grafana.example.com:443");
  });
});
