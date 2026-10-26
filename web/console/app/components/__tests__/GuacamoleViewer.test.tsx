import { act, createRef } from "react";
import { createRoot, type Root } from "react-dom/client";
import type { InputStream, Mouse, OutputStream } from "guacamole-common-js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type FakePointer = {
  onmousedown: ((state: Mouse.State) => void) | null;
  onmouseup: ((state: Mouse.State) => void) | null;
  onmousemove: ((state: Mouse.State) => void) | null;
  reset: ReturnType<typeof vi.fn>;
};
type FakeClient = {
  onclipboard: ((stream: InputStream, mimetype: string) => void) | null;
  onstatechange: ((state: number) => void) | null;
  onerror: (() => void) | null;
  sendMouseState: ReturnType<typeof vi.fn>;
  sendKeyEvent: ReturnType<typeof vi.fn>;
  disconnect: ReturnType<typeof vi.fn>;
  clipboard: OutputStream;
};
const mocks = vi.hoisted(() => ({ clients: [] as FakeClient[], pointers: [] as FakePointer[], resets: [] as Array<ReturnType<typeof vi.fn>> }));
vi.mock("guacamole-common-js", () => {
  class Pointer {
    onmousedown = null;
    onmouseup = null;
    onmousemove = null;
    reset = vi.fn();
    static Touchscreen = class extends Pointer {};
    constructor() { mocks.pointers.push(this); }
  }
  const module = {
    WebSocketTunnel: class {},
    Mouse: Pointer,
    Keyboard: class {
      onkeydown = null;
      onkeyup = null;
      reset = vi.fn();
      constructor() { mocks.resets.push(this.reset); }
    },
    Client: class {
      onclipboard = null;
      onstatechange = null;
      onerror = null;
      sendMouseState = vi.fn();
      sendKeyEvent = vi.fn();
      disconnect = vi.fn();
      connect = vi.fn();
      display = document.createElement("div");
      clipboard = { sendBlob: vi.fn(), sendEnd: vi.fn() };
      createClipboardStream = () => this.clipboard;
      getDisplay = () => ({ getElement: () => this.display });
      constructor() { mocks.clients.push(this); }
    },
  };
  return { ...module, default: module };
});

import GuacamoleViewer, { type GuacamoleViewerHandle } from "../GuacamoleViewer";
import { readGuacamoleClipboard, writeGuacamoleClipboard } from "../guacamole/clipboard";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
let container: HTMLDivElement;
const writeText = vi.fn(async () => {});
const onConnect = vi.fn();
const onDisconnect = vi.fn();
const inputStream = (): InputStream => ({ onblob: null, onend: null, sendAck: vi.fn() });
const base64 = (bytes: Uint8Array) => btoa(Array.from(bytes, (byte) => String.fromCharCode(byte)).join(""));
const pointerState: Mouse.State = { x: 240, y: 120, left: true, middle: false, right: false, up: false, down: false };

async function render(wsUrl: string, ref = createRef<GuacamoleViewerHandle>()) {
  if (mocks.clients.length === 0) {
    // Mount while the session URL is loading, then connect. This also avoids
    // Vitest's concurrent-import mock race between keyboard and client setup.
    await act(async () => { root.render(<GuacamoleViewer ref={ref} wsUrl={null} />); });
    await act(async () => { await vi.dynamicImportSettled(); });
  }
  await act(async () => {
    root.render(<GuacamoleViewer ref={ref} wsUrl={wsUrl} onConnect={onConnect} onDisconnect={onDisconnect} />);
  });
  await act(async () => { await vi.dynamicImportSettled(); });
  return ref;
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.clients = [];
  mocks.pointers = [];
  mocks.resets = [];
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => { root.unmount(); });
  container.remove();
});

