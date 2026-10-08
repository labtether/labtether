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
      type: "chunk", request_id: start.request_id, path: "/tmp/sample.txt", data: "QUJD", done: true,
    });
    channel.receive({ type: "ack", request_id: start.request_id, bytes_written: 3 });
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
