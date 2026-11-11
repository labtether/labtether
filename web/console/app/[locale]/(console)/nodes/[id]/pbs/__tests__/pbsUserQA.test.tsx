import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import commonMessages from "../../../../../../../messages/en/common.json";
import messages from "../../../../../../../messages/en/devices-proxmox.json";
import { normalizePBSAssetDetailsResponse } from "../../pbsTabModel";
import { PBSCoverageList } from "../PBSCoverageList";
import { PBSDatastoreList } from "../PBSDatastoreList";
import { PBSSnapshotsView } from "../PBSSnapshotsView";
import { PBSTasksView } from "../PBSTasksView";
import { usePBSDetails } from "../usePBSData";

vi.mock("../../../../../../../i18n/navigation", () => ({
  Link: ({ children, href, className }: { children: ReactNode; href: string; className?: string }) => (
    <a href={href} className={className}>{children}</a>
  ),
}));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

const detailsPayload = {
  asset_id: "pbs-server-qa",
  kind: "server",
  collector_id: "collector-qa",
  node: "localhost",
  datastores: [
    { store: "qa-store", status: "healthy", total_bytes: 100, used_bytes: 25 },
    { store: "archive-store", status: "healthy", total_bytes: 200, used_bytes: 50 },
  ],
  tasks: [
    {
      upid: "UPID:qa:verify",
      node: "localhost",
      worker_type: "verificationjob",
      worker_id: "weekly-integrity",
      status: "OK",
    },
  ],
};
const details = normalizePBSAssetDetailsResponse(detailsPayload);
const sortedStores = [...(details.datastores ?? [])].sort((a, b) => a.store.localeCompare(b.store));

let container: HTMLDivElement;
let root: Root;

function respond(payload: unknown, ok = true, status = 200): Response {
  return { ok, status, json: async () => payload } as Response;
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  if (!("PointerEvent" in window)) vi.stubGlobal("PointerEvent", MouseEvent);
  if (!("ResizeObserver" in window)) {
    vi.stubGlobal("ResizeObserver", class { observe() {} unobserve() {} disconnect() {} });
  }
  if (!("hasPointerCapture" in HTMLElement.prototype)) {
    Object.defineProperty(HTMLElement.prototype, "hasPointerCapture", { configurable: true, value: () => false });
  }
  if (!("scrollIntoView" in HTMLElement.prototype)) {
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", { configurable: true, value: () => undefined });
  }
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      return respond(url.includes("/groups")
        ? { datastores: [{ store: "qa-store", groups: [{ backup_type: "vm", backup_id: "900", backup_count: 2 }] }] }
        : detailsPayload);
    }),
  );
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

async function flushEffects() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

async function render(node: ReactNode) {
  await act(async () => root.render(
    <NextIntlClientProvider locale="en" messages={{ common: commonMessages, "devices-proxmox": messages }}>{node}</NextIntlClientProvider>,
  ));
  await flushEffects();
}

function buttons(text: string, exact = true): HTMLButtonElement[] {
  return [...document.body.querySelectorAll<HTMLButtonElement>("button")].filter((button) =>
    exact ? button.textContent?.trim() === text : button.textContent?.includes(text));
}

async function openMenu(label: string) {
  const trigger = document.body.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
  expect(trigger).not.toBeNull();
  await act(async () => {
    trigger?.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, button: 0 }));
  });
}

async function chooseMenuItem(text: string) {
  const item = [...document.body.querySelectorAll<HTMLElement>('[role="menuitem"]')].find((node) => node.textContent?.trim() === text);
  expect(item).not.toBeUndefined();
  await act(async () => item?.click());
  await flushEffects();
}

async function confirmDialog(text: string) {
  const dialog = document.body.querySelector('[role="dialog"]');
  expect(dialog).not.toBeNull();
  const confirm = [...(dialog?.querySelectorAll<HTMLButtonElement>("button") ?? [])].find((button) => button.textContent?.trim() === text);
  await act(async () => confirm?.click());
  await flushEffects();
  await flushEffects();
}

function PBSDetailsHarness({ onManualRefreshSettled }: { onManualRefreshSettled?: () => void }) {
  const { details: loaded, error, refresh } = usePBSDetails("pbs-server-qa", onManualRefreshSettled);
  return (
    <div>
      {error && <p role="alert">{error}</p>}
      {loaded && <p>{loaded.asset_id}</p>}
      <button type="button" onClick={refresh}>Refresh details</button>
    </div>
  );
}

