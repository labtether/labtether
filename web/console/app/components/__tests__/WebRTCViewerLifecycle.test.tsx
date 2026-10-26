import { act, createRef } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const signaling = vi.hoisted(() => ({
  handlers: new Map<string, (data: unknown) => void | Promise<void>>(),
  send: vi.fn(),
  on: (type: string, handler: (data: unknown) => void | Promise<void>) => {
    signaling.handlers.set(type, handler);
    return () => { signaling.handlers.delete(type); };
  },
}));
vi.mock("../../hooks/useWebRTCSignaling", () => ({
  useWebRTCSignaling: () => ({ send: signaling.send, on: signaling.on }),
}));

import WebRTCViewer, { type WebRTCViewerHandle } from "../WebRTCViewer";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

class FakeChannel {
  readyState = "open";
  bufferedAmount = 0;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn(() => { this.readyState = "closed"; this.onclose?.(); });
  send = vi.fn();
  receive(value: unknown) { this.onmessage?.({ data: JSON.stringify(value) }); }
}

class FakePeer {
  static instances: FakePeer[] = [];
  channels = new Map<string, FakeChannel>();
  connectionState = "new";
  onconnectionstatechange: (() => void) | null = null;
  onicecandidate: ((event: { candidate: RTCIceCandidateInit }) => void) | null = null;
  ontrack: ((event: RTCTrackEvent) => void) | null = null;
  close = vi.fn();
  addTransceiver = vi.fn();
  createOffer = vi.fn(async () => ({ sdp: "offer-sdp" }));
  setLocalDescription = vi.fn(async () => {});
  setRemoteDescription = vi.fn(async () => {});
  addIceCandidate = vi.fn(async () => {});
  getStats = vi.fn(async () => new Map());
  constructor() { FakePeer.instances.push(this); }
  createDataChannel(label: string) {
    const channel = new FakeChannel();
    this.channels.set(label, channel);
    return channel;
  }
}

let root: Root;
let container: HTMLDivElement;
let viewer: ReturnType<typeof createRef<WebRTCViewerHandle>>;
const onDisconnect = vi.fn();

async function render(wsUrl = "wss://example.test/first") {
  await act(async () => {
    root.render(<WebRTCViewer ref={viewer} wsUrl={wsUrl} onDisconnect={onDisconnect} />);
  });
}

async function ready() {
  await act(async () => { await signaling.handlers.get("ready")?.({}); });
  return FakePeer.instances.at(-1)!;
}

beforeEach(async () => {
  vi.useFakeTimers();
  FakePeer.instances = [];
  signaling.handlers.clear();
  signaling.send.mockReset();
  onDisconnect.mockReset();
  vi.stubGlobal("RTCPeerConnection", FakePeer);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  viewer = createRef<WebRTCViewerHandle>();
  await render();
});

afterEach(async () => {
  await act(async () => { root.unmount(); });
  container.remove();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("WebRTC viewer lifecycle", () => {
  it("keeps clipboard replies and file acknowledgments on their own channels", async () => {
    const peer = await ready();
    expect([...peer.channels.keys()]).toEqual(["input", "clipboard", "file-transfer"]);
    const clipboard = peer.channels.get("clipboard")!;
    const pendingRead = viewer.current!.requestClipboardText();
    expect(JSON.parse(clipboard.send.mock.calls[0][0])).toEqual({ type: "get", format: "text" });
    await expect(viewer.current!.writeClipboardText("busy")).rejects.toThrow("already in progress");
    clipboard.receive({ type: "data", text: "  exact\n" });
    await expect(pendingRead).resolves.toBe("  exact\n");

    const progress = vi.fn();
    const file = {
      name: "sample.txt", size: 3,
      slice: () => ({ arrayBuffer: async () => new Uint8Array([65, 66, 67]).buffer }),
    } as unknown as File;
    const transfer = viewer.current!.uploadFile(file, "/tmp/sample.txt", progress);
    const channel = peer.channels.get("file-transfer")!;
    const start = JSON.parse(channel.send.mock.calls[0][0]);
    expect(start).toMatchObject({ type: "start", name: "sample.txt", path: "/tmp/sample.txt" });
    await act(async () => { channel.receive({ type: "ready", request_id: start.request_id }); });
    expect(JSON.parse(channel.send.mock.calls[1][0])).toEqual({
      type: "chunk", request_id: start.request_id, path: "/tmp/sample.txt", offset: 0, data: "QUJD", done: true,
    });
    channel.receive({ type: "ack", request_id: start.request_id, bytes_written: 3, done: true });
    await expect(transfer).resolves.toBeUndefined();
    expect(progress).toHaveBeenCalledWith(3, 3);
  });

  it("rejects pending transfers on disconnect and does not send stale ICE after a URL change", async () => {
    const peer = await ready();
    const oldAnswer = signaling.handlers.get("answer")!;
    peer.onicecandidate?.({ candidate: { candidate: "candidate:1 1 udp 1 1.2.3.4 9000 typ relay" } });
    const clipboardResult = viewer.current!.requestClipboardText().catch((error: Error) => error.message);
    const fileResult = viewer.current!.uploadFile({ name: "pending", size: 1 } as File, "/tmp/pending")
      .catch((error: Error) => error.message);
    await render("wss://example.test/second");
    expect(peer.close).toHaveBeenCalledTimes(1);
    await expect(clipboardResult).resolves.toBe("session ended");
    await expect(fileResult).resolves.toBe("session ended");
    await act(async () => {
      await oldAnswer({ sdp: "stale-answer" });
      vi.advanceTimersByTime(400);
    });
    expect(peer.setRemoteDescription).not.toHaveBeenCalled();
    expect(signaling.send.mock.calls.filter(([type]) => type === "ice")).toEqual([]);
    expect(onDisconnect).toHaveBeenCalledWith({ clean: true, reason: "session ended" });
  });

  it("releases held keys on blur and suppresses repeated keydown frames", async () => {
    const peer = await ready();
    await act(async () => {
      peer.connectionState = "connected";
      peer.onconnectionstatechange?.();
    });
    const surface = container.querySelector<HTMLDivElement>(".vncContainer")!;
    const input = peer.channels.get("input")!;
    await act(async () => {
      surface.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, code: "KeyA", key: "a", keyCode: 65 }));
      surface.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, code: "KeyA", key: "a", keyCode: 65, repeat: true }));
      window.dispatchEvent(new Event("blur"));
    });
    expect(input.send.mock.calls.map(([payload]) => JSON.parse(payload))).toEqual([
      { type: "keydown", keyCode: 65, code: "KeyA", key: "a" },
      { type: "keyup", keyCode: 65, code: "KeyA", key: "a" },
    ]);
  });
});


