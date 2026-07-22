import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PBSDatastoresCard } from "../../PBSDatastoresCard";
import { PBSBackupGroupsTab } from "../PBSBackupGroupsTab";
import { PBSDatastoresTab } from "../PBSDatastoresTab";
import { PBSSnapshotsTab } from "../PBSSnapshotsTab";
import { PBSTasksTab } from "../PBSTasksTab";
import { usePBSDetails } from "../usePBSData";

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

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      const payload = url.includes("/groups")
        ? {
            datastores: [
              {
                store: "qa-store",
                groups: [
                  {
                    backup_type: "vm",
                    backup_id: "900",
                    backup_count: 2,
                  },
                ],
              },
            ],
          }
        : detailsPayload;
      return {
        ok: true,
        status: 200,
        json: async () => payload,
      } as Response;
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

function PBSDetailsHarness({
  onManualRefreshSettled,
}: {
  onManualRefreshSettled?: () => void;
}) {
  const { details, error, refresh } = usePBSDetails(
    "pbs-server-qa",
    onManualRefreshSettled,
  );
  return (
    <div>
      {error && <p role="alert">{error}</p>}
      {details && <p>{details.asset_id}</p>}
      <button type="button" onClick={refresh}>Refresh details</button>
    </div>
  );
}

describe("PBS user-facing controls", () => {
  it("turns a generic gateway failure into an actionable PBS message", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      status: 502,
      json: async () => ({ error: "An internal error occurred." }),
    } as Response);

    await act(async () => root.render(<PBSDetailsHarness />));
    await flushEffects();

    expect(container.querySelector('[role="alert"]')?.textContent).toContain(
      "Unable to reach Proxmox Backup Server",
    );
    expect(container.textContent).not.toContain("An internal error occurred.");
  });

  it("refreshes parent status after a successful manual PBS refresh", async () => {
    const onManualRefreshSettled = vi.fn();
    await act(async () =>
      root.render(
        <PBSDetailsHarness onManualRefreshSettled={onManualRefreshSettled} />,
      ),
    );
    await flushEffects();
    expect(onManualRefreshSettled).not.toHaveBeenCalled();

    const refresh = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Refresh details",
    );
    await act(async () => refresh?.click());
    await flushEffects();

    expect(onManualRefreshSettled).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain("pbs-server-qa");
  });

  it("refreshes parent status and retains inventory after a failed manual PBS refresh", async () => {
    const onManualRefreshSettled = vi.fn();
    await act(async () =>
      root.render(
        <PBSDetailsHarness onManualRefreshSettled={onManualRefreshSettled} />,
      ),
    );
    await flushEffects();
    expect(container.textContent).toContain("pbs-server-qa");

    vi.mocked(fetch).mockResolvedValueOnce({
      ok: false,
      status: 502,
      json: async () => ({ error: "An internal error occurred." }),
    } as Response);

    const refresh = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Refresh details",
    );
    await act(async () => refresh?.click());
    await flushEffects();

    expect(onManualRefreshSettled).toHaveBeenCalledTimes(1);
    expect(container.querySelector('[role="alert"]')?.textContent).toContain(
      "Unable to reach Proxmox Backup Server",
    );
    expect(container.textContent).toContain("pbs-server-qa");
  });

  it("exposes datastore expansion as a named keyboard button", async () => {
    await act(async () => {
      root.render(
        <PBSDatastoresCard
          assetId="pbs-server-qa"
          datastores={detailsPayload.datastores.map((store) => ({
            ...store,
            group_count: 1,
            snapshot_count: 2,
            usage_percent: 25,
          }))}
        />,
      );
    });

    const toggle = [...container.querySelectorAll("button")].find((button) =>
      button.textContent?.includes("qa-store"),
    );
    expect(toggle?.getAttribute("aria-expanded")).toBe("false");

    await act(async () => toggle?.click());
    expect(toggle?.getAttribute("aria-expanded")).toBe("true");
    expect(toggle?.getAttribute("aria-controls")).toBeTruthy();
    expect(document.getElementById(toggle?.getAttribute("aria-controls") ?? "")).not.toBeNull();
  });

  it("populates the snapshot datastore selector from discovered inventory", async () => {
    await act(async () => root.render(<PBSSnapshotsTab assetId="pbs-server-qa" />));
    await flushEffects();

    const select = container.querySelector<HTMLSelectElement>('select[aria-label="Datastore"]');
    expect(select).not.toBeNull();
    expect([...select!.options].map((option) => option.value)).toEqual([
      "archive-store",
      "qa-store",
    ]);
    expect(select?.value).toBe("archive-store");
  });

  it("does not allow protected snapshots into the bulk-forget path", async () => {
    vi.mocked(fetch).mockImplementation(async (input: string | URL | Request) => {
      const url = String(input);
      const payload = url.includes("/snapshots?")
        ? {
            snapshots: [
              {
                backup_type: "vm",
                backup_id: "900",
                backup_time: 1784088000,
                protected: true,
              },
              {
                backup_type: "vm",
                backup_id: "901",
                backup_time: 1784088060,
                protected: false,
              },
            ],
          }
        : detailsPayload;
      return {
        ok: true,
        status: 200,
        json: async () => payload,
      } as Response;
    });

    await act(async () => root.render(<PBSSnapshotsTab assetId="pbs-server-qa" />));
    await flushEffects();

    const load = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Load",
    );
    await act(async () => load?.click());
    await flushEffects();

    const protectedCheckbox = container.querySelector<HTMLInputElement>(
      'input[aria-label="Select snapshot vm/900/1784088000"]',
    );
    const disposableCheckbox = container.querySelector<HTMLInputElement>(
      'input[aria-label="Select snapshot vm/901/1784088060"]',
    );
    expect(protectedCheckbox?.disabled).toBe(true);
    expect(disposableCheckbox?.disabled).toBe(false);

    const verify = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Verify",
    );
    await act(async () => verify?.click());
    const confirmVerify = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Confirm Verify",
    );
    await act(async () => confirmVerify?.click());
    await flushEffects();

    const verifyCall = vi.mocked(fetch).mock.calls.find(([input, init]) =>
      String(input).includes("/snapshots/verify") && init?.method === "POST",
    );
    expect(String(verifyCall?.[0])).toContain("/snapshots/verify?store=archive-store");

    const forget = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Forget" && !button.hasAttribute("disabled"),
    );
    await act(async () => forget?.click());
    const confirmForget = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Confirm",
    );
    await act(async () => confirmForget?.click());
    await flushEffects();

    const forgetCall = vi.mocked(fetch).mock.calls.find(([input, init]) =>
      String(input).includes("/snapshots/forget") && init?.method === "DELETE",
    );
    expect(String(forgetCall?.[0])).toContain("store=archive-store");
    expect(String(forgetCall?.[0])).toContain("backup-type=vm");
    expect(String(forgetCall?.[0])).toContain("backup-id=901");
    expect(String(forgetCall?.[0])).toContain("backup-time=1784088060");
  });

  it("uses the backend delete contract for group forget", async () => {
    let deleted = false;
    vi.mocked(fetch).mockImplementation(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/groups/forget") && init?.method === "DELETE") {
        deleted = true;
        return {
          ok: true,
          status: 200,
          json: async () => ({ status: "forgotten" }),
        } as Response;
      }
      const payload = url.includes("/groups")
        ? {
            datastores: [
              {
                store: "qa-store",
                groups: deleted
                  ? []
                  : [
                      {
                        backup_type: "vm",
                        backup_id: "900",
                        backup_count: 2,
                      },
                    ],
              },
            ],
          }
        : detailsPayload;
      return {
        ok: true,
        status: 200,
        json: async () => payload,
      } as Response;
    });

    await act(async () => root.render(<PBSBackupGroupsTab assetId="pbs-server-qa" />));
    await flushEffects();

    const open = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Forget Group",
    );
    await act(async () => open?.click());
    await flushEffects();

    const review = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Review Forget",
    );
    await act(async () => review?.click());
    const confirm = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Confirm Forget",
    );
    await act(async () => confirm?.click());
    await flushEffects();

    const groupForgetCall = vi.mocked(fetch).mock.calls.find(([input, init]) =>
      String(input).includes("/groups/forget") && init?.method === "DELETE",
    );
    expect(String(groupForgetCall?.[0])).toContain("store=qa-store");
    expect(String(groupForgetCall?.[0])).toContain("backup-type=vm");
    expect(String(groupForgetCall?.[0])).toContain("backup-id=900");
    expect(container.textContent).not.toContain("Forget Backup Group");
    expect(container.textContent).toContain("No groups in this datastore.");
  });

  it("requires confirmation before a datastore mutation", async () => {
    await act(async () => root.render(<PBSDatastoresTab assetId="pbs-server-qa" />));
    await flushEffects();

    const runGC = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Run GC",
    );
    await act(async () => runGC?.click());

    const postCallsBeforeConfirm = vi
      .mocked(fetch)
      .mock.calls.filter(([, init]) => init?.method === "POST");
    expect(postCallsBeforeConfirm).toHaveLength(0);
    expect(container.textContent).toContain("Confirm: run garbage collection on archive-store?");

    const confirm = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Confirm",
    );
    await act(async () => confirm?.click());
    await flushEffects();

    const postCallsAfterConfirm = vi
      .mocked(fetch)
      .mock.calls.filter(([, init]) => init?.method === "POST");
    expect(postCallsAfterConfirm).toHaveLength(1);
    expect(container.querySelector('[role="status"]')?.textContent).toContain(
      "requested for archive-store",
    );
  });

  it("labels the task-type filter and renders one status-filter control set", async () => {
    await act(async () => root.render(<PBSTasksTab assetId="pbs-server-qa" />));
    await flushEffects();

    expect(container.querySelector('select[aria-label="Task type"]')).not.toBeNull();
    for (const label of ["All", "Errors", "Running"]) {
      const matching = [...container.querySelectorAll("button")].filter(
        (button) => button.textContent?.trim() === label,
      );
      expect(matching).toHaveLength(1);
    }

    const completedTask = [...container.querySelectorAll("button")].find((button) =>
      button.textContent?.includes("weekly-integrity"),
    );
    await act(async () => completedTask?.click());
    await flushEffects();

    const finishedButton = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Task Finished",
    );
    expect(finishedButton).not.toBeUndefined();
    expect(finishedButton?.hasAttribute("disabled")).toBe(true);
  });
});
