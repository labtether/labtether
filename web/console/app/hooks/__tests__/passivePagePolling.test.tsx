import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useProxmoxFetch } from "../../[locale]/(console)/nodes/[id]/proxmox/useProxmoxData";
import { usePBSDetails } from "../../[locale]/(console)/nodes/[id]/pbs/usePBSData";
import { DockerLogsTab } from "../../[locale]/(console)/nodes/[id]/DockerLogsTab";
import { useTerminalSessionLibrary } from "../../components/terminal/useTerminalSessionLibrary";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
let container: HTMLDivElement;
let visibility: DocumentVisibilityState;
let proxmox: ReturnType<typeof useProxmoxFetch<{ name: string }>>;
let pbs: ReturnType<typeof usePBSDetails>;
const noop = () => {};

function Proxmox({ path = "/fixture/a" }: { path?: string }) {
  proxmox = useProxmoxFetch<{ name: string }>(path, 5000);
  return null;
}
function PBS({ onSettled }: { onSettled: () => void }) {
  pbs = usePBSDetails("fixture", onSettled);
  return null;
}
function Library({ open }: { open: boolean }) {
  useTerminalSessionLibrary({ isOpen: open, onSessionSelect: noop, onBookmarkConnect: noop, onArchivedView: noop });
  return null;
}
function pending() {
  let resolve!: (response: Response) => void;
  const promise = new Promise<Response>((done) => { resolve = done; });
  return { promise, resolve };
}
const json = (data: unknown) => new Response(JSON.stringify(data));
async function show(value: DocumentVisibilityState) {
  visibility = value;
  await act(async () => document.dispatchEvent(new Event("visibilitychange")));
}
async function click(label: string) {
  const button = [...container.querySelectorAll("button")].find((item) => item.textContent === label);
  expect(button).toBeTruthy();
  await act(async () => button!.click());
}

beforeEach(() => {
  vi.useFakeTimers();
  visibility = "visible";
  vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});
afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("passive page request ownership", () => {
  it("keeps Proxmox idle while hidden, then refreshes on return at the same cadence", async () => {
    const fetchMock = vi.fn(async () => json({ data: { name: "current" } }));
    vi.stubGlobal("fetch", fetchMock);
    visibility = "hidden";
    await act(async () => root.render(<Proxmox />));
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
    await show("visible");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(5000));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await show("hidden");
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(vi.getTimerCount()).toBe(0);
    await show("visible");
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("does not stack slow reads and discards the old target before loading the new one", async () => {
    const old = pending();
    const fetchMock = vi.fn().mockReturnValueOnce(old.promise).mockImplementation(async () => json({ data: { name: "new" } }));
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<Proxmox />));
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const signal = fetchMock.mock.calls[0][1].signal as AbortSignal;
    await act(async () => root.render(<Proxmox path="/fixture/b" />));
    expect(signal.aborted).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await act(async () => old.resolve(json({ data: { name: "stale" } })));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[1][0]).toBe("/fixture/b");
    expect(proxmox.data?.name).toBe("new");
  });

  it("keeps Docker logs frozen with Follow off, and pauses Follow while hidden", async () => {
    const fetchMock = vi.fn(async () => json({ logs: "line" }));
    vi.stubGlobal("fetch", fetchMock);
    visibility = "hidden";
    await act(async () => root.render(<DockerLogsTab containerId="fixture" />));
    expect(fetchMock).not.toHaveBeenCalled();
    await show("visible");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await show("hidden");
    await show("visible");
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await click("Follow");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await act(async () => vi.advanceTimersByTimeAsync(5000));
    expect(fetchMock).toHaveBeenCalledTimes(3);
    await show("hidden");
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(fetchMock).toHaveBeenCalledTimes(3);
    await show("visible");
    expect(fetchMock).toHaveBeenCalledTimes(4);
  });

  it("a manual Docker refresh replaces a pending old read instead of accepting its stale result", async () => {
    const old = pending();
    const fresh = pending();
    const fetchMock = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<DockerLogsTab containerId="fixture" />));
    await click("Refresh");
    expect(fetchMock.mock.calls[0][1].signal.aborted).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await act(async () => old.resolve(json({ logs: "stale log" })));
    expect(container.textContent).not.toContain("stale log");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await act(async () => fresh.resolve(json({ logs: "fresh log" })));
    expect(container.textContent).toContain("fresh log");
  });

  it("resumes an interrupted manual-only log load on return without accepting the hidden result", async () => {
    const old = pending();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json({ logs: "first log" }))
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce(json({ logs: "resumed log" }));
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<DockerLogsTab containerId="fixture" />));
    await click("Refresh");
    await show("hidden");
    expect(fetchMock.mock.calls[1][1].signal.aborted).toBe(true);
    await act(async () => old.resolve(json({ logs: "hidden log" })));
    expect(container.textContent).toContain("first log");
    await show("visible");
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(container.textContent).toContain("resumed log");
    expect(vi.getTimerCount()).toBe(0);
  });

  it("queues a manual-only refresh that arrives while hidden, then performs it once on return", async () => {
    const fetchMock = vi.fn(async () => json({ logs: "current log" }));
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<DockerLogsTab containerId="fixture" />));
    await show("hidden");
    // An action that finished in the background can request this same refresh.
    await click("Refresh");
    await click("Refresh");
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await show("visible");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await show("hidden");
    await show("visible");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not start a parent status refresh after a manual PBS read is hidden or unmounted", async () => {
    const old = pending();
    const onSettled = vi.fn();
    const fetchMock = vi.fn().mockResolvedValueOnce(json({ asset_id: "fixture" })).mockReturnValueOnce(old.promise);
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<PBS onSettled={onSettled} />));
    await act(async () => { void pbs.refresh(); });
    await show("hidden");
    await act(async () => root.render(null));
    expect(fetchMock.mock.calls[1][1].signal.aborted).toBe(true);
    await act(async () => old.resolve(json({ asset_id: "stale" })));
    expect(onSettled).not.toHaveBeenCalled();
  });

  it("keeps a deferred PBS manual refresh manual when it can run again", async () => {
    const onSettled = vi.fn();
    vi.stubGlobal("fetch", vi.fn(async () => json({ asset_id: "fixture" })));
    await act(async () => root.render(<PBS onSettled={onSettled} />));
    await show("hidden");
    await act(async () => { void pbs.refresh(); });
    expect(onSettled).not.toHaveBeenCalled();
    expect(fetch).toHaveBeenCalledTimes(1);
    await show("visible");
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(onSettled).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(onSettled).toHaveBeenCalledTimes(1);
  });

  it("cancels both session-library reads on close and leaves no background polling", async () => {
    const reads = [pending(), pending()];
    const fetchMock = vi.fn().mockReturnValueOnce(reads[0].promise).mockReturnValueOnce(reads[1].promise);
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<Library open={false} />));
    expect(fetchMock).not.toHaveBeenCalled();
    await act(async () => root.render(<Library open />));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await act(async () => root.render(<Library open={false} />));
    expect(fetchMock.mock.calls.every((call) => call[1].signal.aborted)).toBe(true);
    await act(async () => reads.forEach((read) => read.resolve(json({}))));
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(vi.getTimerCount()).toBe(0);
  });
  it("cancels the other library request when one member of the parallel read fails", async () => {
    const sibling = pending();
    const fetchMock = vi.fn().mockRejectedValueOnce(new Error("offline")).mockReturnValueOnce(sibling.promise);
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<Library open />));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[1][1].signal.aborted).toBe(true);
    await act(async () => sibling.resolve(json({})));
  });

});
