import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ConnectedAgentsProvider, useConnectedAgentsContext } from "../ConnectedAgentsContext";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let container: HTMLDivElement;
let current: ReturnType<typeof useConnectedAgentsContext>;
let renders: number;
let payload: { assets: string[]; assetsInfo?: Array<{ id: string; has_tmux: boolean }> };

function Consumer() {
  current = useConnectedAgentsContext();
  renders++;
  return null;
}

beforeEach(() => {
  vi.useFakeTimers();
  renders = 0;
  payload = { assets: ["a", "b"], assetsInfo: [{ id: "a", has_tmux: true }, { id: "b", has_tmux: false }] };
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(payload))));
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("connected agent polling", () => {
  it("does not start a hidden mount and refreshes when first shown", async () => {
    let visibility: DocumentVisibilityState = "hidden";
    vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
    await act(async () => root.render(<ConnectedAgentsProvider><Consumer /></ConnectedAgentsProvider>));
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(fetch).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
    visibility = "visible";
    await act(async () => document.dispatchEvent(new Event("visibilitychange")));
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("does not rerender consumers for unchanged or reordered polling data", async () => {
    await act(async () => root.render(<ConnectedAgentsProvider><Consumer /></ConnectedAgentsProvider>));
    const previous = current;
    const before = renders;
    payload.assets.reverse();
    payload.assetsInfo?.reverse();
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(current).toBe(previous);
    expect(renders).toBe(before);
  });

  it("publishes changed values and removals, retaining the last map when info is omitted", async () => {
    await act(async () => root.render(<ConnectedAgentsProvider><Consumer /></ConnectedAgentsProvider>));
    const oldMap = current.agentTmuxStatus;
    payload.assetsInfo = [{ id: "a", has_tmux: false }, { id: "b", has_tmux: true }];
    await act(async () => current.refreshConnected());
    expect(current.agentTmuxStatus).not.toBe(oldMap);
    expect(current.agentTmuxStatus.get("a")).toBe(false);
    payload = { assets: ["a"], assetsInfo: [{ id: "a", has_tmux: false }] };
    await act(async () => current.refreshConnected());
    expect([...current.connectedAgentIds]).toEqual(["a"]);
    expect(current.agentTmuxStatus.has("b")).toBe(false);
    const lastMap = current.agentTmuxStatus;
    payload = { assets: ["a"] };
    await act(async () => current.refreshConnected());
    expect(current.agentTmuxStatus).toBe(lastMap);
    payload = { assets: [], assetsInfo: [] };
    await act(async () => current.refreshConnected());
    expect(current.agentTmuxStatus.size).toBe(0);
  });
});