describe("WebRTC transfer integrity", () => {
  it.each([0, 100_003])("commits exactly %i bytes with ordered offsets", async (size) => {
    const peer = await ready();
    const channel = peer.channels.get("file-transfer")!;
    const bytes = Uint8Array.from({ length: size }, (_, index) => index % 251);
    const received: number[] = [];
    let committed = false;
    channel.send.mockImplementation((raw: string) => {
      expect(new TextEncoder().encode(raw).length).toBeLessThanOrEqual(64 * 1024);
      const message = JSON.parse(raw);
      if (message.type === "start") {
        channel.receive({ type: "ready", request_id: message.request_id });
        return;
      }
      expect(message.offset).toBe(received.length);
      received.push(...Array.from(atob(message.data), (character) => character.charCodeAt(0)));
      committed = message.done;
      channel.receive({ type: "ack", request_id: message.request_id, bytes_written: received.length, done: message.done });
    });
    const file = {
      name: "bytes.bin", size,
      slice: (start: number, end: number) => ({ arrayBuffer: async () => bytes.slice(start, end).buffer }),
    } as File;
    await viewer.current!.uploadFile(file, "/tmp/bytes.bin");
    expect(new Uint8Array(received)).toEqual(bytes);
    expect(committed).toBe(true);
  });

  it("rejects a lost clipboard channel and times out missing file replies", async () => {
    const peer = await ready();
    const clipboardResult = viewer.current!.requestClipboardText().catch((error: Error) => error.message);
    peer.channels.get("clipboard")!.close();
    await expect(clipboardResult).resolves.toBe("clipboard channel closed");
    const uploadResult = viewer.current!.uploadFile({ name: "waiting", size: 1 } as File, "/tmp/waiting")
      .catch((error: Error) => error.message);
    await vi.advanceTimersByTimeAsync(15_000);
    await expect(uploadResult).resolves.toBe("file transfer timed out");
  });

  it("clears failed clipboard sends so a later request can proceed", async () => {
    const peer = await ready();
    const channel = peer.channels.get("clipboard")!;
    channel.send.mockImplementationOnce(() => { throw new Error("send failed"); });
    await expect(viewer.current!.requestClipboardText()).rejects.toThrow("send failed");
    const result = viewer.current!.requestClipboardText();
    channel.receive({ type: "data", text: "retry" });
    await expect(result).resolves.toBe("retry");
  });

  it("ignores delayed events from a timed-out clipboard channel after reconnect", async () => {
    const peer = await ready();
    const oldChannel = peer.channels.get("clipboard")!;
    const oldClose = oldChannel.onclose!;
    const oldError = oldChannel.onerror!;
    const oldMessage = oldChannel.onmessage!;
    const timedOut = viewer.current!.requestClipboardText().catch((error: Error) => error.message);
    await vi.advanceTimersByTimeAsync(15_000);
    await expect(timedOut).resolves.toBe("clipboard request timed out");
    await render("wss://example.test/after-timeout");
    const replacement = await ready();
    const result = viewer.current!.requestClipboardText();
    oldClose();
    oldError();
    oldMessage({ data: JSON.stringify({ type: "data", text: "stale" }) });
    replacement.channels.get("clipboard")!.receive({ type: "data", text: "current" });
    await expect(result).resolves.toBe("current");
  });
});

describe("WebRTC touch input", () => {
  it("maps a touch tap to video pixels and releases a held button on blur", async () => {
    const peer = await ready();
    await act(async () => {
      peer.connectionState = "connected";
      peer.onconnectionstatechange?.();
    });
    const surface = container.querySelector<HTMLDivElement>(".vncContainer")!;
    const video = surface.querySelector("video")!;
    Object.defineProperties(video, { videoWidth: { value: 1920 }, videoHeight: { value: 1080 } });
    video.getBoundingClientRect = () => ({ left: 0, top: 0, width: 960, height: 600 }) as DOMRect;
    const down = new MouseEvent("pointerdown", { bubbles: true, button: 0, buttons: 1, clientX: 480, clientY: 300 });
    Object.defineProperties(down, { isPrimary: { value: true }, pointerId: { value: 7 }, pointerType: { value: "touch" } });
    await act(async () => {
      surface.dispatchEvent(down);
      window.dispatchEvent(new Event("blur"));
    });
    const messages = peer.channels.get("input")!.send.mock.calls.map(([raw]) => JSON.parse(raw));
    expect(messages).toEqual([
      { type: "mousemove", x: 960, y: 540 },
      { type: "mousedown", button: 0 },
      { type: "mouseup", button: 0 },
    ]);
  });
});
