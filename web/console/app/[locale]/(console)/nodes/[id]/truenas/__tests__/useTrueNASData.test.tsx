import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useTrueNASList, type TrueNASListEndpoint } from "../useTrueNASData";
import { TrueNASServicesTab } from "../TrueNASServicesTab";
import { TrueNASDatasetsTab } from "../TrueNASDatasetsTab";
import { TrueNASVMsTab } from "../TrueNASVMsTab";
import { TrueNASReplicationTab } from "../TrueNASReplicationTab";
import { TrueNASPoolsTab } from "../TrueNASPoolsTab";
import { TrueNASSnapshotsTab } from "../TrueNASSnapshotsTab";
import { TrueNASDisksTab } from "../TrueNASDisksTab";
import { TrueNASSharesTab } from "../TrueNASSharesTab";
import { TrueNASOverviewTab } from "../TrueNASOverviewTab";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
let fetchMock: ReturnType<typeof vi.fn>;

function Consumer({ endpoint }: { endpoint: TrueNASListEndpoint }) {
  const { data, loading, error } = useTrueNASList<{ name?: string; path?: string; id?: string | number }>("nas/fixture", endpoint);
  return <>
    <output>{loading ? "loading" : error ?? "ready"}</output>
    <ul>{data.filter(Boolean).map((item, i) => <li key={i}>{item.name ?? item.path ?? String(item.id)}</li>)}</ul>
  </>;
}

async function render(endpoint: TrueNASListEndpoint, payload: unknown, status = 200) {
  fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(payload), { status }));
  await act(async () => { root.render(<Consumer key={endpoint} endpoint={endpoint} />); });
}

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const validRows: Record<TrueNASListEndpoint, unknown> = {
  pools: { id: 1, name: "expected", status: "ONLINE" },
  datasets: { id: "tank/expected", name: "expected" },
  snapshots: { id: "tank/data@expected", name: "expected", dataset: "tank/data" },
  replication: { id: 7, name: "expected", enabled: true },
  vms: { id: 42, name: "expected", status: { state: "STOPPED" } },
  services: { service: "expected", state: "running", enabled: true },
  disks: { name: "expected" },
  "shares/smb": { id: 7, name: "expected", path: "/mnt/tank/shared", enabled: true },
  "shares/nfs": { id: 8, path: "expected", enabled: true },
};

describe("TrueNAS list response contract", () => {
  it.each([
    ["pools", "pools"], ["datasets", "datasets"], ["snapshots", "snapshots"],
    ["replication", "replications"], ["vms", "vms"], ["services", "services"],
    ["disks", "disks"], ["shares/smb", "smb"], ["shares/nfs", "nfs"],
  ] as const)("renders the exact %s collection from its Hub envelope", async (endpoint, key) => {
    await render(endpoint, {
      asset_id: "nas/fixture", fetched_at: "2026-10-11T00:00:00Z",
      [key]: [validRows[endpoint]],
      unrelated: [{ id: "wrong collection" }],
    });
    expect(container.querySelector("output")?.textContent).toBe("ready");
    expect(container.querySelector("ul")?.textContent).toBe("expected");
    expect(fetchMock).toHaveBeenCalledWith(`/api/truenas/assets/nas%2Ffixture/${endpoint}`, { cache: "no-store" });
  });

  it.each([
    [[{ id: "raw", name: "raw" }], "raw"],
    [{ data: [{ id: "wrapped", name: "wrapped" }] }, "wrapped"],
    [{ data: { datasets: [{ id: "named wrapper", name: "named wrapper" }] } }, "named wrapper"],
    [{ datasets: [] }, ""],
  ])("preserves supported array response forms %#", async (payload, expected) => {
    await render("datasets", payload);
    expect(container.querySelector("output")?.textContent).toBe("ready");
    expect(container.querySelector("ul")?.textContent).toBe(expected);
  });

  it.each([
    { pools: [] }, { datasets: {} }, { datasets: null, warnings: ["query unavailable"] },
    { datasets: "wrong" }, null, "wrong", 42,
  ])("reports a malformed successful list instead of rendering an object %#", async (payload) => {
    await render("datasets", payload);
    expect(container.querySelector("output")?.textContent).toBe("invalid datasets response: expected datasets list");
    expect(container.querySelector("ul")?.textContent).toBe("");
  });

  it("preserves an HTTP failure instead of decoding its collection", async () => {
    await render("datasets", { error: "asset forbidden", datasets: [{ id: "must not render" }] }, 403);
    expect(container.querySelector("output")?.textContent).toBe("asset forbidden");
    expect(container.querySelector("ul")?.textContent).toBe("");
  });
});


