import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import truenasMessages from "../../../../../../../messages/en/devices-truenas.json";
import proxmoxMessages from "../../../../../../../messages/en/devices-proxmox.json";
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

vi.mock("../../../../../../../i18n/navigation", () => ({
  Link: ({ children, href }: { children: ReactNode; href: string }) => <a href={href}>{children}</a>,
}));

// Row menus and confirm dialogs render through Radix portals; here the menu items are plain buttons and a confirm
// request is accepted at once, so the tests reach the same actions a person reaches through ⋯ and the dialog.
vi.mock("../../proxmox/DiControls", async (original) => ({
  ...await original<typeof import("../../proxmox/DiControls")>(),
  DiRowMenu: ({ items }: { items: Array<{ label?: string; disabled?: boolean; onSelect?: () => void; separator?: true }> }) => (
    <span data-menu="">
      {items.filter((item) => !item.separator).map((item) => (
        <button key={item.label} type="button" disabled={item.disabled} onClick={item.onSelect}>{item.label}</button>
      ))}
    </span>
  ),
  useDiConfirm: () => ({ ask: (request: { onConfirm: () => unknown }) => { void request.onConfirm(); }, dialog: null }),
}));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const intl = (node: ReactNode) => (
  <NextIntlClientProvider locale="en" timeZone="UTC" messages={{ "devices-truenas": truenasMessages, "devices-proxmox": proxmoxMessages }}>
    {node}
  </NextIntlClientProvider>
);

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



/** The text of the list row whose name is exactly `name`, inside the section of `kind`. */
function row(kind: string, name: string): HTMLElement {
  const found = [...container.querySelectorAll<HTMLElement>(`[data-kind="${kind}"] [role="listitem"]`)]
    .find((item) => item.querySelector(".di-nm")?.textContent === name);
  if (!found) throw new Error(`no ${kind} row named ${name}`);
  return found;
}

function menuButton(scope: HTMLElement, label: string): HTMLButtonElement {
  const button = [...scope.querySelectorAll<HTMLButtonElement>("[data-menu] button")].find((item) => item.textContent === label);
  if (!button) throw new Error(`no menu item ${label}`);
  return button;
}