describe("PBS user-facing controls", () => {
  it("turns a generic gateway failure into an actionable PBS message", async () => {
    vi.mocked(fetch).mockResolvedValue(respond({ error: "An internal error occurred." }, false, 502));
    await render(<PBSDetailsHarness />);

    expect(container.querySelector('[role="alert"]')?.textContent).toContain("Unable to reach Proxmox Backup Server");
    expect(container.textContent).not.toContain("An internal error occurred.");
  });

  it("refreshes parent status after a successful manual PBS refresh", async () => {
    const onManualRefreshSettled = vi.fn();
    await render(<PBSDetailsHarness onManualRefreshSettled={onManualRefreshSettled} />);
    expect(onManualRefreshSettled).not.toHaveBeenCalled();

    await act(async () => buttons("Refresh details")[0]?.click());
    await flushEffects();

    expect(onManualRefreshSettled).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain("pbs-server-qa");
  });

  it("refreshes parent status and retains inventory after a failed manual PBS refresh", async () => {
    const onManualRefreshSettled = vi.fn();
    await render(<PBSDetailsHarness onManualRefreshSettled={onManualRefreshSettled} />);
    expect(container.textContent).toContain("pbs-server-qa");

    vi.mocked(fetch).mockResolvedValueOnce(respond({ error: "An internal error occurred." }, false, 502));
    await act(async () => buttons("Refresh details")[0]?.click());
    await flushEffects();

    expect(onManualRefreshSettled).toHaveBeenCalledTimes(1);
    expect(container.querySelector('[role="alert"]')?.textContent).toContain("Unable to reach Proxmox Backup Server");
    expect(container.textContent).toContain("pbs-server-qa");
  });

  it("exposes datastore expansion as a named keyboard button", async () => {
    await render(<PBSDatastoreList assetId="pbs-server-qa" datastores={sortedStores} onChanged={() => undefined} />);

    const toggle = buttons("qa-store")[0];
    expect(toggle?.getAttribute("aria-expanded")).toBe("false");

    await act(async () => toggle?.click());
    expect(toggle?.getAttribute("aria-expanded")).toBe("true");
    expect(toggle?.getAttribute("aria-controls")).toBeTruthy();
    expect(document.getElementById(toggle?.getAttribute("aria-controls") ?? "")).not.toBeNull();
  });

  it("populates the snapshot datastore selector from discovered inventory", async () => {
    await render(<PBSSnapshotsView assetId="pbs-server-qa" details={details} />);

    const select = container.querySelector<HTMLSelectElement>('select[aria-label="Datastore"]');
    expect(select).not.toBeNull();
    expect([...select!.options].map((option) => option.value)).toEqual(["archive-store", "qa-store"]);
    expect(select?.value).toBe("archive-store");
  });

  it("does not allow protected snapshots into the forget paths", async () => {
    vi.mocked(fetch).mockImplementation(async (input: string | URL | Request) => respond(String(input).includes("/snapshots?")
      ? {
          snapshots: [
            { backup_type: "vm", backup_id: "900", backup_time: 1784088000, protected: true },
            { backup_type: "vm", backup_id: "901", backup_time: 1784088060, protected: false },
          ],
        }
      : detailsPayload));

    await render(<PBSSnapshotsView assetId="pbs-server-qa" details={details} />);

    const protectedCheckbox = container.querySelector<HTMLInputElement>('input[data-key="vm/900/1784088000"]');
    const disposableCheckbox = container.querySelector<HTMLInputElement>('input[data-key="vm/901/1784088060"]');
    expect(protectedCheckbox?.disabled).toBe(true);
    expect(disposableCheckbox?.disabled).toBe(false);

    const menus = [...container.querySelectorAll<HTMLButtonElement>("button.di-more")];
    const protectedMenu = menus.find((button) => button.getAttribute("aria-label")?.includes("vm/900"));
    const disposableMenu = menus.find((button) => button.getAttribute("aria-label")?.includes("vm/901"));

    await openMenu(disposableMenu?.getAttribute("aria-label") ?? "");
    await chooseMenuItem("Verify");
    const verifyCall = vi.mocked(fetch).mock.calls.find(([input, init]) =>
      String(input).includes("/snapshots/verify") && init?.method === "POST");
    expect(String(verifyCall?.[0])).toContain("/snapshots/verify?store=archive-store");

    await openMenu(disposableMenu?.getAttribute("aria-label") ?? "");
    await chooseMenuItem("Forget…");
    await confirmDialog("Forget");

    const forgetCall = vi.mocked(fetch).mock.calls.find(([input, init]) =>
      String(input).includes("/snapshots/forget") && init?.method === "DELETE");
    expect(String(forgetCall?.[0])).toContain("store=archive-store");
    expect(String(forgetCall?.[0])).toContain("backup-type=vm");
    expect(String(forgetCall?.[0])).toContain("backup-id=901");
    expect(String(forgetCall?.[0])).toContain("backup-time=1784088060");

    await openMenu(protectedMenu?.getAttribute("aria-label") ?? "");
    const protectedForget = [...document.body.querySelectorAll<HTMLElement>('[role="menuitem"]')].find((node) => node.textContent?.startsWith("Forget"));
    expect(protectedForget?.hasAttribute("data-disabled")).toBe(true);
  });

  it("uses the backend delete contract for group forget", async () => {
    let deleted = false;
    vi.mocked(fetch).mockImplementation(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/groups/forget") && init?.method === "DELETE") {
        deleted = true;
        return respond({ status: "forgotten" });
      }
      return respond(url.includes("/groups")
        ? { datastores: [{ store: "qa-store", groups: deleted ? [] : [{ backup_type: "vm", backup_id: "900", backup_count: 2 }] }] }
        : detailsPayload);
    });

    await render(<PBSCoverageList assetId="pbs-server-qa" />);
    await openMenu("Actions for vm/900");
    await chooseMenuItem("Forget group…");
    expect(vi.mocked(fetch).mock.calls.some(([input]) => String(input).includes("/groups/forget"))).toBe(false);
    await confirmDialog("Forget group");

    const groupForgetCall = vi.mocked(fetch).mock.calls.find(([input, init]) =>
      String(input).includes("/groups/forget") && init?.method === "DELETE");
    expect(String(groupForgetCall?.[0])).toContain("store=qa-store");
    expect(String(groupForgetCall?.[0])).toContain("backup-type=vm");
    expect(String(groupForgetCall?.[0])).toContain("backup-id=900");
    expect(container.textContent).toContain("No backup groups");
  });

  it("keeps groups of two namespaces apart and forgets the one picked in its namespace", async () => {
    vi.mocked(fetch).mockImplementation(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/groups/forget") && init?.method === "DELETE") return respond({ status: "deleted" });
      return respond(url.includes("/groups")
        ? { datastores: [{ store: "qa-store", groups: [
          { ns: "site-a", backup_type: "vm", backup_id: "100", backup_count: 3 },
          { ns: "site-b", backup_type: "vm", backup_id: "100", backup_count: 1 },
        ] }] }
        : detailsPayload);
    });

    await render(<PBSCoverageList assetId="pbs-server-qa" />);
    expect(container.textContent).toContain("qa-store/site-a");
    expect(container.textContent).toContain("qa-store/site-b");
    await openMenu("Actions for vm/100 site-b");
    await chooseMenuItem("Forget group…");
    await confirmDialog("Forget group");

    const call = vi.mocked(fetch).mock.calls.find(([input, init]) =>
      String(input).includes("/groups/forget") && init?.method === "DELETE");
    expect(String(call?.[0])).toContain("ns=site-b");
    expect(String(call?.[0])).toContain("backup-id=100");
  });

  it("requires confirmation before a datastore mutation", async () => {
    await render(<PBSDatastoreList assetId="pbs-server-qa" datastores={sortedStores} onChanged={() => undefined} />);

    await openMenu("Actions for archive-store");
    await chooseMenuItem("Run garbage collection…");

    expect(vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(0);
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain("Run garbage collection on archive-store?");

    await confirmDialog("Run garbage collection");

    const posts = vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === "POST");
    expect(posts).toHaveLength(1);
    expect(String(posts[0]?.[0])).toContain("/datastores/archive-store/gc");
    expect(container.querySelector('[role="status"]')?.textContent).toContain("Garbage collection started on archive-store");
  });

  it("labels the task-type filter, renders one status-filter set and offers no stop on a finished task", async () => {
    await render(<PBSTasksView details={details} onRefreshDetails={() => undefined} />);

    expect(container.querySelector('select[aria-label="Task type"]')).not.toBeNull();
    for (const label of ["All", "Errors", "Running"]) {
      expect(buttons(label)).toHaveLength(1);
    }

    const finishedTask = container.querySelector<HTMLButtonElement>("button.di-pbs-open");
    expect(finishedTask?.textContent).toBe("Verify");
    await act(async () => finishedTask?.click());
    await flushEffects();

    expect(finishedTask?.getAttribute("aria-expanded")).toBe("true");
    expect(container.textContent).toContain("weekly-integrity");
    expect(buttons("Stop task")).toHaveLength(0);
  });
});
