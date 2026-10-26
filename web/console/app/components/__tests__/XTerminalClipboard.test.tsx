import { act, createRef } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const terminalState = vi.hoisted(() => ({ instances: [] as Array<{
  paste: ReturnType<typeof vi.fn>;
  data: ((value: string) => void) | null;
}> }));
vi.mock("@xterm/xterm", () => ({ Terminal: class {
  options = {};
  unicode = { activeVersion: "" };
  data: ((value: string) => void) | null = null;
  paste = vi.fn((text: string) => { this.data?.(`\x1b[200~${text}\x1b[201~`); });
  constructor() { terminalState.instances.push(this); }
  loadAddon() {}
  open() {}
  focus() {}
  dispose() {}
  write() {}
  onData(callback: (value: string) => void) { this.data = callback; return { dispose() {} }; }
  onBinary() { return { dispose() {} }; }
  onResize() { return { dispose() {} }; }
} }));
vi.mock("@xterm/addon-fit", () => ({ FitAddon: class { fit() {} } }));
vi.mock("@xterm/addon-search", () => ({ SearchAddon: class {} }));
vi.mock("@xterm/addon-unicode11", () => ({ Unicode11Addon: class {} }));
vi.mock("@xterm/addon-web-links", () => ({ WebLinksAddon: class {} }));

import XTerminal, { type XTerminalHandle } from "../XTerminal";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
class FakeSocket {
  static OPEN = 1;
  static CONNECTING = 0;
  static instances: FakeSocket[] = [];
  readyState = 1;
  send = vi.fn();
  close() { this.readyState = 3; }
  constructor() { FakeSocket.instances.push(this); }
}
let root: Root;
let viewer: ReturnType<typeof createRef<XTerminalHandle>>;
const readText = vi.fn();
const onDataCapture = vi.fn();
async function render(url = "wss://example.test/first") {
  await act(async () => {
    root.render(<XTerminal ref={viewer} wsUrl={url} onDataCapture={onDataCapture} />);
  });
  await act(async () => { await vi.dynamicImportSettled(); });
}

beforeEach(async () => {
  terminalState.instances = [];
  FakeSocket.instances = [];
  readText.mockReset();
  onDataCapture.mockReset();
  vi.stubGlobal("WebSocket", FakeSocket);
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { readText } });
  viewer = createRef<XTerminalHandle>();
  root = createRoot(document.createElement("div"));
  await render();
});

afterEach(async () => {
  await act(async () => { root.unmount(); });
  vi.unstubAllGlobals();
});

describe("terminal paste", () => {
  it("uses xterm's paste path so shell bracketed paste and broadcast agree", async () => {
    readText.mockResolvedValue("first\nsecond");
    await expect(viewer.current!.pasteFromClipboard()).resolves.toEqual({ ok: true });
    expect(terminalState.instances[0].paste).toHaveBeenCalledWith("first\nsecond");
    expect(FakeSocket.instances[0].send).toHaveBeenCalledExactlyOnceWith("\x1b[200~first\nsecond\x1b[201~");
    expect(onDataCapture).toHaveBeenCalledExactlyOnceWith("\x1b[200~first\nsecond\x1b[201~");
  });

  it("does not paste into a replacement SSH session after a clipboard prompt", async () => {
    let resolveRead!: (value: string) => void;
    readText.mockReturnValue(new Promise<string>((resolve) => { resolveRead = resolve; }));
    const pending = viewer.current!.pasteFromClipboard();
    await render("wss://example.test/second");
    resolveRead("secret from first session");
    await expect(pending).resolves.toEqual({ ok: false, reason: "disconnected" });
    expect(FakeSocket.instances).toHaveLength(2);
    for (const terminal of terminalState.instances) expect(terminal.paste).not.toHaveBeenCalled();
    for (const socket of FakeSocket.instances) expect(socket.send).not.toHaveBeenCalled();
  });
});