describe("TrueNAS raw row consumers", () => {
  it("renders real service, ZFS property, VM status and replication schedule rows in their views", async () => {
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
    await act(async () => { root.render(intl(<>
      <section data-kind="services"><TrueNASServicesTab assetId="nas" /></section>
      <section data-kind="datasets"><TrueNASDatasetsTab assetId="nas" /></section>
      <section data-kind="vms"><TrueNASVMsTab assetId="nas" /></section>
      <section data-kind="replication"><TrueNASReplicationTab assetId="nas" /></section>
    </>)); });
    expect(row("services", "nfs").textContent).toContain("Running");
    expect(row("services", "ssh").textContent).toContain("Stopped");
    const dataset = row("datasets", "tank/data").textContent ?? "";
    for (const part of ["1 KB", "2 KB", "lz4", "/mnt/tank/data"]) expect(dataset).toContain(part);
    expect(container.textContent).not.toMatch(/NaN|\[object Object\]/);
    expect(row("vms", "builder").textContent).toContain("512 MB");
    expect(row("vms", "builder").textContent).toContain("Running");
    const nightly = row("replication", "nightly").textContent ?? "";
    for (const part of ["0 2 * * *", "tank/data to backup/data", "Finished"]) expect(nightly).toContain(part);

    // A state TrueNAS reports that is neither running nor stopped shows as it came, and Start and Stop wait.
    const unknownService = row("services", "fixture");
    expect(unknownService.textContent).toContain("STARTING");
    expect(unknownService.textContent).not.toContain("Stopped");
    expect(menuButton(unknownService, "Start").disabled).toBe(true);
    expect(menuButton(unknownService, "Stop…").disabled).toBe(true);
    const unknownVM = row("vms", "uncertain");
    expect(unknownVM.textContent).toContain("UNKNOWN");
    expect(menuButton(unknownVM, "Start").disabled).toBe(true);
    expect(menuButton(unknownVM, "Stop…").disabled).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(4);

    const stop = menuButton(row("vms", "builder"), "Stop…");
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
    await act(async () => { root.render(intl(<>
      <section data-kind="pools"><TrueNASPoolsTab assetId="nas" /></section>
      <section data-kind="snapshots"><TrueNASSnapshotsTab assetId="nas" /></section>
      <section data-kind="disks"><TrueNASDisksTab assetId="nas" /></section>
      <section data-kind="shares"><TrueNASSharesTab assetId="nas" /></section>
    </>)); });
    const text = (kind: string) => container.querySelector(`[data-kind="${kind}"]`)!.textContent;
    const pool = row("pools", "tank");
    for (const part of ["Mirror", "1 disk", "Last scrub", "25%", "1 of 4 KB"]) expect(pool.textContent).toContain(part);
    await act(async () => { pool.querySelector<HTMLElement>(".di-row")!.click(); });
    expect(text("pools")).toContain("mirror-0");
    expect(text("pools")).toContain("/dev/sda");
    const snapshot = row("snapshots", "keep").textContent ?? "";
    for (const part of ["tank/data", "1 KB", "2 KB"]) expect(snapshot).toContain(part);
    const disk = row("disks", "sda").textContent ?? "";
    for (const part of ["Fixture SSD", "8 KB", "42.5"]) expect(disk).toContain(part);
    for (const part of ["/mnt/tank/smb", "/mnt/tank/nfs", "192.0.2.0/24", "fixture.invalid"]) expect(text("shares")).toContain(part);
    expect(container.textContent).not.toMatch(/NaN|\[object Object\]/);
    expect(fetchMock).toHaveBeenCalledTimes(6);

    await act(async () => { menuButton(row("snapshots", "keep"), "Roll back…").click(); });
    expect(fetchMock).toHaveBeenCalledWith("/api/truenas/assets/nas/snapshots/tank%2Fdata%40keep/rollback", expect.objectContaining({ method: "POST" }));
    const share = (label: string) => container.querySelector<HTMLButtonElement>(`[role="switch"][aria-label="${label}"]`)!;
    expect(share("SMB share /mnt/tank/smb").getAttribute("aria-checked")).toBe("true");
    await act(async () => { share("SMB share /mnt/tank/smb").click(); });
    expect(fetchMock).toHaveBeenCalledWith("/api/truenas/assets/nas/shares/smb/17", expect.objectContaining({ method: "PUT", body: '{"enabled":false}' }));
    await act(async () => { share("NFS share /mnt/tank/nfs").click(); });
    expect(fetchMock).toHaveBeenCalledWith("/api/truenas/assets/nas/shares/nfs/23", expect.objectContaining({ method: "PUT", body: '{"enabled":true}' }));
    expect(fetchMock).toHaveBeenCalledTimes(12);
  });

  it("shows nested system details, pool totals, mapped services and alerts in the system view", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      hostname: "audit-nas", system_info: { version: "TrueNAS-SCALE-25.10", model: "Fixture CPU",
        uptime: "1 day", cores: 8, physmem: 1073741824, ecc_memory: true },
      pools: [{ size: 4096, allocated: 1024 }, { size: 4096, allocated: 1024 }],
      services: [{ service: "nfs", state: "running", enabled: true },
        { service: "fixture", state: "STARTING", enabled: true }],
      alerts: [{ level: "WARNING", formatted: "Fixture storage warning", dismissed: false }],
    })));
    await act(async () => { root.render(intl(<TrueNASOverviewTab assetId="nas" />)); });
    for (const part of ["TrueNAS-SCALE-25.10", "Fixture CPU", "1 day", "1 GB", "2 of 8 KB · 25%", "1 of 2 running", "Fixture storage warning"]) {
      expect(container.textContent).toContain(part);
    }
    expect(container.textContent).not.toContain("Stopped, set to start on boot");
    expect(container.textContent).not.toMatch(/NaN|\[object Object\]/);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
