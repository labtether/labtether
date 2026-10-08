import type { DesktopMockEnvironment } from "./desktopBrowserEnvironment";

// Serialized into the same init script as the shared environment.
export function initDesktopWebSocketMocks({ settings, auditState }: DesktopMockEnvironment) {
  type AudioBehavior = DesktopMockEnvironment["settings"]["audioBehaviors"][number];

  const NativeWebSocket = window.WebSocket;

  class MockDesktopWebSocket {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;

    readyState = MockDesktopWebSocket.CONNECTING;
    binaryType: BinaryType = "blob";
    onopen: ((event: Event) => void) | null = null;
    onmessage: ((event: MessageEvent) => void) | null = null;
    onerror: ((event: Event) => void) | null = null;
    onclose: ((event: CloseEvent) => void) | null = null;
    url: string;
    private delegate: WebSocket | null = null;
    private listeners = new Map<string, Array<(event: Event) => void>>();

    constructor(url: string) {
      this.url = url;
      if (!this.url.includes("desktop-webrtc") && !this.url.includes("desktop-vnc") && !this.url.includes("desktop-audio")) {
        const socket = new NativeWebSocket(url);
        this.delegate = socket;
        socket.binaryType = this.binaryType;
        socket.addEventListener("open", (event) => {
          this.readyState = socket.readyState;
          this.onopen?.(event);
          this.dispatch("open", event);
        });
        socket.addEventListener("message", (event) => {
          this.readyState = socket.readyState;
          this.onmessage?.(event);
          this.dispatch("message", event);
        });
        socket.addEventListener("error", (event) => {
          this.readyState = socket.readyState;
          this.onerror?.(event);
          this.dispatch("error", event);
        });
        socket.addEventListener("close", (event) => {
          this.readyState = socket.readyState;
          this.onclose?.(event);
          this.dispatch("close", event);
        });
        return;
      }
      window.setTimeout(() => {
        if (this.readyState !== MockDesktopWebSocket.CONNECTING) {
          return;
        }
        this.readyState = MockDesktopWebSocket.OPEN;
        const openEvent = new Event("open");
        this.onopen?.(openEvent);
        this.dispatch("open", openEvent);
        if (this.url.includes("desktop-webrtc")) {
          auditState.webrtcEvents.push("ws:ready");
          this.emit({ type: "ready", data: {} });
          return;
        }
        if (this.url.includes("desktop-audio")) {
          auditState.audioConnections.push(this.url);
          const behavior =
            settings.audioBehaviors[auditState.audioConnections.length - 1] ??
            ({ state: "started" } as AudioBehavior);
          this.emit({
            state: behavior.state,
            ...(behavior.error ? { error: behavior.error } : {}),
          });
        }
      }, 0);
    }

    send(data: unknown) {
      if (this.delegate) {
        // TS 6's lib.dom typings tightened WebSocket.send to require
        // BufferSource, which excludes SharedArrayBuffer. Cast to the
        // narrower set of types the mock is known to pass through.
        this.delegate.send(data as string | ArrayBuffer | Blob | ArrayBufferView<ArrayBuffer>);
        return;
      }
      const payload = typeof data === "string" ? data : String(data);
      auditState.websocketMessages.push({ url: this.url, payload });
      if (!this.url.includes("desktop-webrtc")) {
        return;
      }
      try {
        const parsed = JSON.parse(payload) as { type?: string };
        if (parsed.type === "offer") {
          auditState.webrtcEvents.push("ws:offer");
          this.emit({
            type: "answer",
            data: { sdp: "mock-answer-sdp" },
          });
        }
      } catch {
        // Ignore malformed frames.
      }
    }

    close(code = 1000, reason = "") {
      if (this.delegate) {
        this.delegate.close(code, reason);
        return;
      }
      this.readyState = MockDesktopWebSocket.CLOSED;
      const closeEvent = {
        code,
        reason,
        wasClean: code === 1000,
      } as CloseEvent;
      this.onclose?.(closeEvent);
      this.dispatch("close", closeEvent);
    }

    addEventListener(type: string, listener: (event: Event) => void) {
      const existing = this.listeners.get(type) ?? [];
      existing.push(listener);
      this.listeners.set(type, existing);
    }

    removeEventListener(type: string, listener: (event: Event) => void) {
      const existing = this.listeners.get(type) ?? [];
      this.listeners.set(
        type,
        existing.filter((candidate) => candidate !== listener),
      );
    }

    private emit(payload: unknown) {
      const event = {
        data: JSON.stringify(payload),
      } as MessageEvent;
      this.onmessage?.(event);
      this.dispatch("message", event);
    }

    private dispatch(type: string, event: Event) {
      const listeners = this.listeners.get(type) ?? [];
      for (const listener of listeners) {
        listener(event);
      }
    }
  }

  (window as unknown as { WebSocket: unknown }).WebSocket = MockDesktopWebSocket;
}
