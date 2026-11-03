import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@xterm/xterm", () => ({ Terminal: class {
  options = {};
  unicode = { activeVersion: "" };
  parser = { registerCsiHandler: () => ({ dispose() {} }), registerDcsHandler: () => ({ dispose() {} }), registerOscHandler: () => ({ dispose() {} }) };
  loadAddon() {}
  attachCustomKeyEventHandler() {}
  open() {}
  focus() {}
  dispose() {}
  write() {}
  onData() { return { dispose() {} }; }
  onBinary() { return { dispose() {} }; }
  onResize() { return { dispose() {} }; }
} }));
vi.mock("@xterm/addon-fit", () => ({ FitAddon: class { fit() {} proposeDimensions() { return undefined; } } }));
vi.mock("@xterm/addon-search", () => ({ SearchAddon: class {} }));
vi.mock("@xterm/addon-unicode11", () => ({ Unicode11Addon: class {} }));
vi.mock("@xterm/addon-web-links", () => ({ WebLinksAddon: class {} }));

import XTerminal from "../XTerminal";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

/** A socket whose upgrade gets no answer: it stays CONNECTING until closed, and closing it fails it like a browser. */
class HangingSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static instances: HangingSocket[] = [];
  readyState = 0;
  binaryType = "";
  onopen: (() => void) | null = null;
  onmessage: ((event: unknown) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: ((event: { code: number; reason: string }) => void) | null = null;
  send = vi.fn();
  close = vi.fn(() => {
    const wasConnecting = this.readyState === 0;
    this.readyState = 3;
    if (wasConnecting) this.onerror?.();
    this.onclose?.({ code: 1006, reason: "" });
  });
  constructor() { HangingSocket.instances.push(this); }
}

let root: Root;
const onError = vi.fn();
const onDisconnected = vi.fn();

beforeEach(async () => {
  vi.useFakeTimers();
  HangingSocket.instances = [];
  onError.mockReset();
  onDisconnected.mockReset();
  vi.stubGlobal("WebSocket", HangingSocket);
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
  root = createRoot(document.createElement("div"));
  await act(async () => {
    root.render(<XTerminal wsUrl="wss://example.test/terminal/sessions/s1/stream" onError={onError} onDisconnected={onDisconnected} />);
  });
  await act(async () => { await vi.dynamicImportSettled(); });
});

afterEach(async () => {
  await act(async () => { root.unmount(); });
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("terminal stream that never opens", () => {
  it("gives up after 45 s so the pane shows the error and Try again", async () => {
    const socket = HangingSocket.instances[0];
    await act(async () => { vi.advanceTimersByTime(44_000); });
    expect(socket.close).not.toHaveBeenCalled();

    await act(async () => { vi.advanceTimersByTime(1_000); });
    expect(socket.close).toHaveBeenCalledOnce();
    expect(onError).toHaveBeenCalled();
    expect(onDisconnected).toHaveBeenCalledOnce();
  });

  it("leaves a stream that opened alone", async () => {
    const socket = HangingSocket.instances[0];
    await act(async () => { socket.readyState = 1; socket.onopen?.(); });
    await act(async () => { vi.advanceTimersByTime(120_000); });
    expect(socket.close).not.toHaveBeenCalled();
    expect(onDisconnected).not.toHaveBeenCalled();
  });
});
