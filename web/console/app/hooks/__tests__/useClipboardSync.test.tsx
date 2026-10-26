import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useClipboardSync } from "../useClipboardSync";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
let container: HTMLDivElement;
let hook: ReturnType<typeof useClipboardSync>;
const readText = vi.fn();
const writeText = vi.fn();
const readRemoteText = vi.fn();
const writeRemoteText = vi.fn();

function Harness({ nodeId = "node-a", sessionKey = "session-a" }: { nodeId?: string; sessionKey?: string }) {
  hook = useClipboardSync({ nodeId, sessionKey, enabled: true, readRemoteText, writeRemoteText });
  return null;
}

beforeEach(async () => {
  vi.resetAllMocks();
  vi.stubGlobal("ClipboardItem", undefined);
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { readText, writeText } });
  container = document.createElement("div");
  root = createRoot(container);
  await act(async () => { root.render(<Harness />); });
});

afterEach(async () => {
  await act(async () => { root.unmount(); });
  vi.unstubAllGlobals();
});

describe("clipboard fidelity and session ownership", () => {
  it.each(["", "  text\r\n雪🦉  "])("copies exact text in both directions: %j", async (text) => {
    readRemoteText.mockResolvedValue(text);
    readText.mockResolvedValue(text);
    await act(async () => { await hook.pullFromRemote(); });
    expect(writeText).toHaveBeenCalledWith(text);
    await act(async () => { await hook.pushToRemote(); });
    expect(writeRemoteText).toHaveBeenCalledWith(text);
    expect(hook.lastSync).toBe("success");
  });

  it("never sends a clipboard read to a different node after permission is granted", async () => {
    let finishRead!: (text: string) => void;
    readText.mockReturnValue(new Promise<string>((resolve) => { finishRead = resolve; }));
    let pending!: Promise<void>;
    await act(async () => { pending = hook.pushToRemote(); });
    await act(async () => { root.render(<Harness nodeId="node-b" />); });
    await act(async () => { finishRead("private text"); await pending; });
    expect(writeRemoteText).not.toHaveBeenCalled();
    expect(hook.lastSync).toBe("idle");
    expect(hook.syncing).toBe(false);
  });

  it("does not send clipboard text into a replacement session on the same node", async () => {
    let finishRead!: (text: string) => void;
    readText.mockReturnValue(new Promise<string>((resolve) => { finishRead = resolve; }));
    let pending!: Promise<void>;
    await act(async () => { pending = hook.pushToRemote(); });
    await act(async () => { root.render(<Harness sessionKey="session-b" />); });
    await act(async () => { finishRead("old session text"); await pending; });
    expect(writeRemoteText).not.toHaveBeenCalled();
    expect(hook.lastSync).toBe("idle");
  });

  it("starts Safari clipboard write during the click, before the remote reply", async () => {
    let finishRead!: (text: string) => void;
    let clipboardData!: Record<string, Promise<Blob>>;
    vi.stubGlobal("ClipboardItem", class {
      constructor(data: Record<string, Promise<Blob>>) { clipboardData = data; }
    });
    const write = vi.fn(async () => { await clipboardData["text/plain"]; });
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { write } });
    readRemoteText.mockReturnValue(new Promise<string>((resolve) => { finishRead = resolve; }));
    let pending!: Promise<void>;
    await act(async () => {
      pending = hook.pullFromRemote();
      expect(write).toHaveBeenCalledTimes(1);
    });
    await act(async () => { finishRead("remote"); await pending; });
    expect((await clipboardData["text/plain"]).size).toBe(6);
    expect(hook.lastSync).toBe("success");
  });
});