describe("RDP clipboard Unicode", () => {
  it.each(["", "\uFEFF  emoji 😀 / 日本語\r\n".repeat(500)])("writes exact UTF-8 bytes and ends without waiting for ACKs (%#)", (text) => {
    const stream = { sendBlob: vi.fn(), sendEnd: vi.fn() };
    writeGuacamoleClipboard(stream, text);
    const bytes = stream.sendBlob.mock.calls.flatMap(([blob]) => {
      const binary = atob(blob);
      expect(binary.length).toBeLessThanOrEqual(6048);
      return Array.from(binary, (character) => character.charCodeAt(0));
    });
    expect(bytes).toEqual(Array.from(new TextEncoder().encode(text)));
    expect(stream.sendEnd).toHaveBeenCalledOnce();
  });

  it("reads split multibyte characters and preserves leading BOM and whitespace", () => {
    const stream = inputStream();
    const onText = vi.fn();
    const text = "\uFEFF  😀日本語\r\n";
    readGuacamoleClipboard(stream, onText);
    for (const byte of new TextEncoder().encode(text)) stream.onblob!(base64(new Uint8Array([byte])));
    stream.onend!();
    expect(onText).toHaveBeenCalledExactlyOnceWith(text);
    expect(stream.sendAck).toHaveBeenCalledTimes(new TextEncoder().encode(text).length);
  });

  it("never replaces the clipboard with malformed or cancelled stream text", () => {
    const onText = vi.fn();
    const invalid = inputStream();
    readGuacamoleClipboard(invalid, onText);
    invalid.onblob!(base64(new Uint8Array([0xff])));
    expect(invalid.sendAck).toHaveBeenCalledWith("Invalid UTF-8 clipboard data", 0x0300);
    const cancelled = inputStream();
    const stopReading = readGuacamoleClipboard(cancelled, onText);
    const staleBlob = cancelled.onblob!;
    const staleEnd = cancelled.onend!;
    stopReading();
    staleBlob(base64(new TextEncoder().encode("old session")));
    staleEnd();
    expect(onText).not.toHaveBeenCalled();
  });
});

describe("RDP viewer input and session ownership", () => {
  it("forwards touchscreen input with display scale and releases virtual keys on blur", async () => {
    const ref = await render("wss://example.test/rdp");
    const client = mocks.clients[0];
    const touch = mocks.pointers[1];
    touch.onmousedown!(pointerState);
    expect(client.sendMouseState).toHaveBeenCalledWith(pointerState, true);
    ref.current!.sendKey(0xffe3, true);
    window.dispatchEvent(new Event("blur"));
    expect(client.sendKeyEvent.mock.calls).toEqual([[1, 0xffe3], [0, 0xffe3]]);
    expect(mocks.resets[0]).toHaveBeenCalledOnce();
    for (const pointer of mocks.pointers) expect(pointer.reset).toHaveBeenCalledOnce();
    client.sendMouseState.mockClear();
    touch.onmousedown!(pointerState);
    expect(client.sendMouseState).not.toHaveBeenCalled();
    const display = container.querySelector(".vncContainer")!.firstElementChild!;
    const cancelledTouches: Array<number> = [];
    display.addEventListener("touchend", (event) => cancelledTouches.push((event as TouchEvent).changedTouches.length));
    display.dispatchEvent(new Event("touchstart"));
    display.dispatchEvent(new Event("touchcancel"));
    touch.onmousedown!(pointerState);
    expect(cancelledTouches).toEqual([0]);
    expect(client.sendMouseState).not.toHaveBeenCalled();
  });

  it("pastes Unicode and empty text through native UTF-8 streams", async () => {
    await render("wss://example.test/rdp");
    const surface = container.querySelector<HTMLDivElement>(".vncContainer")!;
    surface.focus();
    for (const text of ["😀\n", ""]) {
      const event = new Event("paste", { bubbles: true, cancelable: true });
      Object.defineProperty(event, "clipboardData", { value: { types: ["text/plain"], getData: () => text } });
      surface.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(true);
    }
    expect(mocks.clients[0].clipboard.sendBlob).toHaveBeenCalledExactlyOnceWith(base64(new TextEncoder().encode("😀\n")));
    expect(mocks.clients[0].clipboard.sendEnd).toHaveBeenCalledTimes(2);
  });

  it("ignores old clipboard and connection callbacks after replacing a session", async () => {
    await render("wss://example.test/first");
    const old = mocks.clients[0];
    const stream = inputStream();
    old.onclipboard!(stream, "text/plain;charset=utf-8");
    const oldBlob = stream.onblob!;
    const oldEnd = stream.onend!;
    const oldState = old.onstatechange!;
    const oldError = old.onerror!;
    await render("wss://example.test/second");
    container.querySelector<HTMLDivElement>(".vncContainer")!.focus();
    oldBlob(base64(new TextEncoder().encode("old")));
    oldEnd();
    oldState(3);
    oldState(5);
    oldError();
    expect(writeText).not.toHaveBeenCalled();
    expect(onConnect).not.toHaveBeenCalled();
    expect(onDisconnect).not.toHaveBeenCalled();
    const current = inputStream();
    mocks.clients[1].onclipboard!(current, "text/plain");
    current.onend!();
    expect(writeText).toHaveBeenCalledExactlyOnceWith("");
  });
});