describe("TrueNAS raw row consumers", () => {
  it("renders real service, ZFS property, VM status and replication schedule rows in their tabs", async () => {
    const bodies: Record<string, unknown> = {
      services: { services: [
        { id: "1", service: "nfs", state: "running", enabled: true },
        { id: "2", service: "ssh", state: "stopped", enabled: false },
        { id: "3", service: "fixture", state: "STARTING", enabled: true },
      ] },
      datasets: { datasets: [{ id: "tank/data", name: "tank/data", used: { parsed: 1024 },
        available: { rawvalue: "2048" }, compression: { value: "lz4" }, mountpoint: { value: "/mnt/tank/data" },
        readonly: { value: "off" } }] },
      vms: { vms: [{ id: 42, name: "builder", status: { state: "RUNNING", pid: 123 }, vcpus: 2, memory: 512 },
        { id: 43, name: "uncertain", status: { state: "UNKNOWN" } }] },
      replication: { replications: [{ id: 9, name: "nightly", enabled: true,
        source_datasets: ["tank/data"], target_dataset: "backup/data",
        schedule: { minute: "0", hour: "2", dom: "*", month: "*", dow: "*" },
        state: { state: "FINISHED", datetime: "2026-10-11T02:00:00Z" } }] },
    };
    fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
      if (url === "/api/truenas/assets/nas/vms/42/stop" && init?.method === "POST") {
        return new Response("{}");
      }
      const body = bodies[url.split("/").pop() ?? ""];
      if (!body) throw new Error("unexpected fixture URL");
      return new Response(JSON.stringify(body));
    });
    await act(async () => { root.render(<>
      <section data-kind="services"><TrueNASServicesTab assetId="nas" /></section>
      <section data-kind="datasets"><TrueNASDatasetsTab assetId="nas" /></section>
      <section data-kind="vms"><TrueNASVMsTab assetId="nas" /></section>
      <section data-kind="replication"><TrueNASReplicationTab assetId="nas" /></section>
    </>); });
    const cells = (kind: string) => [...container.querySelectorAll(`[data-kind="${kind}"] tbody td`)]
      .map((cell) => cell.textContent);
    expect(cells("services")).toEqual(expect.arrayContaining(["nfs", "Running", "Stopped", "Yes", "No"]));
    expect(cells("datasets")).toEqual(expect.arrayContaining(["data", "1.0 KB", "2.0 KB", "lz4", "/mnt/tank/data"]));
    expect(cells("datasets").join(" ")).not.toMatch(/NaN|\[object Object\]/);
    expect(cells("vms")).toEqual(expect.arrayContaining(["builder", "512 MiB"]));
    expect(cells("vms").join(" ")).toContain("RUNNING");
    expect(cells("replication")).toEqual(expect.arrayContaining(["nightly", "0 2 * * *", "tank/data", "backup/data"]));
    expect(cells("replication").join(" ")).toContain("FINISHED");
    const unknownService = [...container.querySelectorAll('[data-kind="services"] tbody tr')]
      .find((row) => row.querySelector("td")?.textContent === "fixture")!;
    expect(unknownService.textContent).toContain("STARTING");
    expect(unknownService.textContent).not.toContain("Stopped");
    const stateButtons = [...unknownService.querySelectorAll("button")]
      .filter((button) => ["Start", "Stop"].includes(button.textContent ?? ""));
    expect(stateButtons).toHaveLength(2);
    expect(stateButtons.every((button) => button.disabled)).toBe(true);
    const unknownVM = [...container.querySelectorAll('[data-kind="vms"] tbody tr')]
      .find((row) => row.querySelector("td")?.textContent === "uncertain")!;
    expect(unknownVM.textContent).toContain("UNKNOWN");
    const vmStateButtons = [...unknownVM.querySelectorAll("button")]
      .filter((button) => ["Start", "Stop"].includes(button.textContent ?? ""));
    expect(vmStateButtons).toHaveLength(2);
    expect(vmStateButtons.every((button) => button.disabled)).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(4);
    const stop = [...container.querySelectorAll<HTMLButtonElement>('[data-kind="vms"] button')]
      .find((button) => button.textContent === "Stop")!;
    expect(stop.disabled).toBe(false);
    await act(async () => { stop.click(); });
    expect(fetchMock).toHaveBeenCalledWith("/api/truenas/assets/nas/vms/42/stop", expect.objectContaining({ method: "POST" }));
    expect(fetchMock).toHaveBeenCalledTimes(6);
  });

  it("renders storage topology, snapshot properties, disk temperatures and both shares without changing action IDs", async () => {
    const now = Date.parse("2026-10-11T03:00:00Z");
    vi.spyOn(Date, "now").mockReturnValue(now);
    const bodies: Record<string, unknown> = {
      pools: { pools: [{ id: 1, name: "tank", status: "ONLINE", size: "4096", allocated: 1024, free: 3072,
        topology: { data: [{ name: "mirror-0", type: "MIRROR", status: "ONLINE",
          children: [{ path: "/dev/sda", type: "DISK", status: "ONLINE" }] }] },
        scan: { function: "SCRUB", state: "FINISHED", end_time: { $date: now - 3600000 } } }] },
      snapshots: { snapshots: [{ id: "tank/data@keep", name: "keep",
        properties: { creation: { rawvalue: String((now - 3600000) / 1000) },
          used: { parsed: 1024 }, referenced: { rawvalue: "2048" } } }] },
      disks: { disks: [{ name: "sda", model: "Fixture SSD", serial: "fixture-serial", size: "8192" }],
        temperatures: { sda: 42.5 } },
      smart: { summary: { total: 0, healthy: 0, warning: 0, critical: 0, unknown: 0 }, disks: [] },
      "shares/smb": { smb: [{ id: 17, name: "shared", path: "/mnt/tank/smb", enabled: true, comment: "Fixture share" }] },
      "shares/nfs": { nfs: [{ id: 23, path: "/mnt/tank/nfs", enabled: false,
        networks: ["192.0.2.0/24"], hosts: ["fixture.invalid"] }] },
    };
    fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
      if (init?.method === "POST" && url.endsWith("/snapshots/tank%2Fdata%40keep/rollback")) return new Response("{}");
      if (init?.method === "PUT" && ["/shares/smb/17", "/shares/nfs/23"].some((path) => url.endsWith(path))) return new Response("{}");
      const body = bodies[url.replace("/api/truenas/assets/nas/", "")];
      if (!body) throw new Error("unexpected storage fixture URL");
      return new Response(JSON.stringify(body));
    });
    await act(async () => { root.render(<>
      <section data-kind="pools"><TrueNASPoolsTab assetId="nas" /></section>
      <section data-kind="snapshots"><TrueNASSnapshotsTab assetId="nas" /></section>
      <section data-kind="disks"><TrueNASDisksTab assetId="nas" /></section>
      <section data-kind="shares"><TrueNASSharesTab assetId="nas" /></section>
    </>); });
    const text = (kind: string) => container.querySelector(`[data-kind="${kind}"]`)!.textContent;
    const click = async (kind: string, label: string) => {
      const button = [...container.querySelectorAll<HTMLButtonElement>(`[data-kind="${kind}"] button`)]
        .find((item) => item.textContent === label)!;
      expect(button.disabled).toBe(false);
      await act(async () => { button.click(); });
    };
    expect(text("pools")).toContain("4.0 KB");
    expect(text("pools")).toContain("1h ago");
    await click("pools", "▸");
    expect(text("pools")).toContain("mirror-0");
    expect(text("pools")).toContain("/dev/sda");
    expect(text("snapshots")).toContain("tank/data");
    expect(text("snapshots")).toContain("1h ago");
    expect(text("snapshots")).toContain("1.0 KB");
    expect(text("snapshots")).toContain("2.0 KB");
    expect(text("disks")).toContain("Fixture SSD");
    expect(text("disks")).toContain("8.0 KB");
    expect(text("disks")).toContain("42.5°C");
    expect(text("shares")).toContain("/mnt/tank/smb");
    expect(text("shares")).toContain("/mnt/tank/nfs");
    expect(text("shares")).toContain("192.0.2.0/24");
    expect(text("shares")).toContain("fixture.invalid");
    expect(container.textContent).not.toMatch(/NaN|\[object Object\]/);
    expect(fetchMock).toHaveBeenCalledTimes(6);
    await click("snapshots", "Rollback");
    expect(fetchMock).toHaveBeenCalledWith("/api/truenas/assets/nas/snapshots/tank%2Fdata%40keep/rollback", expect.objectContaining({ method: "POST" }));
    await click("shares", "Disable");
    expect(fetchMock).toHaveBeenCalledWith("/api/truenas/assets/nas/shares/smb/17", expect.objectContaining({ method: "PUT", body: '{"enabled":false}' }));
    await click("shares", "Enable");
    expect(fetchMock).toHaveBeenCalledWith("/api/truenas/assets/nas/shares/nfs/23", expect.objectContaining({ method: "PUT", body: '{"enabled":true}' }));
    expect(fetchMock).toHaveBeenCalledTimes(12);
  });

  it("shows nested system, pool totals and mapped services and alerts in the actual overview", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      hostname: "audit-nas", system_info: { version: "TrueNAS-SCALE-25.10", model: "Fixture CPU",
        uptime: "1 day", cores: 8, physmem: 1073741824, ecc_memory: true },
      pools: [{ size: 4096, allocated: 1024 }, { size: 4096, allocated: 1024 }],
      services: [{ service: "nfs", state: "running", enabled: true },
        { service: "fixture", state: "STARTING", enabled: true }],
      alerts: [{ level: "WARNING", formatted: "Fixture storage warning", dismissed: false }],
    })));
    await act(async () => { root.render(<TrueNASOverviewTab assetId="nas" />); });
    expect(container.textContent).toContain("TrueNAS-SCALE-25.10");
    expect(container.textContent).toContain("Fixture CPU");
    expect(container.textContent).toContain("1 day");
    expect(container.textContent).toContain("25%");
    expect(container.textContent).toContain("1.0 GB");
    expect(container.textContent).toContain("2.0 KB used");
    expect(container.textContent).toContain("8.0 KB total");
    expect(container.textContent).toContain("1 running");
    expect(container.textContent).toContain("0 stopped");
    expect(container.textContent).toContain("1 other states");
    expect(container.textContent).not.toContain("Auto-start services that are stopped");
    expect(container.textContent).toContain("Fixture storage warning");
    expect(container.textContent).not.toMatch(/NaN|\[object Object\]/);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
